package web

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
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
	s.propose(w, agent.Proposal(you, false))
}

func (s *Server) changeAgent(w http.ResponseWriter, r *http.Request) {
	agent, ok := s.agentFrom(w, r)
	if !ok {
		return
	}
	agent.Name = r.PathValue("name") // the name is who the agent is; it doesn't change
	s.propose(w, agent.Proposal(you, true))
}

// renameAgent gives an agent a new name, keeping everything else, and tells
// every task that names it, as its coordinator or as who it is handed to, so
// work goes on with it. What has happened keeps the name it had.
func (s *Server) renameAgent(w http.ResponseWriter, r *http.Request) {
	var request struct{ To string }
	if !decode(w, r, &request) {
		return
	}
	name, to := r.PathValue("name"), strings.TrimSpace(request.To)
	if !s.proposeOrRefuse(w, agents.Agent{Name: name}.Renaming(you, to)) {
		return
	}
	ids, err := s.store.TaskIDs()
	if err != nil {
		failed(w, err)
		return
	}
	for _, id := range ids {
		task, err := s.task(id)
		if err != nil || task == nil || (task.Coordinator != name && (task.Handoff == nil || task.Handoff.To != name)) {
			continue
		}
		if !s.proposeOrRefuse(w, events.Proposal{Stream: id, Actor: you, Type: events.AgentRenamedInTask,
			Data: map[string]any{"name": name, "to": to}}) {
			return
		}
	}
	reply(w, map[string]string{"name": to})
}

// removeAgent removes an agent no unfinished task needs: one that isn't
// working on a task, about to, or coordinating one.
func (s *Server) removeAgent(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if busy, err := s.needing(name); err != nil {
		failed(w, err)
		return
	} else if len(busy) > 0 {
		http.Error(w, name+" is still needed by "+strings.Join(busy, ", ")+": finish those first (dragging a task to Done on the board finishes it as it is)", http.StatusConflict)
		return
	}
	s.propose(w, agents.Agent{Name: name}.Removal(you))
}

// removeTeam removes a team and all its agents, its coordinator last, once
// no unfinished task needs any of them. The last team stays: every task
// needs a coordinator.
func (s *Server) removeTeam(w http.ResponseWriter, r *http.Request) {
	roster, err := s.roster()
	if err != nil {
		failed(w, err)
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
		failed(w, err)
		return
	} else if len(busy) > 0 {
		http.Error(w, "the "+team+" team is still needed by "+strings.Join(busy, ", ")+": finish those first (dragging a task to Done on the board finishes it as it is)",
			http.StatusConflict)
		return
	}
	for _, agent := range everyone {
		if !s.proposeOrRefuse(w, agent.Removal(you)) {
			return
		}
	}
	reply(w, map[string]any{"team": team, "removed": names})
}

// needing lists the unfinished tasks that one of the agents coordinates,
// works on now, or is about to.
func (s *Server) needing(names ...string) ([]string, error) {
	ids, err := s.store.TaskIDs()
	if err != nil {
		return nil, err
	}
	var busy []string
	for _, id := range ids {
		task, err := s.task(id)
		if err != nil || task == nil || task.IsDone {
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

// teamTemplates are the ready-made teams you can add, without their prompts.
func (s *Server) teamTemplates(w http.ResponseWriter, _ *http.Request) {
	type member struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Coordinates bool   `json:"coordinates"`
	}
	type template struct {
		Team        string   `json:"team"`
		Description string   `json:"description"`
		Agents      []member `json:"agents"`
	}
	list := []template{}
	for _, t := range agents.Templates {
		view := template{Team: t.Team, Description: t.Description}
		for _, a := range t.Agents {
			view.Agents = append(view.Agents, member{Name: a.Name, Description: a.Description, Coordinates: a.Coordinates})
		}
		list = append(list, view)
	}
	reply(w, list)
}

// addTeam adds a team made from a template: its coordinator first, so the
// team always has one, then its agents, each with its own prompt.
func (s *Server) addTeam(w http.ResponseWriter, r *http.Request) {
	var request struct{ Template string }
	if !decode(w, r, &request) {
		return
	}
	template, ok := agents.TemplateNamed(request.Template)
	if !ok {
		http.Error(w, "no team template called "+request.Template, http.StatusNotFound)
		return
	}
	for _, a := range template.Agents {
		if problem := s.unknownProfile(a.Profile); problem != "" {
			http.Error(w, problem+". Copy [profiles."+a.Profile+"] from internal/policy/default.toml, or the README's Profiles section, into that file, then add the team.", http.StatusBadRequest)
			return
		}
	}
	roster, err := s.roster()
	if err != nil {
		failed(w, err)
		return
	}
	proposals := agents.AddTeam(template, roster, you)
	for _, p := range proposals[:len(proposals)-1] {
		if !s.proposeOrRefuse(w, p) {
			return
		}
	}
	s.propose(w, proposals[len(proposals)-1])
}

func (s *Server) agentFrom(w http.ResponseWriter, r *http.Request) (agents.Agent, bool) {
	var agent agents.Agent
	if !decode(w, r, &agent) {
		return agent, false
	}
	agent.Name, agent.Model = strings.TrimSpace(agent.Name), strings.TrimSpace(agent.Model)
	agent.Description, agent.Prompt = strings.TrimSpace(agent.Description), strings.TrimSpace(agent.Prompt)
	if agent.Team = strings.TrimSpace(agent.Team); strings.EqualFold(agent.Team, agents.DefaultTeam) {
		agent.Team = "" // the default team, as agents from before teams have it
	}
	if problem := s.unknownProfile(agent.Profile); problem != "" {
		http.Error(w, problem, http.StatusBadRequest)
		return agent, false
	}
	return agent, true
}

// unknownProfile says why an agent can't have profile, or "" when the policy
// has it. A policy written before a profile was added doesn't have it.
func (s *Server) unknownProfile(profile string) string {
	if profile == "" || s.config.Policy == nil || slices.Contains(s.config.Policy.Current().ProfileNames(), profile) {
		return ""
	}
	return "no profile called " + profile + " in " + s.config.Policy.Path()
}

// setKnowledge replaces the agent's knowledge with yours.
func (s *Server) setKnowledge(w http.ResponseWriter, r *http.Request) {
	var request struct{ Knowledge string }
	if !decode(w, r, &request) {
		return
	}
	s.propose(w, agents.Agent{Name: r.PathValue("name")}.Learned(you, strings.TrimSpace(request.Knowledge)))
}

// knowledgeHistory is every change to the agent's knowledge, newest first:
// by the agent as it worked, or by you.
func (s *Server) knowledgeHistory(w http.ResponseWriter, r *http.Request) {
	stream, err := s.store.Stream(events.AgentsStream)
	if err != nil {
		failed(w, err)
		return
	}
	type change struct {
		By        string    `json:"by"`
		At        time.Time `json:"at"`
		Knowledge string    `json:"knowledge"`
	}
	history := []change{}
	for i := len(stream) - 1; i >= 0; i-- {
		if e := stream[i]; e.Type == events.AgentLearned && e.Text("name") == r.PathValue("name") {
			history = append(history, change{By: e.Actor.ID, At: e.At, Knowledge: e.Text("knowledge")})
		}
	}
	reply(w, history)
}
