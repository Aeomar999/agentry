// Package connectionsapi is the connections' HTTP API, as the Integrations
// page uses it: make an install command for Claude Code on another machine,
// list the machines connected, and revoke one. It also serves what that
// machine calls with its install command's code: the install scripts,
// agentry, and connect. Web mounts its Routes as it mounts an extension's.
package connectionsapi

import (
	"net/http"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/connections"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// State is what the routes read: the connections, and whether their stream
// is broken.
type State interface {
	Connections() connections.Roster
	Broken(stream string) error
}

// API holds what the connections' routes need: the log to write invites,
// connections and revocations to, the state to list connections from, and
// from web, the install commands and the server's address for the address
// the request came in on, and what a connecting machine downloads.
type API struct {
	Log             extensions.Proposer
	State           State
	InstallCommands func(r *http.Request, path, code string) map[string]any
	PublicURL       func(r *http.Request) string // the address connect replies with as the page's
	ServeBinary     http.HandlerFunc             // agentry for ?os= and ?arch=, with its hash as its ETag
	// ServeScript sends template, the script called name (sh, or PowerShell
	// for a .ps1), filled for the request's code if works says it is one
	// that works, or else refusal, as the shell shows it.
	ServeScript func(w http.ResponseWriter, r *http.Request, name, template string, works func(code string) bool, refusal string)
}

// Routes are the connections' endpoints, for the Integrations page, and
// the ones a connecting machine calls with its code.
func (a API) Routes() []extensions.Route {
	return []extensions.Route{
		{Pattern: "POST /api/integrations/claude-code", Auth: extensions.UI, Handler: a.inviteClaudeCode},
		{Pattern: "GET /api/integrations/connections", Auth: extensions.UI, Handler: a.listConnections},
		{Pattern: "DELETE /api/integrations/connections/{id}", Auth: extensions.UI, Handler: a.revokeConnection},
		{Pattern: "GET /integrations/claude-code.sh", Auth: extensions.Open, Handler: a.claudeCodeScript},
		{Pattern: "GET /integrations/claude-code.ps1", Auth: extensions.Open, Handler: a.claudeCodeWindowsScript},
		{Pattern: "GET /integrations/agentry", Auth: extensions.Open, Handler: a.downloadBinary},
		{Pattern: "POST /api/integrations/connect", Auth: extensions.Open, Handler: a.connect},
	}
}

// inviteClaudeCode makes a one-time install command.
func (a API) inviteClaudeCode(w http.ResponseWriter, r *http.Request) {
	code, invited := connections.Invite(httpapi.You, time.Now())
	if !httpapi.ProposeOrRefuse(w, a.Log, invited) {
		return
	}
	answer := a.InstallCommands(r, "/integrations/claude-code", code)
	answer["expires"] = time.Now().Add(connections.InviteLifetime)
	httpapi.Reply(w, answer)
}

func (a API) listConnections(w http.ResponseWriter, _ *http.Request) {
	if err := a.State.Broken(events.ConnectionsStream); err != nil {
		httpapi.Failed(w, err)
		return
	}
	list := a.State.Connections().Connections
	if list == nil {
		list = []connections.Connection{}
	}
	httpapi.Reply(w, map[string]any{"connections": list})
}

func (a API) revokeConnection(w http.ResponseWriter, r *http.Request) {
	httpapi.Propose(w, a.Log, connections.Revocation(httpapi.You, r.PathValue("id")))
}
