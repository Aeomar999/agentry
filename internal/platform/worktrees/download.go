package worktrees

import (
	"archive/tar"
	"archive/zip"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// A repo can also be named by a link to an archive of it, such as a release's
// .zip or .tar.gz: Agentry downloads it, unpacks it, and makes it a git repo,
// so a task can work in it like any other.

// archiveKinds are the archives Agentry unpacks, by the end of their name.
var archiveKinds = []string{".zip", ".tar.gz", ".tgz", ".tar.bz2", ".tar"}

// maxArchive is the most a download may be, and the most it may unpack to:
// enough for any source tree. A variable, so tests can lower it.
var maxArchive int64 = 2 << 30

// archiveKind is the kind of archive address links to, or "" if it isn't one.
func archiveKind(address string) string {
	link, err := url.Parse(address)
	if err != nil || (link.Scheme != "http" && link.Scheme != "https") {
		return ""
	}
	for _, kind := range archiveKinds {
		if strings.HasSuffix(strings.ToLower(link.Path), kind) {
			return kind
		}
	}
	return ""
}

// download makes folder a git repo of the archive at address: unpacked, with
// the folder an archive usually wraps everything in taken away, and all of it
// in a first commit.
func download(folder, address, kind string) error {
	file, err := fetch(address)
	if err != nil {
		return err
	}
	defer os.Remove(file)
	if err := os.MkdirAll(folder, 0o755); err != nil {
		return err
	}
	if kind == ".zip" {
		err = unzip(file, folder)
	} else {
		err = untar(file, folder, kind)
	}
	if errors.Is(err, errTooBig) {
		err = fmt.Errorf("it unpacks to more than %s", gigabytes(maxArchive))
	}
	if err != nil {
		return fmt.Errorf("unpacking %s: %w", address, err)
	}
	if err := unwrap(folder); err != nil {
		return err
	}
	if _, err := git(folder, "init", "-q", "-b", "main"); err != nil {
		return err
	}
	if _, err := git(folder, "add", "--all"); err != nil {
		return err
	}
	commit := []string{"commit", "-q", "--allow-empty", "--no-verify", "-m", "Downloaded from " + address}
	if email, _ := git(folder, "config", "user.email"); email == "" {
		commit = append([]string{"-c", "user.name=Agentry", "-c", "user.email=agentry@localhost"}, commit...)
	}
	_, err = git(folder, commit...)
	return err
}

// fetch downloads address into a file of its own, and gives its path.
func fetch(address string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), CloneTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return "", err
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		return "", fmt.Errorf("downloading %s: %w", address, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("downloading %s: the server said %s", address, response.Status)
	}
	file, err := os.CreateTemp("", "agentry-download-*")
	if err != nil {
		return "", err
	}
	defer file.Close()
	written, err := io.Copy(file, io.LimitReader(response.Body, maxArchive+1))
	if err == nil && written > maxArchive {
		err = fmt.Errorf("%s is bigger than %s", address, gigabytes(maxArchive))
	}
	if err != nil {
		os.Remove(file.Name())
		return "", err
	}
	return file.Name(), nil
}

func gigabytes(n int64) string {
	return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
}

var errTooBig = errors.New("the archive unpacks to too much")

func unzip(file, folder string) error {
	archive, err := zip.OpenReader(file)
	if err != nil {
		return err
	}
	defer archive.Close()
	left := maxArchive
	for _, entry := range archive.File {
		if entry.FileInfo().IsDir() {
			continue
		}
		if !entry.Mode().IsRegular() {
			continue // links could point out of the folder
		}
		contents, err := entry.Open()
		if err != nil {
			return err
		}
		err = writeEntry(folder, entry.Name, entry.Mode(), contents, &left)
		contents.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func untar(file, folder, kind string) error {
	opened, err := os.Open(file)
	if err != nil {
		return err
	}
	defer opened.Close()
	var stream io.Reader = opened
	switch kind {
	case ".tar.gz", ".tgz":
		unzipped, err := gzip.NewReader(opened)
		if err != nil {
			return err
		}
		defer unzipped.Close()
		stream = unzipped
	case ".tar.bz2":
		stream = bzip2.NewReader(opened)
	}
	archive := tar.NewReader(stream)
	left := maxArchive
	for {
		header, err := archive.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Typeflag == tar.TypeReg { // links could point out of the folder
			if err := writeEntry(folder, header.Name, header.FileInfo().Mode(), archive, &left); err != nil {
				return err
			}
		}
	}
}

// writeEntry writes one file of an archive into folder, taking what it
// writes from left. A name that would land outside the folder is refused, and
// git's own files are left out: their config and hooks would run when Agentry
// and the agents use git there.
func writeEntry(folder, name string, mode os.FileMode, contents io.Reader, left *int64) error {
	path := filepath.Join(folder, filepath.FromSlash(name))
	if !strings.HasPrefix(path, filepath.Clean(folder)+string(filepath.Separator)) {
		return fmt.Errorf("the archive has a file outside its folder: %s", name)
	}
	if inGitFolder(name) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode.Perm()|0o600)
	if err != nil {
		return err
	}
	written, err := io.Copy(file, io.LimitReader(contents, *left+1))
	*left -= written
	if err == nil && *left < 0 {
		err = errTooBig
	}
	if err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

// inGitFolder says whether an archive's file is a .git folder's, or a .git
// file pointing git at another repo, in any case, as macOS and Windows match
// names.
func inGitFolder(name string) bool {
	for _, part := range strings.FieldsFunc(name, func(r rune) bool { return r == '/' || r == '\\' }) {
		if strings.EqualFold(part, ".git") {
			return true
		}
	}
	return false
}

// unwrap takes away the one folder an archive often has everything in, such
// as app-1.2/, so the project is at the top of the repo.
func unwrap(folder string) error {
	entries, err := os.ReadDir(folder)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return err
	}
	// Moved aside first, so a file in it named like it doesn't land on it.
	inner := filepath.Join(folder, ".agentry-unwrap")
	if err := os.Rename(filepath.Join(folder, entries[0].Name()), inner); err != nil {
		return err
	}
	children, err := os.ReadDir(inner)
	if err != nil {
		return err
	}
	for _, child := range children {
		if err := os.Rename(filepath.Join(inner, child.Name()), filepath.Join(folder, child.Name())); err != nil {
			return err
		}
	}
	return os.Remove(inner)
}
