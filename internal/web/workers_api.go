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
	"slices"
	"strconv"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// Workers: the machines, besides this one, that agents run on. You make a
// join code on the page; the page shows one command that installs agentry
// on the other machine, joins it with the code, and starts it.

// workerView is a worker as the page shows it, with the tasks whose work is on it.
type workerView struct {
	workers.Worker
	Tasks []string `json:"tasks"`
}

func (s *Server) listWorkers(w http.ResponseWriter, r *http.Request) {
	roster, err := s.workers()
	if err != nil {
		failed(w, err)
		return
	}
	on := s.tasksByMachine()
	list := []workerView{}
	for _, worker := range roster.Workers {
		worker.Engines, worker.Missing = listOf(worker.Engines), listOf(worker.Missing)
		list = append(list, workerView{Worker: worker, Tasks: listOf(on[worker.Name])})
	}
	local := s.config.Local
	local.Engines, local.Missing = listOf(local.Engines), listOf(local.Missing)
	reply(w, map[string]any{"workers": list, "local": map[string]any{"tasks": listOf(on[workers.Local]), "machine": local}})
}

// workerDetail is one machine as the Workers page's side panel shows it.
type workerDetail struct {
	Worker     workerView      `json:"worker"`
	About      *machines.About `json:"about"`      // read from the machine now; nil if it couldn't be
	AboutError string          `json:"aboutError"` // why not
	Agents     []string        `json:"agents"`     // the agents that may run on it
	History    []historyEntry  `json:"history"`    // when it joined, came online and went offline, newest first
}

type historyEntry struct {
	At   time.Time `json:"at"`
	What string    `json:"what"` // joined, online, offline
}

// showWorker is one machine in detail: workers.Local, or a worker's name.
func (s *Server) showWorker(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	roster, err := s.workers()
	if err != nil {
		failed(w, err)
		return
	}
	var detail workerDetail
	if name == workers.Local {
		local := s.config.Local
		detail.Worker = workerView{Worker: workers.Worker{Name: workers.Local, Host: local.Host, OS: local.OS, Arch: local.Arch,
			Online: true, Version: local.Version, Engines: listOf(local.Engines), Missing: listOf(local.Missing)}}
	} else if worker, ok := roster.Get(name); ok {
		worker.Engines, worker.Missing = listOf(worker.Engines), listOf(worker.Missing)
		detail.Worker = workerView{Worker: worker}
	} else {
		http.Error(w, "no worker called "+name, http.StatusNotFound)
		return
	}
	detail.Worker.Tasks = listOf(s.tasksByMachine()[name])

	if s.config.Fleet != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		if about, err := s.config.Fleet.Of(name).About(ctx); err != nil {
			detail.AboutError = err.Error()
		} else {
			detail.About = &about
		}
	}

	agentRoster, _ := s.roster()
	detail.Agents = []string{}
	for _, agent := range agentRoster {
		if slices.Contains(agent.Workers, name) || name == workers.Local && len(agent.Workers) == 0 {
			detail.Agents = append(detail.Agents, agent.Name)
		}
	}

	detail.History = []historyEntry{}
	stream, _ := s.store.Stream(events.WorkersStream)
	for i := len(stream) - 1; i >= 0 && len(detail.History) < 20; i-- {
		e := stream[i]
		what := map[events.Type]string{events.WorkerJoined: "joined", events.WorkerConnected: "online",
			events.WorkerDisconnected: "offline"}[e.Type]
		if what != "" && e.Text("name") == name {
			detail.History = append(detail.History, historyEntry{At: e.At, What: what})
		}
		if e.Type == events.WorkerJoined && e.Text("name") == name {
			break // what came before was an earlier machine by this name
		}
	}
	reply(w, detail)
}

// tasksByMachine is every unfinished task with a worktree, by the machine it is on.
func (s *Server) tasksByMachine() map[string][]string {
	on := map[string][]string{}
	ids, _ := s.store.TaskIDs()
	for _, id := range ids {
		if task, err := s.task(id); err == nil && task != nil && !task.IsDone && task.Worktree != nil {
			on[task.Worktree.Worker] = append(on[task.Worktree.Worker], task.ID)
		}
	}
	return on
}

// invite makes a join code, and the command that uses it.
func (s *Server) invite(w http.ResponseWriter, r *http.Request) {
	code, p := workers.Invite(you, time.Now())
	if _, rejection, err := s.store.Propose(p); err != nil || rejection != nil {
		failed(w, firstError(err, rejection))
		return
	}
	install := s.publicURL(r) + "/worker/install.sh?code=" + code
	reply(w, map[string]any{"code": code, "expires": time.Now().Add(workers.InviteLifetime),
		"command": "curl -fsSL '" + install + "' | sh"})
}

func (s *Server) removeWorker(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	s.propose(w, workers.Removal(you, name))
	if s.config.Workers != nil {
		s.config.Workers.Disconnect(name)
	}
}

// publicURL is the server's address as people and workers reach it: the
// one it was started with, or the one this request came to.
func (s *Server) publicURL(r *http.Request) string {
	if s.config.PublicURL != "" {
		return strings.TrimSuffix(s.config.PublicURL, "/")
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
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
	if !decode(w, r, &request) {
		return
	}
	roster, err := s.workers()
	if err != nil {
		failed(w, err)
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
	reply(w, map[string]string{"name": name, "credential": credential})
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
			failed(w, err)
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
	s.propose(w, workers.Left(worker.Name))
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

// installScript is the script the join command pipes to sh, with the code
// in it. Anyone can fetch it, but it does nothing without a code that works.
func (s *Server) installScript(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	if !s.codeWorks(code) {
		http.Error(w, "echo 'That join code is unknown, used or expired. Make a new one on the Workers page.' >&2; exit 1", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "text/x-shellscript")
	script := strings.NewReplacer("{{SERVER}}", shellQuoted(s.publicURL(r)), "{{CODE}}", shellQuoted(code)).Replace(installTemplate)
	w.Write([]byte(script))
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
		failed(w, err)
		return
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		failed(w, err)
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
	stream, err := s.store.Stream(events.WorkersStream)
	return workers.FromEvents(stream), err
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
