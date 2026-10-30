package tasksapi

import (
	"errors"
	"net/http"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// setEpic files a task under an epic as one of its stories, moves it to
// another, or with "" takes it out of the one it is in. The desk may do
// it too: it sorts work, and approves nothing. A task or epic there is
// none of is answered as httpapi.NoTask answers it: 410 once erased, else 404.
func (a API) setEpic(w http.ResponseWriter, r *http.Request) {
	var request struct{ Epic string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	id, epic := r.PathValue("task"), strings.TrimSpace(request.Epic)
	for _, named := range []string{id, epic} {
		if named != "" && httpapi.IsNone(a.State, named) {
			httpapi.NoTask(w, a.State, named)
			return
		}
	}
	if refused := a.FileUnder(id, epic); refused != "" {
		http.Error(w, refused, http.StatusConflict)
		return
	}
	httpapi.Reply(w, map[string]string{"task": id, "epic": request.Epic})
}

// FileUnder makes the task a story of epic, out of any epic it was in, and
// says why not if it can't. Each side has its own event: the story knows
// its epic, and the epic knows its stories. The two can disagree, when an
// erasure took the story out of its epic and then failed, or the epic was
// erased: then only the story lets go, and filing it under the same epic
// again files it there.
func (a API) FileUnder(id, epic string) string {
	story, err := a.State.Task(id)
	if err != nil || story == nil {
		return "no task " + id
	}
	if story.Epic == epic && (epic == "" || !a.unlisted(epic, id)) {
		return ""
	}
	if epic != "" {
		target, err := a.State.Task(epic)
		switch {
		case errors.Is(err, current.ErrArchived):
			// Only a done task is archived, and a done epic takes no
			// stories, so it isn't unarchived for one: you reopen it.
			if summary, _ := a.State.Summary(epic); summary.Kind != tasks.Epic {
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
		if !a.unlisted(old, id) {
			leaving = append(leaving, events.Proposal{Stream: old, Actor: httpapi.You, Type: events.StoryRemoved, Data: map[string]any{"story": id}})
		}
		leaving = append(leaving, events.Proposal{Stream: id, Actor: httpapi.You, Type: events.EpicLeft, Data: map[string]any{"epic": old}})
		for _, p := range leaving {
			if refused := a.tryPropose(p); refused != "" {
				return refused
			}
		}
	}
	if epic == "" {
		return ""
	}
	if refused := a.tryPropose(events.Proposal{Stream: id, Actor: httpapi.You, Type: events.EpicJoined, Data: map[string]any{"epic": epic}}); refused != "" {
		return refused
	}
	return a.tryPropose(events.Proposal{Stream: epic, Actor: httpapi.You, Type: events.StoryAdded, Data: map[string]any{
		"story": id, "title": story.Title, "done": story.IsDone, "summary": story.Summary}})
}

// unlisted is whether epic is known not to list story: it is erased, or
// its stories leave it out. An archived epic's stories are in its blob, so
// it isn't known.
func (a API) unlisted(epic, story string) bool {
	task, err := a.State.Task(epic)
	if errors.Is(err, current.ErrErased) {
		return true
	}
	return err == nil && task != nil && !slices.Contains(task.Stories, story)
}

// tryPropose writes the proposal, and returns why it was refused, or "".
func (a API) tryPropose(p events.Proposal) string {
	_, rejection, err := a.Log.Propose(p)
	switch {
	case err != nil:
		return err.Error()
	case rejection != nil:
		return rejection.Reason
	}
	return ""
}
