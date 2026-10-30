package agentsapi

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
)

// setKnowledge replaces the agent's knowledge with yours.
func (a API) setKnowledge(w http.ResponseWriter, r *http.Request) {
	var request struct{ Knowledge string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	httpapi.Propose(w, a.Log, agents.Agent{Name: r.PathValue("name")}.Learned(httpapi.You, strings.TrimSpace(request.Knowledge)))
}

// knowledgeHistory is every change to the agent's knowledge, newest first:
// by the agent as it worked, or by you. A change may be a patch on the
// version before, so each version is rebuilt walking forward from the start.
func (a API) knowledgeHistory(w http.ResponseWriter, r *http.Request) {
	if _, err := a.Roster(); err != nil { // the walk would fail as the state's did
		httpapi.Failed(w, err)
		return
	}
	stream, err := a.Events.Stream(events.AgentsStream)
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
