package agentsapi

import (
	"cmp"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// editTeam saves a team, named in any case, as the team panel edited it, in
// one checked request: {version, team, agents: [{was, name, ...}],
// addMissingProfiles, hash}. version is the team's as the panel loaded it
// (teamVersion in GET /api/agents); team its name after; agents every agent
// it keeps or adds, each with the name it was, and the team's agents left
// out are removed. Every problem is listed at once, and the team is saved
// whole or not at all. Then its unfinished tasks follow: those naming an
// agent by a name it had take its new one, and those another agent of the
// team coordinates, or did until the team removed it, move to its
// coordinator. Saving again with the version the reply gives, a refusal's
// too, finishes those a refusal cut short.
func (a API) editTeam(writer http.ResponseWriter, r *http.Request) {
	w := &jsonErrors{ResponseWriter: writer}
	var request struct {
		Version            string
		Team               string
		Agents             []agents.EditedAgent
		AddMissingProfiles bool
		Hash               string
	}
	if !httpapi.Decode(w, r, &request) {
		return
	}
	roster, err := a.Roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	team := roster.TeamNamed(r.PathValue("team"))
	if team == "" {
		http.Error(w, "there is no team called "+r.PathValue("team"), http.StatusNotFound)
		return
	}
	if version := roster.TeamVersion(team); request.Version != version {
		replyWith(w, http.StatusConflict, map[string]string{"error": "changed elsewhere", "version": version,
			"message": "the " + team + " team changed since you opened it: reload it, or save your changes over it with this version"})
		return
	}
	edit := agents.EditTeam(roster, team, cmp.Or(strings.TrimSpace(request.Team), team), request.Agents)
	edit.Version = request.Version
	missing, ok := a.mayEditTeam(w, edit)
	if !ok {
		return
	}
	stream, err := a.Events.Stream(events.AgentsStream)
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	p := edit.Proposal(httpapi.You)
	after, renames, removals := roster.Next(events.Event{Proposal: p}), agents.RenamesIn(stream), agents.RemovalsIn(stream)
	renames.Apply(events.Event{Proposal: p})
	removals.Apply(events.Event{Proposal: p})
	if busy, involved := a.teamAtWork(roster, after, edit, renames, removals); len(busy) > 0 {
		http.Error(w, "the "+team+" team can't be saved while "+strings.Join(busy, ", ")+" has "+strings.Join(involved, " or ")+
			" at work, as its tasks would move: try again once that turn ends", http.StatusConflict)
		return
	}
	if !a.hasProfiles(w, edit.To, missing, request.AddMissingProfiles, request.Hash) {
		return
	}
	// An edit that changes nothing, as saving again to finish the tasks'
	// follow-ups, writes no event.
	if after.TeamVersion(edit.To) != roster.TeamVersion(team) && !httpapi.ProposeOrRefuse(w, a.Log, p) {
		return
	}
	if roster, err = a.Roster(); err != nil {
		httpapi.Failed(w, err)
		return
	}
	left, why, status := a.followUp(a.renamedInTasks(roster, edit.To, renames))
	if left == nil {
		left, why, status = a.followUp(a.coordinatorMoves(roster, removals, edit.To))
	} else {
		left = append(left, a.coordinatorMoves(roster, removals, edit.To)...) // the moves still to come count too
	}
	version := a.State.Roster().TeamVersion(edit.To)
	if left != nil {
		replyWith(w, status, map[string]string{"error": cutShort("saved the "+edit.To+" team", left, "save again", why), "version": version})
		return
	}
	httpapi.Reply(w, map[string]string{"team": edit.To, "version": version})
}

// teamProblem is one of a team edit's problems as the page reads it: an
// agent's, with its row in the request and its field, or the team's.
type teamProblem struct {
	Row   *int   `json:"row,omitempty"`
	Field string `json:"field"`
	Agent string `json:"agent,omitempty"` // with "removed", the agent a task still needs
	Error string `json:"error"`
}

// mayEditTeam says whether the edit passes every check, listing every
// problem when it doesn't: 409 when the only ones are agents removed that a
// task still needs, else 400. When it says no, it has replied. Otherwise it
// gives the profiles policy.toml lacks that Agentry ships, to offer, or add
// when asked, as for a new team.
func (a API) mayEditTeam(w http.ResponseWriter, edit agents.TeamEdit) ([]string, bool) {
	var problems []teamProblem
	conflict := true
	for _, problem := range rules.TeamEditProblems(a.State, edit.Proposal(httpapi.You)) {
		problems = append(problems, teamProblem{Row: rowOf(problem.Row), Field: problem.Field, Agent: problem.Agent, Error: problem.Reason})
		conflict = conflict && problem.Field == "removed"
	}
	members := make([]agents.Agent, len(edit.Agents))
	for i, e := range edit.Agents {
		members[i] = agents.Agent{Name: e.Name, Profile: e.Profile}
	}
	missing := a.missingProfiles(members)
	for row, member := range members {
		if problem := unshippedProfile([]agents.Agent{member}, missing); problem != "" {
			problems = append(problems, teamProblem{Row: rowOf(row), Field: "profile", Error: problem})
			conflict = false
		}
	}
	if len(problems) > 0 {
		var reasons []string
		for _, problem := range problems {
			reasons = append(reasons, problem.Error)
		}
		status := http.StatusBadRequest
		if conflict {
			status = http.StatusConflict
		}
		replyWith(w, status, map[string]any{"problems": problems,
			"error": "the " + edit.To + " team can't be saved: " + strings.Join(reasons, "; ")})
		return nil, false
	}
	return missing, true
}

// rowOf is a problem's row as the page reads it: none for the team's.
func rowOf(row int) *int {
	if row < 0 {
		return nil
	}
	return &row
}

// teamAtWork lists the unfinished tasks that have at work an agent the edit
// renames, or whose tasks it would move, or the agents that coordinate the
// team before and after it, when that changes, with those agents: a turn
// under way goes on as the agent it started as.
func (a API) teamAtWork(before, after agents.Roster, edit agents.TeamEdit, renames agents.Renames, removals agents.Removals) (busy, involved []string) {
	for _, p := range a.renamedInTasks(after, edit.To, renames) {
		involved = append(involved, p.Text("name"))
	}
	for _, e := range edit.Agents {
		if e.Was != "" && e.Was != e.Name {
			involved = append(involved, e.Was)
		}
	}
	lead, _ := after.Coordinator(edit.To)
	was := ""
	for _, e := range edit.Agents {
		if e.Name == lead.Name {
			was = e.Was
		}
	}
	for _, task := range a.coordinatedByTeam(before, removals, edit.Team, was) {
		involved = append(involved, task.Coordinator)
	}
	if had, _ := before.Coordinator(edit.Team); had.Name != was {
		involved = append(involved, had.Name, cmp.Or(was, lead.Name))
	}
	slices.Sort(involved)
	involved = slices.Compact(involved)
	return a.atWork(involved...), involved
}

// renamedInTasks are the task.agent_renamed that let the unfinished tasks
// naming an agent of team by a name it had go on with the name it has in
// roster.
func (a API) renamedInTasks(roster agents.Roster, team string, renames agents.Renames) []events.Proposal {
	var renamed []events.Proposal
	for _, task := range a.State.Tasks() {
		named := []string{task.Coordinator}
		if task.Handoff != nil && task.Handoff.To != task.Coordinator {
			named = append(named, task.Handoff.To)
		}
		for _, name := range named {
			_, exists := roster.Get(name)
			now, ok := roster.Get(renames[name])
			if !task.IsDone && !exists && ok && now.TeamName() == team {
				renamed = append(renamed, events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.AgentRenamedInTask,
					Data: map[string]any{"name": name, "to": now.Name}})
			}
		}
	}
	return renamed
}

