package web

import (
	"cmp"
	"encoding/json"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// Adding, changing and removing agents from the page. Each is an event, so
// the rules in package rules check it like any other: a new name, a known
// engine, a prompt, known workers, and always a coordinator left. The
// policy's profiles are checked here, against the file in force.

func (s *Server) createAgent(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.agentFrom(w, r)
	if !ok {
		return
	}
	httpapi.Propose(w, s.store, agent.Proposal(httpapi.You, false))
}

func (s *Server) changeAgent(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.agentFrom(w, r)
	if !ok {
		return
	}
	agent.Name = r.PathValue("name") // the name is who the agent is; it doesn't change
	if had, found := s.state.Roster().Get(agent.Name); found {
		agent.Template = had.Template // what it was made from stays, so its team keeps its features
	}
	httpapi.Propose(w, s.store, agent.Proposal(httpapi.You, true))
}

// renameAgent gives an agent a new name, keeping everything else, and tells
// every task that names it, as its coordinator or as who it is handed to, so
// work goes on with it. What has happened keeps the name it had.
func (s *Server) renameAgent(w http.ResponseWriter, r *http.Request) {
	var request struct{ To string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	name, to := r.PathValue("name"), strings.TrimSpace(request.To)
	if !httpapi.ProposeOrRefuse(w, s.store, agents.Agent{Name: name}.Renaming(httpapi.You, to)) {
		return
	}
	for _, task := range s.state.Tasks() {
		if task.Coordinator != name && (task.Handoff == nil || task.Handoff.To != name) {
			continue
		}
		if !httpapi.ProposeOrRefuse(w, s.store, events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.AgentRenamedInTask,
			Data: map[string]any{"name": name, "to": to}}) {
			return
		}
	}
	httpapi.Reply(w, map[string]string{"name": to})
}

// removeAgent removes an agent no unfinished task needs: one that isn't
// working on a task, about to, or coordinating one.
func (s *Server) removeAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if busy, err := s.needing(name); err != nil {
		httpapi.Failed(w, err)
		return
	} else if len(busy) > 0 {
		http.Error(w, name+" is still needed by "+strings.Join(busy, ", ")+": finish those first (dragging a task to Done on the board finishes it as it is)", http.StatusConflict)
		return
	}
	httpapi.Propose(w, s.store, agents.Agent{Name: name}.Removal(httpapi.You))
}

