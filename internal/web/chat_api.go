package web

import (
	"net/http"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
)

// message is your chat with a task's coordinator. What you say is the answer
// to its open question, or a direction for it; on a finished task it opens
// the task again, unarchived first if it is archived. Any turn in progress
// is stopped, so the coordinator reads it at once: the agent's work stays
// in the worktree, and its plan carries on when it is handed back.
func (s *Server) message(w http.ResponseWriter, r *http.Request) {
	var request struct{ Text string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	s.say(w, r.PathValue("task"), request.Text, "")
}

// say is what you send the coordinator of task id: typed in the chat, or
// asked of Claude in the Workspace (via "claude"), which sends it for you.
func (s *Server) say(w http.ResponseWriter, id, text, via string) {
	text = strings.TrimSpace(text)
	if text == "" {
		http.Error(w, "say something", http.StatusBadRequest)
		return
	}
	task := s.reopenable(w, id)
	if task == nil {
		return
	}
	if task.IsDone {
		if !s.dropErasedStories(w, task) {
			return
		}
		reopen := s.reopen(task, text, "")
		if via != "" {
			reopen.Data["via"] = via
		}
		httpapi.Propose(w, s.store, reopen)
		return
	}
	kind, field := events.TaskDirected, "text"
	if task.Question != "" {
		kind, field = events.TaskAnswered, "answer"
	}
	said := events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: kind, Data: map[string]any{field: text}}
	if via != "" {
		said.Data["via"] = via
	}
	open := task.OpenTurn()
	if open == nil {
		httpapi.Propose(w, s.store, said)
		return
	}
	if !httpapi.ProposeOrRefuse(w, s.store, said) {
		return
	}
	httpapi.Propose(w, s.store, events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.TurnStopped, Correlation: open.ID})
}