// coordinatorMoves are the task.coordinator_moved that hand the team's
// coordinator the unfinished tasks another agent of the team coordinates,
// or one it removed.
func (a API) coordinatorMoves(roster agents.Roster, removals agents.Removals, team string) []events.Proposal {
	lead, ok := roster.Coordinator(team)
	if !ok {
		return nil
	}
	var moves []events.Proposal
	for _, task := range a.coordinatedByTeam(roster, removals, team, lead.Name) {
		moves = append(moves, events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.CoordinatorMoved,
			Data: map[string]any{"from": task.Coordinator, "to": lead.Name}})
	}
	return moves
}

// followUp proposes the task events that let tasks go on after a change to
// their team, saved already. When one is refused, it gives those it didn't
// reach, with why and the status that says so.
func (a API) followUp(proposals []events.Proposal) (left []events.Proposal, why string, status int) {
	for i, p := range proposals {
		switch _, rejection, err := a.Log.Propose(p); {
		case err != nil:
			return proposals[i:], p.Stream + ": " + err.Error(), http.StatusInternalServerError
		case rejection != nil:
			return proposals[i:], p.Stream + ": " + rejection.Reason, http.StatusConflict
		}
	}
	return nil, "", 0
}

// cutShort says what was saved, how many tasks the follow-ups left still
// need moving, and what finishes them, after one was refused for why.
func cutShort(saved string, left []events.Proposal, again, why string) string {
	var streams []string
	for _, p := range left {
		if !slices.Contains(streams, p.Stream) {
			streams = append(streams, p.Stream)
		}
	}
	tasks := strconv.Itoa(len(streams)) + " tasks still need"
	if len(streams) == 1 {
		tasks = "1 task still needs"
	}
	return saved + ", but " + tasks + " moving: " + again + " to finish (" + why + ")"
}

// coordinatedByTeam lists the unfinished tasks an agent of the team other
// than to coordinates, or one the team removed: all of them before a move,
// and after one cut short, those it didn't reach.
func (a API) coordinatedByTeam(roster agents.Roster, removals agents.Removals, team, to string) []*tasks.Task {
	var moving []*tasks.Task
	for _, task := range a.State.Tasks() {
		lead, ok := roster.Get(task.Coordinator)
		if !task.IsDone && task.Coordinator != to && (ok && lead.TeamName() == team || !ok && removals[task.Coordinator] == team) {
			moving = append(moving, task)
		}
	}
	return moving
}
