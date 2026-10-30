package workerlink

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// UpdatePath is where a worker gets the server's agentry for its kind of
// machine, so workers run what the server runs without anyone reinstalling.
const UpdatePath = "/api/worker/agentry"

// ErrUpdated means this worker's agentry was replaced with the server's
// newer one: the process restarts into it (`agentry worker` does).
var ErrUpdated = errors.New("agentry was updated; restarting into the new one")

// Update replaces the running agentry with the server's build for this
// machine when the two differ, and reports whether it did. The running
// file is replaced whole, so a failure leaves the old one in place.
func (w *Worker) Update(ctx context.Context) (bool, error) {
	self, err := os.Executable()
	if err != nil {
		return false, err
	}
	if self, err = filepath.EvalSymlinks(self); err != nil {
		return false, err
	}
	return w.replace(ctx, self)
}

// Newer says whether the server has a different agentry for this machine
// than the one at self, without fetching it.
func (w *Worker) Newer(ctx context.Context, self string) (bool, error) {
	response, err := w.askForUpdate(ctx, http.MethodHead, self)
	if err != nil {
		return false, err
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK, nil
}

// askForUpdate asks the server for its agentry for this machine, unless it
// is the one at self: then the answer is 304 Not Modified.
func (w *Worker) askForUpdate(ctx context.Context, method, self string) (*http.Response, error) {
	have, err := fileHash(self)
	if err != nil {
		return nil, err
	}
	url := fmt.Sprintf("%s%s?os=%s&arch=%s", w.Server, UpdatePath, runtime.GOOS, runtime.GOARCH)
	request, err := http.NewRequestWithContext(ctx, method, url, nil)
	if err != nil {
		return nil, err
	}
	request.Header = w.header()
	request.Header.Set("If-None-Match", `"`+have+`"`)
	return w.client().Do(request)
}

// replace swaps the agentry at self for the server's, if they differ.
func (w *Worker) replace(ctx context.Context, self string) (bool, error) {
	response, err := w.askForUpdate(ctx, http.MethodGet, self)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusNotModified, http.StatusNotFound: // current, or the server has no build for this machine
		return false, nil
	case http.StatusOK:
	default:
		return false, fmt.Errorf("getting the server's agentry: %s", response.Status)
	}

	want := strings.Trim(response.Header.Get("ETag"), `"`)
	next := self + ".new"
	file, err := os.OpenFile(next, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return false, err
	}
	sum := sha256.New()
	_, err = io.Copy(io.MultiWriter(file, sum), response.Body)
	file.Close()
	if err == nil && hex.EncodeToString(sum.Sum(nil)) != want {
		err = errors.New("the new agentry didn't arrive whole")
	}
	if err != nil {
		os.Remove(next)
		return false, err
	}
	return true, swap(next, self)
}

// swap puts the new agentry at self. Windows won't replace a program while
// it runs, but lets it be moved aside: the old one goes to self.old, and
// the next update removes it.
func swap(next, self string) error {
	if runtime.GOOS != "windows" {
		return os.Rename(next, self)
	}
	old := self + ".old"
	os.Remove(old)
	if err := os.Rename(self, old); err != nil {
		os.Remove(next)
		return err
	}
	if err := os.Rename(next, self); err != nil {
		os.Rename(old, self)
		return err
	}
	return nil
}

func fileHash(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}
