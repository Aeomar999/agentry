package web

import (
	"net/http"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
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

func (s *Server) removeAgent(w http.ResponseWriter, r *http.Request) {
	s.propose(w, agents.Agent{Name: r.PathValue("name")}.Removal(you))
}

func (s *Server) agentFrom(w http.ResponseWriter, r *http.Request) (agents.Agent, bool) {
	var agent agents.Agent
	if !decode(w, r, &agent) {
		return agent, false
	}
	agent.Name, agent.Model = strings.TrimSpace(agent.Name), strings.TrimSpace(agent.Model)
	agent.Description, agent.Prompt = strings.TrimSpace(agent.Description), strings.TrimSpace(agent.Prompt)
	if agent.Profile != "" && s.config.Policy != nil && !slices.Contains(s.config.Policy.Current().ProfileNames(), agent.Profile) {
		http.Error(w, "no profile called "+agent.Profile+" in "+s.config.Policy.Path(), http.StatusBadRequest)
		return agent, false
	}
	return agent, true
}
