// Package agentsapi is the Agents page's HTTP API: the agents and their
// teams, adding, changing, renaming and removing them, what each knows,
// and the team templates, built in and yours, saved, exported and imported.
// Web mounts its Routes as it mounts an extension's.
package agentsapi

import (
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// State is what the routes read: the roster and your templates, what the
// rules check a team edit against, and every task, for the agents at work.
type State interface {
	extensions.Reader
	Tasks() []*tasks.Task
}

// Policy is the policy in force, whose profiles the agents use.
type Policy interface {
	Current() *policy.Policy
}

// API holds what the agents' routes need: the log to write to and read the
// agents' history from, the state, the agents and the workers as web reads
// them, the policy (nil for none) with the one way to change it and the
// view of the profile that judges an agent, and this Agentry's version.
type API struct {
	Log         extensions.Proposer
	Events      extensions.EventLog
	State       State
	Roster      func() (agents.Roster, error)
	Workers     func() (workers.Roster, error)
	Policy      Policy
	ApplyPolicy func(w http.ResponseWriter, seen, change string, edit func(text []byte) ([]byte, error)) (*policy.Change, bool)
	EffectiveOf func(p *policy.Policy, profile string) any // the profile that judges an agent's calls, as the page shows it
	Version     string                                     // as the comment above a profile it adds says
}

// Routes are the agents' and teams' endpoints, for the page alone.
func (a API) Routes() []extensions.Route {
	return []extensions.Route{
		{Pattern: "GET /api/agents", Auth: extensions.UI, Handler: a.listAgents},
		{Pattern: "GET /api/teams/templates", Auth: extensions.UI, Handler: a.teamTemplates},
		{Pattern: "POST /api/teams", Auth: extensions.UI, Handler: a.addTeam},
		{Pattern: "DELETE /api/teams/{team}", Auth: extensions.UI, Handler: a.removeTeam},
		{Pattern: "POST /api/teams/{team}/rename", Auth: extensions.UI, Handler: a.renameTeam},
		{Pattern: "PUT /api/teams/{team}", Auth: extensions.UI, Handler: a.editTeam},
		{Pattern: "POST /api/teams/{team}/template", Auth: extensions.UI, Handler: a.saveTemplate},
		{Pattern: "POST /api/teams/{team}/link", Auth: extensions.UI, Handler: a.linkTeam},
		{Pattern: "DELETE /api/teams/templates/{id}", Auth: extensions.UI, Handler: a.removeTemplate},
		{Pattern: "GET /api/teams/templates/{id}/export", Auth: extensions.UI, Handler: a.exportTemplate},
		{Pattern: "POST /api/teams/templates/import", Auth: extensions.UI, Handler: a.importTemplate},
		{Pattern: "POST /api/agents", Auth: extensions.UI, Handler: a.createAgent},
		{Pattern: "PUT /api/agents/{name}", Auth: extensions.UI, Handler: a.changeAgent},
		{Pattern: "DELETE /api/agents/{name}", Auth: extensions.UI, Handler: a.removeAgent},
		{Pattern: "POST /api/agents/{name}/rename", Auth: extensions.UI, Handler: a.renameAgent},
		{Pattern: "PUT /api/agents/{name}/knowledge", Auth: extensions.UI, Handler: a.setKnowledge},
		{Pattern: "GET /api/agents/{name}/knowledge", Auth: extensions.UI, Handler: a.knowledgeHistory},
	}
}

func (a API) listAgents(w http.ResponseWriter, r *http.Request) {
	roster, err := a.Roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	var inForce *policy.Policy
	if a.Policy != nil {
		inForce = a.Policy.Current()
	}
	httpapi.Reply(w, a.agentsOf(roster, agents.BasesIn(a.State), inForce))
}

// agentView is an agent with the profile that judges its calls, when there
// is a policy, and the built-in template its template comes from.
type agentView struct {
	agents.Agent
	Base        string `json:"base,omitempty"` // the built-in template its Template resolves to, live or removed
	Effective   any    `json:"effective,omitempty"`
	TeamVersion string `json:"teamVersion"` // its team's, which PUT /api/teams/{team} checks
}

func (a API) agentsOf(roster agents.Roster, bases agents.Bases, p *policy.Policy) []agentView {
	list := []agentView{}
	versions := map[string]string{}
	for _, team := range roster.Teams() {
		versions[team] = roster.TeamVersion(team)
	}
	for _, agent := range roster {
		if agent.Workers == nil {
			agent.Workers = []string{} // the page reads a list, never null
		}
		view := agentView{Agent: agent, Base: bases.Of(agent.Template), TeamVersion: versions[agent.TeamName()]}
		if p != nil {
			view.Effective = a.EffectiveOf(p, agent.Profile)
		}
		list = append(list, view)
	}
	return list
}