// renameTeam gives a team, named in any case, a new name: every agent of it
// moves there in one event. Its open tasks go on as they were, as a task
// names its coordinator, not its team, and the coordinator keeps its name.
func (s *Server) renameTeam(w http.ResponseWriter, r *http.Request) {
	var request struct{ To string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	roster, err := s.roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	team, to := roster.TeamNamed(r.PathValue("team")), strings.TrimSpace(request.To)
	if team == "" {
		http.Error(w, "there is no team called "+r.PathValue("team"), http.StatusNotFound)
		return
	}
	if !httpapi.ProposeOrRefuse(w, s.store, agents.TeamRenaming(team, to, httpapi.You)) {
		return
	}
	httpapi.Reply(w, map[string]string{"team": team, "to": to})
}

// moveCoordinator makes another agent of a team, named in any case, its
// coordinator, and hands it the team's open tasks; finished ones keep the
// coordinator they had. Asked again, it hands over the open tasks a move cut
// short left with another agent of the team. It waits while an agent whose
// tasks move, or the new coordinator, is at work on a task, as a turn under
// way goes on as the agent it started as.
func (s *Server) moveCoordinator(w http.ResponseWriter, r *http.Request) {
	var request struct{ Agent string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	roster, err := s.roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	team, to := roster.TeamNamed(r.PathValue("team")), strings.TrimSpace(request.Agent)
	if team == "" {
		http.Error(w, "there is no team called "+r.PathValue("team"), http.StatusNotFound)
		return
	}
	agent, ok := roster.Get(to)
	if !ok {
		http.Error(w, "no agent called "+to, http.StatusNotFound)
		return
	}
	var involved []string // an old log can have left a team two coordinators
	for _, other := range roster {
		if other.Coordinates && other.TeamName() == team && other.Name != to {
			involved = append(involved, other.Name)
		}
	}
	moving := s.coordinatedByTeam(roster, team, to)
	for _, task := range moving {
		if !slices.Contains(involved, task.Coordinator) {
			involved = append(involved, task.Coordinator)
		}
	}
	involved = append(involved, to)
	move := agents.CoordinatorMoving(team, to, httpapi.You)
	already := agent.Coordinates && agent.TeamName() == team
	if !already || len(moving) == 0 {
		if reason := rules.CoordinatorMoveRefusal(roster, move); reason != "" {
			http.Error(w, reason, http.StatusConflict)
			return
		}
	}
	if busy := s.atWork(involved...); len(busy) > 0 {
		http.Error(w, "the "+team+" team's coordinator can't move while "+strings.Join(busy, ", ")+" has "+strings.Join(involved, " or ")+" at work: try again once that turn ends",
			http.StatusConflict)
		return
	}
	if !already && !httpapi.ProposeOrRefuse(w, s.store, move) {
		return
	}
	if !s.followUp(w, s.coordinatorMoves(s.state.Roster(), team), "moved the "+team+" team's coordinator to "+to, "ask again") {
		return
	}
	httpapi.Reply(w, map[string]string{"team": team, "coordinator": to})
}

// editTeam saves a team, named in any case, as the team panel edited it, in
// one checked request: {version, team, agents: [{was, name, ...}],
// addMissingProfiles, hash}. version is the team's as the panel loaded it
// (teamVersion in GET /api/agents); team its name after; agents every agent
// it keeps or adds, each with the name it was, and the team's agents left
// out are removed. Every problem is listed at once, and the team is saved
// whole or not at all. Then its unfinished tasks follow: those naming an
// agent by a name it had take its new one, and those another agent of the
// team coordinates move to its coordinator. Saving again with the version
// the reply gives finishes those a refusal cut short.
func (s *Server) editTeam(writer http.ResponseWriter, r *http.Request) {
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
	roster, err := s.roster()
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
	if !s.mayEditTeam(w, edit, request.AddMissingProfiles, request.Hash) {
		return
	}
	stream, err := s.store.Stream(events.AgentsStream)
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	p := edit.Proposal(httpapi.You)
	after, renames := roster.Next(events.Event{Proposal: p}), agents.RenamesIn(stream)
	renames.Apply(events.Event{Proposal: p})
	if busy, involved := s.teamAtWork(roster, after, edit, renames); len(busy) > 0 {
		http.Error(w, "the "+team+" team can't be saved while "+strings.Join(busy, ", ")+" has "+strings.Join(involved, " or ")+
			" at work, as its tasks would move: try again once that turn ends", http.StatusConflict)
		return
	}
	// An edit that changes nothing, as saving again to finish the tasks'
	// follow-ups, writes no event.
	if after.TeamVersion(edit.To) != roster.TeamVersion(team) && !httpapi.ProposeOrRefuse(w, s.store, p) {
		return
	}
	if roster, err = s.roster(); err != nil {
		httpapi.Failed(w, err)
		return
	}
	saved := "saved the " + edit.To + " team"
	if !s.followUp(w, s.renamedInTasks(roster, edit.To, renames), saved, "save again") ||
		!s.followUp(w, s.coordinatorMoves(roster, edit.To), saved, "save again") {
		return
	}
	httpapi.Reply(w, map[string]string{"team": edit.To, "version": s.state.Roster().TeamVersion(edit.To)})
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
// task still needs, else 400. A missing profile Agentry ships is offered,
// or added when asked, as for a new team. When it says no, it has replied.
func (s *Server) mayEditTeam(w http.ResponseWriter, edit agents.TeamEdit, addMissing bool, hash string) bool {
	var problems []teamProblem
	conflict := true
	for _, problem := range rules.TeamEditProblems(s.state, edit.Proposal(httpapi.You)) {
		problems = append(problems, teamProblem{Row: rowOf(problem.Row), Field: problem.Field, Agent: problem.Agent, Error: problem.Reason})
		conflict = conflict && problem.Field == "removed"
	}
	members := make([]agents.Agent, len(edit.Agents))
	for i, e := range edit.Agents {
		members[i] = agents.Agent{Name: e.Name, Profile: e.Profile}
	}
	missing := s.missingProfiles(members)
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
		return false
	}
	return s.hasProfiles(w, edit.To, missing, addMissing, hash)
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
func (s *Server) teamAtWork(before, after agents.Roster, edit agents.TeamEdit, renames agents.Renames) (busy, involved []string) {
	for _, p := range s.renamedInTasks(after, edit.To, renames) {
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
	for _, task := range s.coordinatedByTeam(before, edit.Team, was) {
		involved = append(involved, task.Coordinator)
	}
	if had, _ := before.Coordinator(edit.Team); had.Name != was {
		involved = append(involved, had.Name, cmp.Or(was, lead.Name))
	}
	slices.Sort(involved)
	involved = slices.Compact(involved)
	return s.atWork(involved...), involved
}

// renamedInTasks are the task.agent_renamed that let the unfinished tasks
// naming an agent of team by a name it had go on with the name it has in
// roster.
func (s *Server) renamedInTasks(roster agents.Roster, team string, renames agents.Renames) []events.Proposal {
	var renamed []events.Proposal
	for _, task := range s.state.Tasks() {
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
// coordinator the unfinished tasks another agent of the team coordinates.
func (s *Server) coordinatorMoves(roster agents.Roster, team string) []events.Proposal {
	lead, ok := roster.Coordinator(team)
	if !ok {
		return nil
	}
	var moves []events.Proposal
	for _, task := range s.coordinatedByTeam(roster, team, lead.Name) {
		moves = append(moves, events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.CoordinatorMoved,
			Data: map[string]any{"from": task.Coordinator, "to": lead.Name}})
	}
	return moves
}

// followUp proposes the task events that let tasks go on after a change to
// their team, saved already. When one is refused, it replies with what was
// saved, how many tasks still need moving, and what finishes them.
func (s *Server) followUp(w http.ResponseWriter, proposals []events.Proposal, saved, again string) bool {
	for i, p := range proposals {
		_, rejection, err := s.store.Propose(p)
		if err == nil && rejection == nil {
			continue
		}
		var left []string
		for _, q := range proposals[i:] {
			if !slices.Contains(left, q.Stream) {
				left = append(left, q.Stream)
			}
		}
		tasks := strconv.Itoa(len(left)) + " tasks still need"
		if len(left) == 1 {
			tasks = "1 task still needs"
		}
		why, status := "", http.StatusConflict
		if err != nil {
			why, status = err.Error(), http.StatusInternalServerError
		} else {
			why = rejection.Reason
		}
		http.Error(w, saved+", but "+tasks+" moving: "+again+" to finish ("+p.Stream+": "+why+")", status)
		return false
	}
	return true
}

// coordinatedByTeam lists the unfinished tasks an agent of the team other
// than to coordinates: all of them before a move, and after one cut short,
// those it didn't reach.
func (s *Server) coordinatedByTeam(roster agents.Roster, team, to string) []*tasks.Task {
	var moving []*tasks.Task
	for _, task := range s.state.Tasks() {
		if lead, ok := roster.Get(task.Coordinator); ok && !task.IsDone && lead.TeamName() == team && lead.Name != to {
			moving = append(moving, task)
		}
	}
	return moving
}

// removeTeam removes a team and all its agents, its coordinator last, once
// no unfinished task needs any of them. The last team stays: every task
// needs a coordinator.
func (s *Server) removeTeam(w http.ResponseWriter, r *http.Request) {
	roster, err := s.roster()
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
	if busy, err := s.needing(names...); err != nil {
		httpapi.Failed(w, err)
		return
	} else if len(busy) > 0 {
		http.Error(w, "the "+team+" team is still needed by "+strings.Join(busy, ", ")+": finish those first (dragging a task to Done on the board finishes it as it is)",
			http.StatusConflict)
		return
	}
	for _, agent := range everyone {
		if !httpapi.ProposeOrRefuse(w, s.store, agent.Removal(httpapi.You)) {
			return
		}
	}
	httpapi.Reply(w, map[string]any{"team": team, "removed": names})
}

// needing lists the unfinished tasks that one of the agents coordinates,
// works on now, or is about to.
func (s *Server) needing(names ...string) ([]string, error) {
	var busy []string
	for _, task := range s.state.Tasks() {
		if slices.ContainsFunc(names, task.Needs) {
			busy = append(busy, task.ID)
		}
	}
	return busy, nil
}

// atWork lists the unfinished tasks one of the agents works on now, or is
// about to as other than the task's coordinator.
func (s *Server) atWork(names ...string) []string {
	var busy []string
	for _, task := range s.state.Tasks() {
		open, next := task.OpenTurn(), task.Handoff
		switch {
		case task.IsDone:
		case open != nil && slices.Contains(names, open.Agent),
			next != nil && next.To != task.Coordinator && slices.Contains(names, next.To):
			busy = append(busy, task.ID)
		}
	}
	return busy
}

// teamTemplates are the teams you can add, each agent with what a team made
// from it gets, prompt included, so the page can copy one: the ready-made
// ones, then yours in the order you saved them.
func (s *Server) teamTemplates(w http.ResponseWriter, _ *http.Request) {
	list := []templateView{}
	for _, t := range agents.Templates {
		list = append(list, viewOf(t, false))
	}
	for _, u := range s.state.Templates() {
		list = append(list, viewOf(u.Template(), true))
	}
	httpapi.Reply(w, list)
}

// templateView is a team template as the page lists it.
type templateView struct {
	ID          string       `json:"id"` // what GET /api/agents says an agent was made from
	Team        string       `json:"team"`
	Description string       `json:"description"`
	Features    []string     `json:"features"` // what a team made from it turns on, so the page can find that team
	Agents      []memberView `json:"agents"`
	UserMade    bool         `json:"userMade"` // one you saved, not one Agentry ships
}

type memberView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Coordinates bool   `json:"coordinates"`
	Prompt      string `json:"prompt"`
	Engine      string `json:"engine"`
	Model       string `json:"model"`
	Profile     string `json:"profile"`
	Color       string `json:"color"`
	Column      string `json:"column"`
}

func viewOf(t agents.Template, userMade bool) templateView {
	view := templateView{ID: t.ID, Team: t.Team, Description: t.Description, Features: t.Features, UserMade: userMade}
	if view.Features == nil {
		view.Features = []string{}
	}
	for _, a := range t.Prompted() {
		view.Agents = append(view.Agents, memberView{Name: a.Name, Description: a.Description, Coordinates: a.Coordinates,
			Prompt: a.Prompt, Engine: a.Engine, Model: a.Model, Profile: a.Profile, Color: a.Color, Column: a.Column})
	}
	return view
}

// addTeam adds a team, made from a template ({template}: a built-in one's
// team, or the id of one you saved) or from agents you define ({team,
// agents}): its coordinator first, so the team always has
// one, then its agents, each with its own prompt. When the policy lacks a
// profile the team needs, it refuses with the profiles missing, unless asked
// to add them first (addMissingProfiles), which it does in one change of the
// policy. hash is the version of the policy the page saw, if it saw one.
// Every error is JSON {error, missingProfiles?}.
func (s *Server) addTeam(writer http.ResponseWriter, r *http.Request) {
	w := &jsonErrors{ResponseWriter: writer}
	var request struct {
		Template           string
		Team               string
		Agents             []agents.Agent
		AddMissingProfiles bool
		Hash               string
	}
	if !httpapi.Decode(w, r, &request) {
		return
	}
	custom := strings.TrimSpace(request.Team) != "" || len(request.Agents) > 0
	switch {
	case request.Template != "" && custom:
		http.Error(w, "a team comes from a template or from agents you define, not both", http.StatusBadRequest)
		return
	case custom:
		s.addCustomTeam(w, strings.TrimSpace(request.Team), request.Agents, request.AddMissingProfiles, request.Hash)
		return
	case request.Template == "":
		http.Error(w, "name a template, or a team and its agents", http.StatusBadRequest)
		return
	}
	template, ok := agents.TemplateNamed(request.Template)
	if !ok {
		s.addFromYourTemplate(w, request.Template, request.AddMissingProfiles, request.Hash)
		return
	}
	if !s.hasProfiles(w, template.Team, s.missingProfiles(template.Agents), request.AddMissingProfiles, request.Hash) {
		return
	}
	roster, err := s.roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	s.proposeTeam(w, agents.AddTeam(template, roster, httpapi.You))
}

// addFromYourTemplate adds the team of the template you saved as id, under
// the rules a custom team has: a saved template was checked as it was then,
// and the roster and the policy may have changed since. The agents checked
// are those AddTeam makes, with the team name and numbers it picks.
func (s *Server) addFromYourTemplate(w http.ResponseWriter, id string, addMissing bool, hash string) {
	saved, ok := s.state.Templates().WithID(id)
	if !ok {
		http.Error(w, "no team template called "+id, http.StatusNotFound)
		return
	}
	roster, err := s.roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	template := saved.Template()
	made := template.MadeFor(roster)
	if len(made) == 0 { // the rules refuse a template without agents, so only a broken log has one
		http.Error(w, "your template "+id+" has no agents", http.StatusInternalServerError)
		return
	}
	if !s.mayAddTeam(w, made[0].TeamName(), made, roster, addMissing, hash) {
		return
	}
	s.proposeTeam(w, agents.AddTeam(template, roster, httpapi.You))
}

// addCustomTeam adds the team of the agents you define, once every agent
// passes: nothing is proposed before then, so a team is never half made.
func (s *Server) addCustomTeam(w http.ResponseWriter, team string, members []agents.Agent, addMissing bool, hash string) {
	roster, err := s.roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	if !s.mayAddTeam(w, team, members, roster, addMissing, hash) {
		return
	}
	s.proposeTeam(w, agents.CustomTeam(team, members, httpapi.You))
}

// mayAddTeam says whether the team of these members passes every check a
// team you define has, adding the profiles it lacks when asked; when it
// says no, it has replied.
func (s *Server) mayAddTeam(w http.ResponseWriter, team string, members []agents.Agent, roster agents.Roster, addMissing bool, hash string) bool {
	machines, err := s.workers()
	if err != nil {
		httpapi.Failed(w, err)
		return false
	}
	problem := agents.CheckCustomTeam(team, members, roster)
	if problem == "" {
		problem = unknownEngine(members)
	}
	if problem == "" {
		problem = unknownWorker(members, machines)
	}
	missing := s.missingProfiles(members)
	if problem == "" {
		problem = unshippedProfile(members, missing)
	}
	if problem != "" {
		http.Error(w, problem, http.StatusBadRequest)
		return false
	}
	return s.hasProfiles(w, team, missing, addMissing, hash)
}

// unknownEngine says which agent names an engine Agentry can't run, or "".
func unknownEngine(members []agents.Agent) string {
	for _, a := range members {
		if engine := strings.TrimSpace(a.Engine); !slices.Contains(rules.KnownEngines, engine) {
			return strings.TrimSpace(a.Name) + "'s engine must be one of " + strings.Join(rules.KnownEngines, ", ") + ", not " + strconv.Quote(engine)
		}
	}
	return ""
}

// unknownWorker says which agent names a worker that hasn't joined, or "".
func unknownWorker(members []agents.Agent, machines workers.Roster) string {
	for _, a := range members {
		if problem := rules.UnknownWorker(machines, a.Workers); problem != "" {
			return strings.TrimSpace(a.Name) + ": " + problem
		}
	}
	return ""
}

// unshippedProfile says which agent needs a missing profile that Agentry
// doesn't ship, so only you can write it, or "".
func unshippedProfile(members []agents.Agent, missing []string) string {
	for _, a := range members {
		profile := strings.TrimSpace(a.Profile)
		if _, shipped := policy.ShippedProfile(profile); slices.Contains(missing, profile) && !shipped {
			return strings.TrimSpace(a.Name) + "'s profile " + profile + " isn't in policy.toml, and Agentry doesn't ship it: write it in policy.toml, or pick another profile"
		}
	}
	return ""
}

// hasProfiles says whether the policy has every profile the team needs,
// adding the missing ones when asked; when it says no, it has replied.
func (s *Server) hasProfiles(w http.ResponseWriter, team string, missing []string, addMissing bool, hash string) bool {
	if len(missing) == 0 {
		return true
	}
	if addMissing {
		return s.addProfilesFor(w, team, missing, hash)
	}
	replyWith(w, http.StatusConflict, map[string]any{"missingProfiles": missing,
		"error": "the " + team + " team needs " + profilesNamed(missing) + ", which policy.toml doesn't have: add it on the Policy page"})
	return false
}

// replyWith answers status with value as JSON.
func replyWith(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

// proposeTeam proposes the events that add a team, stopping at the first
// one refused.
func (s *Server) proposeTeam(w http.ResponseWriter, proposals []events.Proposal) {
	for _, p := range proposals[:len(proposals)-1] {
		if !httpapi.ProposeOrRefuse(w, s.store, p) {
			return
		}
	}
	httpapi.Propose(w, s.store, proposals[len(proposals)-1])
}

// jsonErrors writes the plain-text errors of http.Error as {"error": ...},
// so a handler built from the shared helpers replies in one shape.
type jsonErrors struct {
	http.ResponseWriter
	failing bool
}

func (w *jsonErrors) WriteHeader(status int) {
	if status >= http.StatusBadRequest && strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
		w.failing = true
		w.Header().Set("Content-Type", "application/json")
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *jsonErrors) Write(text []byte) (int, error) {
	if !w.failing {
		return w.ResponseWriter.Write(text)
	}
	err := json.NewEncoder(w.ResponseWriter).Encode(map[string]string{"error": strings.TrimSpace(string(text))})
	return len(text), err
}

func (s *Server) agentFrom(w http.ResponseWriter, r *http.Request) (agents.Agent, bool) {
	var agent agents.Agent
	if !httpapi.Decode(w, r, &agent) {
		return agent, false
	}
	agent.Name, agent.Model = strings.TrimSpace(agent.Name), strings.TrimSpace(agent.Model)
	agent.Template = "" // only AddTeam makes an agent from a template
	agent.Description, agent.Prompt = strings.TrimSpace(agent.Description), strings.TrimSpace(agent.Prompt)
	team := cmp.Or(strings.TrimSpace(agent.Team), agents.DefaultTeam) // a blank team is the default one, however it is spelled now
	switch named := s.state.Roster().TeamNamed(team); {
	case named != "" && named != agents.DefaultTeam:
		agent.Team = named // a team named in another case is that team, not a twin of it; Software respelled too
	case strings.EqualFold(team, agents.DefaultTeam):
		agent.Team = "" // the default team, as agents from before teams have it
	default:
		agent.Team = team
	}
	if problem := s.unknownProfile(agent.Profile); problem != "" {
		http.Error(w, problem, http.StatusBadRequest)
		return agent, false
	}
	return agent, true
}

// missingProfiles are the profiles these agents of a team need that the
// policy lacks, in the agents' order.
func (s *Server) missingProfiles(members []agents.Agent) []string {
	var missing []string
	for _, a := range members {
		if profile := strings.TrimSpace(a.Profile); s.unknownProfile(profile) != "" && !slices.Contains(missing, profile) {
			missing = append(missing, profile)
		}
	}
	return missing
}

// addProfilesFor adds the default profiles a team needs in one change of
// the policy, or replies why it can't: a profile Agentry doesn't ship is
// yours to write.
func (s *Server) addProfilesFor(w http.ResponseWriter, team string, missing []string, seen string) bool {
	for _, name := range missing {
		if _, shipped := policy.ShippedProfile(name); !shipped {
			http.Error(w, "the "+team+" team needs profile "+name+", which Agentry doesn't ship: write it in policy.toml yourself, then add the team", http.StatusConflict)
			return false
		}
	}
	// "" means the policy in force; Live's text check still refuses a hand edit that isn't reloaded.
	if seen == "" {
		seen = s.config.Policy.Current().Hash()
	}
	_, ok := s.applyPolicy(w, seen, "added "+profilesNamed(missing)+" for team "+team, func(text []byte) ([]byte, error) {
		var err error
		for _, name := range missing {
			if text, err = policy.AddProfile(text, name, s.version()); err != nil {
				return nil, err
			}
		}
		return text, nil
	})
	return ok
}

// profilesNamed is "profile read-mostly", or "profiles a, b".
func profilesNamed(names []string) string {
	if len(names) == 1 {
		return "profile " + names[0]
	}
	return "profiles " + strings.Join(names, ", ")
}

// unknownProfile says why an agent can't have profile, or "" when the policy
// has it. A policy written before a profile was added doesn't have it.
func (s *Server) unknownProfile(profile string) string {
	if profile == "" || s.config.Policy == nil || slices.Contains(s.config.Policy.Current().ProfileNames(), profile) {
		return ""
	}
	if _, shipped := policy.ShippedProfile(profile); shipped {
		return "policy.toml has no profile called " + profile + ": Agentry ships one, so add it on the Policy page, then try again"
	}
	return "policy.toml has no profile called " + profile + ": write it in policy.toml, or pick another profile"
}

// setKnowledge replaces the agent's knowledge with yours.
func (s *Server) setKnowledge(w http.ResponseWriter, r *http.Request) {
	var request struct{ Knowledge string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	httpapi.Propose(w, s.store, agents.Agent{Name: r.PathValue("name")}.Learned(httpapi.You, strings.TrimSpace(request.Knowledge)))
}

// knowledgeHistory is every change to the agent's knowledge, newest first:
// by the agent as it worked, or by you. A change may be a patch on the
// version before, so each version is rebuilt walking forward from the start.
func (s *Server) knowledgeHistory(w http.ResponseWriter, r *http.Request) {
	if _, err := s.roster(); err != nil { // the walk would fail as the state's did
		httpapi.Failed(w, err)
		return
	}
	stream, err := s.store.Stream(events.AgentsStream)
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	type change struct {
		By        string    `json:"by"`
		At        time.Time `json:"at"`
		Knowledge string    `json:"knowledge"`
	}
	history := []change{}
	var roster agents.Roster
	name := r.PathValue("name")
	for _, e := range stream {
		roster.Apply(e)
		if e.Type == events.AgentLearned && e.Text("name") == name {
			agent, _ := roster.Get(name)
			history = append(history, change{By: e.Actor.ID, At: e.At, Knowledge: agent.Knowledge})
		}
	}
	slices.Reverse(history)
	httpapi.Reply(w, history)
}
