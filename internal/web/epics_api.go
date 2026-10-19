package web

import (
	"errors"
	"net/http"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// setEpic files a task under an epic as one of its stories, moves it to
// another, or with "" takes it out of the one it is in. The desk may do
// it too: it sorts work, and approves nothing.
func (s *Server) setEpic(w http.ResponseWriter, r *http.Request) {
	var request struct{ Epic string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	if refused := s.fileUnder(r.PathValue("task"), strings.TrimSpace(request.Epic)); refused != "" {
		http.Error(w, refused, http.StatusConflict)
		return
	}
	httpapi.Reply(w, map[string]string{"task": r.PathValue("task"), "epic": request.Epic})
}

// fileUnder makes the task a story of epic, out of any epic it was in, and
// says why not if it can't. Each side has its own event: the story knows
// its epic, and the epic knows its stories.
func (s *Server) fileUnder(id, epic string) string {
	story, err := s.task(id)
	if err != nil || story == nil {
		return "no task " + id
	}
	if story.Epic == epic {
		return ""
	}
	if epic != "" {
		target, err := s.task(epic)
		switch {
		case errors.Is(err, current.ErrArchived):
			// Only a done task is archived, and a done epic takes no
			// stories, so it isn't unarchived for one: you reopen it.
			if summary, _ := s.state.Summary(epic); summary.Kind != tasks.Epic {
				return epic + " isn't an epic"
			}
			return epic + " is done; reopen it to add stories"
		case err != nil || target == nil:
			return "no task " + epic
		case target.Kind != "epic":
			return epic + " isn't an epic"
		case target.IsDone:
			return epic + " is done; reopen it to add stories"
		}
	}
	if old := story.Epic; old != "" {
		for _, p := range []events.Proposal{
			{Stream: old, Actor: httpapi.You, Type: events.StoryRemoved, Data: map[string]any{"story": id}},
			{Stream: id, Actor: httpapi.You, Type: events.EpicLeft, Data: map[string]any{"epic": old}},
		} {
			if refused := s.tryPropose(p); refused != "" {
				return refused
			}
		}
	}
	if epic == "" {
		return ""
	}
	if refused := s.tryPropose(events.Proposal{Stream: id, Actor: httpapi.You, Type: events.EpicJoined, Data: map[string]any{"epic": epic}}); refused != "" {
		return refused
	}
	return s.tryPropose(events.Proposal{Stream: epic, Actor: httpapi.You, Type: events.StoryAdded, Data: map[string]any{
		"story": id, "title": story.Title, "done": story.IsDone, "summary": story.Summary}})
}

// tryPropose writes the proposal, and returns why it was refused, or "".
func (s *Server) tryPropose(p events.Proposal) string {
	_, rejection, err := s.store.Propose(p)
	switch {
	case err != nil:
		return err.Error()
	case rejection != nil:
		return rejection.Reason
	}
	return ""
}
