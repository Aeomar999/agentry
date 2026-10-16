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
