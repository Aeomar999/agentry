package web

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// listIdeas answers the ideas, of every repo or of ?repo=, newest session
// first: by when the task they came from was made, then newest idea first.
// An idea from a session after an epic closed names that epic, an open one
// why Agentry wouldn't accept it for you, under your rule, and an accepted
// one the column its task is in now.
func (s *Server) listIdeas(w http.ResponseWriter, r *http.Request) {
	list := ideas.List(s.state, r.URL.Query().Get("repo"))
	started, about := map[string]time.Time{}, map[string]*tasks.Task{}
	for _, idea := range list {
		if _, seen := started[idea.Task]; !seen {
			if task, err := s.task(idea.Task); err == nil && task != nil {
				started[idea.Task] = task.CreatedAt
				if epic, err := s.task(task.About); err == nil && epic != nil {
					about[idea.Task] = epic
				}
			}
		}
	}
	slices.SortStableFunc(list, func(a, b *ideas.Idea) int { return started[b.Task].Compare(started[a.Task]) })
	rule, roster := s.state.Settings().AutoAcceptRule, s.state.Roster()
	views := []ideaView{}
	for _, idea := range list {
		why := ""
		if idea.State == ideas.Open {
			why = ideas.NotAutoAccepted(idea, list, roster, rule)
		}
		var became *tasks.Task
		if idea.Became != "" {
			became, _ = s.task(idea.Became) // one that can't be read shows with no column
		}
		views = append(views, viewOf(idea, about[idea.Task], became, roster, why))
	}
	reply(w, views)
}

// acceptIdea makes the idea a task in your Backlog (ideas.Accept), or
// started with "start", with "budget" as a new task's: absent or 0 takes the
// one tasks.NewLimits gives it (the server's, or the epic's it follows up),
// -1 is no limit, and below that is a 400. Its turns are always NewLimits's.
// It answers 404 for no such idea; 409 with the reason when it was refused
// and nothing was written (the idea isn't open, the kind or the repo is
// wrong); otherwise 200 with what happened: the task made, the column it is
// in, whether the idea is marked accepted, and if not, why. A task made and
// the idea not marked is still a 200, so the page can show the task.
func (s *Server) acceptIdea(w http.ResponseWriter, r *http.Request) {
	var request struct {
		Kind, Note string
		Start      bool
		Budget     int
	}
	if !decode(w, r, &request) {
		return
	}
	if request.Budget < noLimit {
		http.Error(w, "a budget is a number of tokens, 0 for the default or -1 for no limit", http.StatusBadRequest)
		return
	}
	id := r.PathValue("idea")
	if !s.ideaExists(w, id) {
		return
	}
	got, err := ideas.Accept(s.store, s.state, id, ideas.Choice{Kind: strings.TrimSpace(request.Kind), Note: strings.TrimSpace(request.Note),
		Defaults: s.config.defaults(), Budget: chosenBudget(request.Budget), Start: request.Start})
	switch {
	case err != nil:
		failed(w, err)
	case got.Task == "":
		http.Error(w, got.Refused, http.StatusConflict)
	default:
		reply(w, acceptance{Task: got.Task, Column: got.Column, Accepted: got.Accepted, Refused: got.Refused})
	}
}

// chosenBudget is the budget a request chose for an idea's task, nil when
// it chose none (0), so the task takes the one its limits give.
func chosenBudget(asked int) *int {
	if asked == 0 {
		return nil
	}
	budget := limitOf(asked, 0)
	return &budget
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
	s.propose(w, events.Proposal{Stream: id, Actor: you, Type: ideas.IdeaDismissed, Data: map[string]any{"note": strings.TrimSpace(request.Note)}})
}

// ideaExists answers 404 when there is no such idea.
func (s *Server) ideaExists(w http.ResponseWriter, id string) bool {
	if !ideas.IsStream(id) {
		http.Error(w, "no such idea", http.StatusNotFound)
		return false
	}
	idea, err := s.state.Idea(id)
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
