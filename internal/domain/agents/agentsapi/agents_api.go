package agentsapi

import (
	"cmp"
	"net/http"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
)

// Adding, changing and removing agents from the page. Each is an event, so
// the rules in package rules check it like any other: a new name, a known
// engine, a prompt, known workers, and always a coordinator left. The
// policy's profiles are checked here, against the file in force.

func (a API) createAgent(w http.ResponseWriter, r *http.Request) {
	agent, ok := a.agentFrom(w, r)
	if !ok {
		return
	}
	httpapi.Propose(w, a.Log, agent.Proposal(httpapi.You, false))
}

func (a API) changeAgent(w http.ResponseWriter, r *http.Request) {
	agent, ok := a.agentFrom(w, r)
	if !ok {
		return
	}
	agent.Name = r.PathValue("name") // the name is who the agent is; it doesn't change
	if had, found := a.State.Roster().Get(agent.Name); found {
		agent.Template = had.Template // what it was made from stays, so its team keeps its extensions
	}
	httpapi.Propose(w, a.Log, agent.Proposal(httpapi.You, true))
}

// renameAgent gives an agent a new name, keeping everything else, and tells
// every task that names it, as its coordinator or as who it is handed to, so
// work goes on with it. What has happened keeps the name it had.
func (a API) renameAgent(w http.ResponseWriter, r *http.Request) {
	var request struct{ To string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	name, to := r.PathValue("name"), strings.TrimSpace(request.To)
	if !httpapi.ProposeOrRefuse(w, a.Log, agents.Agent{Name: name}.Renaming(httpapi.You, to)) {
		return
	}
	for _, task := range a.State.Tasks() {
		if task.Coordinator != name && (task.Handoff == nil || task.Handoff.To != name) {
			continue
		}
		if !httpapi.ProposeOrRefuse(w, a.Log, events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.AgentRenamedInTask,
			Data: map[string]any{"name": name, "to": to}}) {
			return
		}
	}
	httpapi.Reply(w, map[string]string{"name": to})
}

// removeAgent removes an agent no unfinished task needs: one that isn't
// working on a task, about to, or coordinating one.
func (a API) removeAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if busy, err := Needing(a.State, name); err != nil {
		httpapi.Failed(w, err)
		return
	} else if len(busy) > 0 {
		http.Error(w, name+" is still needed by "+strings.Join(busy, ", ")+": finish those first (dragging a task to Done on the board finishes it as it is)", http.StatusConflict)
		return
	}
	httpapi.Propose(w, a.Log, agents.Agent{Name: name}.Removal(httpapi.You))
}

func (a API) agentFrom(w http.ResponseWriter, r *http.Request) (agents.Agent, bool) {
	var agent agents.Agent
	if !httpapi.Decode(w, r, &agent) {
		return agent, false
	}
	agent.Name, agent.Model = strings.TrimSpace(agent.Name), strings.TrimSpace(agent.Model)
	agent.Template = "" // only adding a team makes an agent from a template
	agent.Description, agent.Prompt = strings.TrimSpace(agent.Description), strings.TrimSpace(agent.Prompt)
	team := cmp.Or(strings.TrimSpace(agent.Team), agents.DefaultTeam) // a blank team is the default one, however it is spelled now
	switch named := a.State.Roster().TeamNamed(team); {
	case named != "" && named != agents.DefaultTeam:
		agent.Team = named // a team named in another case is that team, not a twin of it; Software respelled too
	case strings.EqualFold(team, agents.DefaultTeam):
		agent.Team = "" // the default team, as agents from before teams have it
	default:
		agent.Team = team
	}
	if problem := a.unknownProfile(agent.Profile); problem != "" {
		http.Error(w, problem, http.StatusBadRequest)
		return agent, false
	}
	return agent, true
}
