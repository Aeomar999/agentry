package web

import (
	"net/http"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// direct is what moving a card on the board does: it tells the coordinator
// what you want next, in words it reads at its next turn. The coordinator
// still makes the move, so the board never disagrees with the log. A task
// waiting for your answer takes it as the answer.
func (s *Server) direct(w http.ResponseWriter, r *http.Request) {
	var request struct{ To, Note string }
	if !decode(w, r, &request) {
		return
	}
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil {
		http.Error(w, "no such task", http.StatusNotFound)
		return
	}
	text := "Wrap this task up: finish it as it is."
	if request.To != "done" {
		roster, err := s.roster()
		if err != nil {
			failed(w, err)
			return
		}
		if _, ok := roster.Get(request.To); !ok {
			http.Error(w, "no agent called "+request.To, http.StatusBadRequest)
			return
		}
		text = "Hand this task to " + request.To + " next."
	}
	if note := strings.TrimSpace(request.Note); note != "" {
		text += " " + note
	}
	if task.Question != "" {
		s.propose(w, events.Proposal{Stream: task.ID, Actor: you, Type: events.TaskAnswered, Data: map[string]any{"answer": text}})
		return
	}
	s.propose(w, events.Proposal{Stream: task.ID, Actor: you, Type: events.TaskDirected, Data: map[string]any{"text": text}})
}
