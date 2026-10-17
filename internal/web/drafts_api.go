package web

import (
	"crypto/subtle"
	"fmt"
	"net/http"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Drafts: messages for the coordinator that the Claude session you opened
// beside a task wrote with `agentry draft`. It can only draft; you send a
// draft, as it is or changed, or discard it, because sending starts work.

var helper = events.Actor{Kind: events.Helper, ID: "claude"}

func (s *Server) draft(w http.ResponseWriter, r *http.Request) {
	var request struct{ Text string }
	if !decode(w, r, &request) {
		return
	}
	text := strings.TrimSpace(request.Text)
	task, err := s.task(r.PathValue("task"))
	switch {
	case err != nil || task == nil:
		http.Error(w, "no such task", http.StatusNotFound)
	case text == "":
		http.Error(w, "draft something", http.StatusBadRequest)
	default:
		s.propose(w, events.Proposal{Stream: task.ID, Actor: helper, Type: events.MessageDrafted,
			Correlation: fmt.Sprintf("draft-%d", len(task.Drafts)+1), Data: map[string]any{"text": text}})
	}
}

// sendDraft sends a draft as your message, with the words you settled on.
func (s *Server) sendDraft(w http.ResponseWriter, r *http.Request) {
	var request struct{ Text string }
	if !decode(w, r, &request) {
		return
	}
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil {
		http.Error(w, "no such task", http.StatusNotFound)
		return
	}
	draft := task.Draft(r.PathValue("draft"))
	if draft == nil || draft.Sent || draft.Discarded {
		http.Error(w, "that draft isn't waiting for you", http.StatusConflict)
		return
	}
	s.say(w, task, request.Text, draft.ID)
}

func (s *Server) discardDraft(w http.ResponseWriter, r *http.Request) {
	s.propose(w, events.Proposal{Stream: r.PathValue("task"), Actor: you, Type: events.DraftDiscarded, Correlation: r.PathValue("draft")})
}

// requireUIOrHelper also lets in the token of the Claude sessions beside
// tasks: for drafting, and nothing else.
func (s *Server) requireUIOrHelper(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		given := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		isHelper := s.config.HelperToken != "" && subtle.ConstantTimeCompare([]byte(given), []byte(s.config.HelperToken)) == 1
		if !isHelper && !s.carriesUIToken(r) {
			http.Error(w, "open the link `agentry serve` printed", http.StatusUnauthorized)
			return
		}
		next(w, r)
	})
}
