package web

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
)

// listIdeas answers the ideas, of every repo or of ?repo=, newest session
// first: by when the task they came from was made, then newest idea first.
func (s *Server) listIdeas(w http.ResponseWriter, r *http.Request) {
	list, err := ideas.List(s.store, r.URL.Query().Get("repo"))
	if err != nil {
		failed(w, err)
		return
	}
	started := map[string]time.Time{}
	for _, idea := range list {
		if _, seen := started[idea.Task]; !seen {
			if task, err := s.task(idea.Task); err == nil && task != nil {
				started[idea.Task] = task.CreatedAt
			}
		}
	}
	slices.SortStableFunc(list, func(a, b *ideas.Idea) int { return started[b.Task].Compare(started[a.Task]) })
	views := []ideaView{}
	for _, idea := range list {
		views = append(views, viewOf(idea))
	}
	reply(w, views)
}

// acceptIdea makes the idea a task in your Backlog (ideas.Accept). It
// answers 404 for no such idea; 409 with the reason when it was refused and
// nothing was written (the idea isn't open, the kind or the repo is wrong);
// otherwise 200 with what happened: the task made, whether the idea is
// marked accepted, and if not, why. A task made and the idea not marked is
// still a 200, so the page can show the task.
func (s *Server) acceptIdea(w http.ResponseWriter, r *http.Request) {
	var request struct{ Kind, Note string }
	if !decode(w, r, &request) {
		return
	}
	id := r.PathValue("idea")
	if !s.ideaExists(w, id) {
		return
	}
	got, err := ideas.Accept(s.store, s.state, id, ideas.Choice{Kind: strings.TrimSpace(request.Kind), Note: strings.TrimSpace(request.Note),
		Budget: s.config.Budget})
	switch {
	case err != nil:
		failed(w, err)
	case got.Task == "":
		http.Error(w, got.Refused, http.StatusConflict)
	default:
		reply(w, acceptance{Task: got.Task, Accepted: got.Accepted, Refused: got.Refused})
	}
}

// dismissIdea dismisses an open idea with your note: 404 for no such idea,
// 409 when the rules refuse it, else the event.
func (s *Server) dismissIdea(w http.ResponseWriter, r *http.Request) {
	var request struct{ Note string }
	if !decode(w, r, &request) {
		return
	}
	id := r.PathValue("idea")
	if !s.ideaExists(w, id) {
		return
	}
	s.propose(w, events.Proposal{Stream: id, Actor: you, Type: events.IdeaDismissed, Data: map[string]any{"note": strings.TrimSpace(request.Note)}})
}

// ideaExists answers 404 when there is no such idea.
func (s *Server) ideaExists(w http.ResponseWriter, id string) bool {
	if !events.IsIdeaStream(id) {
		http.Error(w, "no such idea", http.StatusNotFound)
		return false
	}
	idea, err := ideas.Load(s.store, id)
	switch {
	case err != nil:
		failed(w, err)
		return false
	case idea == nil:
		http.Error(w, "no such idea", http.StatusNotFound)
		return false
	}
	return true
}
