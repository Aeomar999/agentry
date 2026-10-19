package web

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/policy"
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
	for _, task := range s.state.Tasks() {
		if task.Coordinator != name && (task.Handoff == nil || task.Handoff.To != name) {
			continue
		}
		if !s.proposeOrRefuse(w, events.Proposal{Stream: task.ID, Actor: you, Type: events.AgentRenamedInTask,
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
// team always has one, then its agents, each with its own prompt. When the
// policy lacks a profile the team needs, it refuses with the profiles
// missing, unless asked to add them first (addMissingProfiles), which it
// does in one change of the policy. hash is the version of the policy
// the page saw, if it saw one.
func (s *Server) addTeam(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Template           string
		AddMissingProfiles bool
		Hash               string
	}
	if !decode(w, r, &request) {
		return
	}
	template, ok := agents.TemplateNamed(request.Template)
	if !ok {
		http.Error(w, "no team template called "+request.Template, http.StatusNotFound)
		return
	}
	if missing := s.missingProfiles(template); len(missing) > 0 && !request.AddMissingProfiles {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]any{"missingProfiles": missing,
			"error": "the " + template.Team + " team needs " + profilesNamed(missing) + ", which policy.toml doesn't have: add it on the Policy page, or add the team with addMissingProfiles"})
		return
	} else if len(missing) > 0 && !s.addProfilesFor(w, template.Team, missing, request.Hash) {
		return
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

// missingProfiles are the profiles the team's agents need that the policy
// lacks, in the template's order.
func (s *Server) missingProfiles(template agents.Template) []string {
	var missing []string
	for _, a := range template.Agents {
		if s.unknownProfile(a.Profile) != "" && !slices.Contains(missing, a.Profile) {
			missing = append(missing, a.Profile)
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
