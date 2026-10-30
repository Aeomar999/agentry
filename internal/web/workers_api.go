package web

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers/workersapi"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
)

// workersAPI is the Workers page's routes as web mounts them.
func (s *Server) workersAPI() workersapi.API {
	api := workersapi.API{Log: s.store, Events: s.store, State: s.state, Workers: s.workers, Roster: s.roster,
		Local: s.config.Local, InstallCommands: s.installCommands}
	if s.config.Fleet != nil {
		api.About = func(ctx context.Context, name string) (machines.About, error) {
			return s.config.Fleet.Of(name).About(ctx)
		}
	}
	if s.config.Workers != nil {
		api.Disconnect = s.config.Workers.Disconnect
	}
	return api
}

// publicURL is the server's address as people and workers reach it: the
// one it was started with, or the one this request came to.
func (s *Server) publicURL(r *http.Request) string {
	if s.config.PublicURL != "" {
		return strings.TrimSuffix(s.config.PublicURL, "/")
	}
	return requestURL(r)
}

// joinRequest is what `agentry worker join` sends with its code.
type joinRequest struct {
	Code    string          `json:"code"`
	Name    string          `json:"name"` // what it would like to be called; "" for its host's name
	Machine workers.Machine `json:"machine"`
}

// join lets a machine in with a join code, under the name it asked for or,
// if that is taken, the first free one after it (laptop-2, laptop-3).
func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	var request joinRequest
	if !httpapi.Decode(w, r, &request) {
		return
	}
	roster, err := s.workers()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	if !roster.CanJoin(workers.Hash(request.Code), time.Now()) {
		http.Error(w, "that join code is unknown, used or expired; make a new one on the Workers page", http.StatusUnauthorized)
		return
	}
	name := freeName(roster, workerName(request.Name, request.Machine.Host))
	credential, p := workers.Join(name, request.Code, request.Machine)
	if _, rejection, err := s.store.Propose(p); err != nil || rejection != nil {
		http.Error(w, firstError(err, rejection).Error(), http.StatusConflict)
		return
	}
	httpapi.Reply(w, map[string]string{"name": name, "credential": credential})
}

var notInName = regexp.MustCompile(`[^a-z0-9-]+`)

// workerName makes a name the rules take from what the machine asked for,
// or its host's name: lowercase letters, digits and dashes, from a letter.
func workerName(asked, host string) string {
	name := strings.Trim(notInName.ReplaceAllString(strings.ToLower(asked), "-"), "-")
	if name == "" {
		host, _, _ = strings.Cut(host, ".")
		name = strings.Trim(notInName.ReplaceAllString(strings.ToLower(host), "-"), "-")
	}
	if name == "" || name[0] < 'a' || name[0] > 'z' {
		name = "worker-" + name
	}
	return strings.TrimSuffix(name[:min(len(name), 28)], "-")
}

func freeName(roster workers.Roster, name string) string {
	taken := func(n string) bool { _, exists := roster.Get(n); return exists || n == workers.Local }
	candidate := name
	for i := 2; taken(candidate); i++ {
		candidate = name + "-" + strconv.Itoa(i)
	}
	return candidate
}

// requireWorker lets in a worker that shows its credential, and tells the
// handler which worker it is.
func (s *Server) requireWorker(next func(http.ResponseWriter, *http.Request, workers.Worker)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roster, err := s.workers()
		if err != nil {
			httpapi.Failed(w, err)
			return
		}
		worker, ok := roster.WithCredential(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if !ok {
			http.Error(w, "this worker isn't joined; join it again from the Workers page", http.StatusUnauthorized)
			return
		}
		next(w, r, worker)
	})
}

// leave takes the worker off at its own request, as on the page.
func (s *Server) leave(w http.ResponseWriter, r *http.Request, worker workers.Worker) {
	httpapi.Propose(w, s.store, workers.Left(worker.Name))
	s.config.Workers.Disconnect(worker.Name) // a no-op if it was refused: the credential still works
}

func (s *Server) connectWorker(w http.ResponseWriter, r *http.Request, worker workers.Worker) {
	s.config.Workers.Connect(w, r, worker)
}

func (s *Server) workerStream(w http.ResponseWriter, r *http.Request, worker workers.Worker) {
	s.config.Workers.Stream(w, r, worker, r.PathValue("id"))
}

//go:embed install.sh
var installTemplate string

//go:embed install.ps1
var installWindowsTemplate string

const unknownJoinCode = "That join code is unknown, used or expired. Make a new one on the Workers page."

// installScript is the script the join command runs: for sh, or for
// PowerShell on Windows.
func (s *Server) installScript(w http.ResponseWriter, r *http.Request) {
	s.serveScript(w, r, shScript(installTemplate), s.codeWorks, unknownJoinCode)
}

func (s *Server) installWindowsScript(w http.ResponseWriter, r *http.Request) {
	s.serveScript(w, r, powerShellScript(installWindowsTemplate), s.codeWorks, unknownJoinCode)
}

// downloadBinary sends agentry for the joining machine's kind, for a code
// that works.
func (s *Server) downloadBinary(w http.ResponseWriter, r *http.Request) {
	if !s.codeWorks(r.URL.Query().Get("code")) {
		http.Error(w, "that join code is unknown, used or expired", http.StatusUnauthorized)
		return
	}
	s.serveBinary(w, r)
}

// updateBinary is the same, for a worker updating itself: it sends the hash
// of the agentry it has as If-None-Match, and gets 304 if it is current.
func (s *Server) updateBinary(w http.ResponseWriter, r *http.Request, _ workers.Worker) {
	s.serveBinary(w, r)
}

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

func (s *Server) codeWorks(code string) bool {
	roster, err := s.workers()
	return err == nil && code != "" && roster.CanJoin(workers.Hash(code), time.Now())
}

func (s *Server) workers() (workers.Roster, error) {
	return s.state.Workers(), s.state.Broken(events.WorkersStream)
}

// shellQuoted quotes a value for the install script. Codes and addresses
// have no quotes in them, but a Host header could.
func shellQuoted(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func firstError(err error, rejection *events.Rejection) error {
	if err != nil {
		return err
	}
	return rejection
}
