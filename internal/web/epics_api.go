package web

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// setEpic files a task under an epic as one of its stories, moves it to
// another, or with "" takes it out of the one it is in. The desk may do
// it too: it sorts work, and approves nothing. A task or epic there is
// none of is answered as noTask answers it: 410 once erased, else 404.
func (s *Server) setEpic(w http.ResponseWriter, r *http.Request) {
	var request struct{ Epic string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	id, epic := r.PathValue("task"), strings.TrimSpace(request.Epic)
	for _, named := range []string{id, epic} {
		if named != "" && s.isNone(named) {
			s.noTask(w, named)
			return
		}
	}
	if refused := s.fileUnder(id, epic); refused != "" {
		http.Error(w, refused, http.StatusConflict)
		return
	}
	httpapi.Reply(w, map[string]string{"task": id, "epic": request.Epic})
}

// isNone is whether there is no task id, never made or erased.
func (s *Server) isNone(id string) bool {
	task, err := s.task(id)
	return errors.Is(err, current.ErrErased) || err == nil && task == nil
}

// fileUnder makes the task a story of epic, out of any epic it was in, and
// says why not if it can't. Each side has its own event: the story knows
// its epic, and the epic knows its stories. The two can disagree, when an
// erasure took the story out of its epic and then failed, or the epic was
// erased: then only the story lets go, and filing it under the same epic
// again files it there.
func (s *Server) fileUnder(id, epic string) string {
	story, err := s.task(id)
	if err != nil || story == nil {
		return "no task " + id
	}
	if story.Epic == epic && (epic == "" || !s.unlisted(epic, id)) {
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
		var leaving []events.Proposal
		if !s.unlisted(old, id) {
			leaving = append(leaving, events.Proposal{Stream: old, Actor: httpapi.You, Type: events.StoryRemoved, Data: map[string]any{"story": id}})
		}
		leaving = append(leaving, events.Proposal{Stream: id, Actor: httpapi.You, Type: events.EpicLeft, Data: map[string]any{"epic": old}})
		for _, p := range leaving {
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

// unlisted is whether epic is known not to list story: it is erased, or
// its stories leave it out. An archived epic's stories are in its blob, so
// it isn't known.
func (s *Server) unlisted(epic, story string) bool {
	task, err := s.task(epic)
	if errors.Is(err, current.ErrErased) {
		return true
	}
	return err == nil && task != nil && !slices.Contains(task.Stories, story)
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
