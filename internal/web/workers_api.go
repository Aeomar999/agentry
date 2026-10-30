package web

import (
	_ "embed"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
)

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

func (s *Server) codeWorks(code string) bool {
	roster, err := s.workers()
	return err == nil && code != "" && roster.CanJoin(workers.Hash(code), time.Now())
}

func firstError(err error, rejection *events.Rejection) error {
	if err != nil {
		return err
	}
	return rejection
}
