// Package connectionsapi is the connections' HTTP API, as the Integrations
// page uses it: make an install command for Claude Code on another machine,
// list the machines connected, and revoke one. Web mounts its Routes as it
// mounts an extension's.
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

// API holds what the connections' routes need: the log to write invites
// and revocations to, the state to list connections from, and web's install
// commands for the address the request came in on.
type API struct {
	Log             extensions.Proposer
	State           State
	InstallCommands func(r *http.Request, path, code string) map[string]any
}

// Routes are the connections' endpoints, for the Integrations page.
func (a API) Routes() []extensions.Route {
	return []extensions.Route{
		{Pattern: "POST /api/integrations/claude-code", Auth: extensions.UI, Handler: a.inviteClaudeCode},
		{Pattern: "GET /api/integrations/connections", Auth: extensions.UI, Handler: a.listConnections},
		{Pattern: "DELETE /api/integrations/connections/{id}", Auth: extensions.UI, Handler: a.revokeConnection},
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
