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
		if task.IsDone {
			continue
		}
		open := task.OpenTurn()
		for _, name := range names {
			if task.Coordinator == name || open != nil && open.Agent == name || task.Handoff != nil && task.Handoff.To == name {
				busy = append(busy, task.ID)
				break
			}
		}
	}
	return busy, nil
}

// teamTemplates are the teams you can add, without their prompts: the
// ready-made ones, then yours in the order you saved them.
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

// templateView is a team template as the page lists it, without its prompts.
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
}

func viewOf(t agents.Template, userMade bool) templateView {
	view := templateView{ID: t.ID, Team: t.Team, Description: t.Description, Features: t.Features, UserMade: userMade}
	if view.Features == nil {
		view.Features = []string{}
	}
	for _, a := range t.Agents {
		view.Agents = append(view.Agents, memberView{Name: a.Name, Description: a.Description, Coordinates: a.Coordinates})
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
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusConflict)
	json.NewEncoder(w).Encode(map[string]any{"missingProfiles": missing,
		"error": "the " + team + " team needs " + profilesNamed(missing) + ", which policy.toml doesn't have: add it on the Policy page"})
	return false
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
