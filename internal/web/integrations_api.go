package web

import (
	_ "embed"
	"net/http"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// The Integrations page connects Claude Code on any machine to Agentry
// (agentry mcp): you make an install command, run it there, and that
// machine's Claude Code has Agentry's tools. Each connection is listed, and
// revoking it stops its credential working.

//go:embed claude-code.sh
var claudeCodeTemplate string

//go:embed claude-code.ps1
var claudeCodeWindowsTemplate string

const unknownInstallCode = "That install command is unknown, used or expired. Make a new one on the Integrations page."

// inviteClaudeCode makes a one-time install command.
func (s *Server) inviteClaudeCode(w http.ResponseWriter, r *http.Request) {
	code, invited := connections.Invite(httpapi.You, time.Now())
	if !httpapi.ProposeOrRefuse(w, s.store, invited) {
		return
	}
	answer := s.installCommands(r, "/integrations/claude-code", code)
	answer["expires"] = time.Now().Add(connections.InviteLifetime)
	httpapi.Reply(w, answer)
}

// claudeCodeScript is the install script: for sh, or for PowerShell on Windows.
func (s *Server) claudeCodeScript(w http.ResponseWriter, r *http.Request) {
	s.serveScript(w, r, shScript(claudeCodeTemplate), s.connectionCodeWorks, unknownInstallCode)
}

func (s *Server) claudeCodeWindowsScript(w http.ResponseWriter, r *http.Request) {
	s.serveScript(w, r, powerShellScript(claudeCodeWindowsTemplate), s.connectionCodeWorks, unknownInstallCode)
}

// integrationBinary sends agentry for the machine, for a code that works.
func (s *Server) integrationBinary(w http.ResponseWriter, r *http.Request) {
	if !s.connectionCodeWorks(r.URL.Query().Get("code")) {
		http.Error(w, "that install command is unknown, used or expired", http.StatusUnauthorized)
		return
	}
	s.serveBinary(w, r)
}

// connect swaps a code for the machine's credential, once.
func (s *Server) connect(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Code string
		connections.Machine
	}
	if !httpapi.Decode(w, r, &request) {
		return
	}
	credential, made := connections.Connect(request.Code, request.Machine)
	if !httpapi.ProposeOrRefuse(w, s.store, made) {
		return
	}
	httpapi.Reply(w, map[string]string{"credential": credential, "page": s.publicURL(r)})
}

func (s *Server) listConnections(w http.ResponseWriter, _ *http.Request) {
	roster, err := s.connections()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	list := roster.Connections
	if list == nil {
		list = []connections.Connection{}
	}
	httpapi.Reply(w, map[string]any{"connections": list})
}

func (s *Server) revokeConnection(w http.ResponseWriter, r *http.Request) {
	httpapi.Propose(w, s.store, connections.Revocation(httpapi.You, r.PathValue("id")))
}

func (s *Server) connectionCodeWorks(code string) bool {
	roster, err := s.connections()
	return err == nil && code != "" && roster.CanConnect(workers.Hash(code), time.Now())
}

// carriesConnection is a request from a connected Claude Code: its
// credential opens what the page's token does, until it is revoked.
func (s *Server) carriesConnection(r *http.Request) bool {
	credential := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if credential == "" {
		return false
	}
	roster, err := s.connections()
	if err != nil {
		return false
	}
	_, ok := roster.WithCredential(credential)
	return ok
}

func (s *Server) connections() (connections.Roster, error) {
	return s.state.Connections(), s.state.Broken(events.ConnectionsStream)
}
