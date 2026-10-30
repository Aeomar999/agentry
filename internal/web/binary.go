package web

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
)

// serveBinary sends agentry for ?os= and ?arch=: a build for it next to this
// binary (agentry-<os>-<arch>, as `make workers` makes them), or this binary
// itself if it is that kind. Its ETag is its SHA-256.
func (s *Server) serveBinary(w http.ResponseWriter, r *http.Request) {
	goos, goarch := r.URL.Query().Get("os"), r.URL.Query().Get("arch")
	path := s.binaryFor(goos, goarch)
	if path == "" {
		http.Error(w, "this server has no agentry for "+goos+"/"+goarch, http.StatusNotFound)
		return
	}
	file, err := os.Open(path)
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		httpapi.Failed(w, err)
		return
	}
	file.Seek(0, io.SeekStart)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum.Sum(nil))+`"`)
	w.Header().Set("Content-Type", "application/octet-stream")
	http.ServeContent(w, r, "agentry", time.Time{}, file) // answers If-None-Match with 304
}

var platform = regexp.MustCompile(`^[a-z0-9]+$`)

func (s *Server) binaryFor(goos, goarch string) string {
	if s.config.Binary == "" || !platform.MatchString(goos) || !platform.MatchString(goarch) {
		return ""
	}
	build := filepath.Join(filepath.Dir(s.config.Binary), "agentry-"+goos+"-"+goarch)
	if _, err := os.Stat(build); err == nil {
		return build
	}
	if goos == runtime.GOOS && goarch == runtime.GOARCH {
		return s.config.Binary
	}
	return ""
}
