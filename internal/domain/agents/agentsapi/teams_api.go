package agentsapi

import (
	"net/http"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
)

// renameTeam gives a team, named in any case, a new name: every agent of it
// moves there in one event. Its open tasks go on as they were, as a task
// names its coordinator, not its team, and the coordinator keeps its name.
func (a API) renameTeam(w http.ResponseWriter, r *http.Request) {
	var request struct{ To string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	roster, err := a.Roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	team, to := roster.TeamNamed(r.PathValue("team")), strings.TrimSpace(request.To)
	if team == "" {
		http.Error(w, "there is no team called "+r.PathValue("team"), http.StatusNotFound)
		return
	}
	if !httpapi.ProposeOrRefuse(w, a.Log, agents.TeamRenaming(team, to, httpapi.You)) {
		return
	}
	httpapi.Reply(w, map[string]string{"team": team, "to": to})
}

// removeTeam removes a team and all its agents, its coordinator last, once
// no unfinished task needs any of them. The last team stays: every task
// needs a coordinator.
func (a API) removeTeam(w http.ResponseWriter, r *http.Request) {
	roster, err := a.Roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	team := r.PathValue("team")
	var members, leads []agents.Agent
	for _, agent := range roster {
		switch {
		case !strings.EqualFold(agent.TeamName(), team):
		case agent.Coordinates:
			leads = append(leads, agent)
		default:
			members = append(members, agent)
		}
	}
	switch {
	case len(members)+len(leads) == 0:
		http.Error(w, "there is no team called "+team, http.StatusNotFound)
		return
	case len(roster.Teams()) == 1:
		http.Error(w, "the "+team+" team is the only one, and every task needs a coordinator: add another team first", http.StatusConflict)
		return
	}
	everyone := append(members, leads...)
	var names []string
	for _, agent := range everyone {
		names = append(names, agent.Name)
	}
	if busy, err := Needing(a.State, names...); err != nil {
		httpapi.Failed(w, err)
		return
	} else if len(busy) > 0 {
		http.Error(w, "the "+team+" team is still needed by "+strings.Join(busy, ", ")+": finish those first (dragging a task to Done on the board finishes it as it is)",
			http.StatusConflict)
		return
	}
	for _, agent := range everyone {
		if !httpapi.ProposeOrRefuse(w, a.Log, agent.Removal(httpapi.You)) {
			return
		}
	}
	httpapi.Reply(w, map[string]any{"team": team, "removed": names})
}
