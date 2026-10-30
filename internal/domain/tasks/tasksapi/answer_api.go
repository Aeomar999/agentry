package tasksapi

import (
	"net/http"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/runtime/coordinator"
)

// answer sends your answer to the coordinator's question. Two answers are
// approvals, which only you give, on the page (or through Claude Code, which
// asks you itself): closing an epic, and copying a file from outside a
// machine's worktrees. The desk passes on what you say, and approves nothing.
func (a API) answer(w http.ResponseWriter, r *http.Request) {
	var request struct{ Answer string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	if a.isNone(r.PathValue("task")) {
		a.noTask(w, r.PathValue("task"))
		return
	}
	answer := strings.TrimSpace(request.Answer)
	task, err := a.State.Task(r.PathValue("task"))
	closes := err == nil && task != nil && task.Closing != "" && strings.EqualFold(answer, coordinator.CloseEpic)
	copies := err == nil && task != nil && task.Asked.Fetch != nil && strings.EqualFold(answer, coordinator.CopyFile)
	switch {
	case !closes && !copies:
		httpapi.Propose(w, a.Log, events.Proposal{Stream: r.PathValue("task"), Actor: httpapi.You, Type: events.TaskAnswered, Data: map[string]any{"answer": answer}})
	case !a.CarriesUIToken(r):
		http.Error(w, "that is approved on the page", http.StatusForbidden)
	case closes:
		// One event, the answer and the close together: an answer alone
		// would start the coordinator's next turn.
		httpapi.Propose(w, a.Log, events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.TaskDone, Data: map[string]any{"summary": task.Closing}})
	default:
		a.copyApproved(w, r, task.ID, *task.Asked.Fetch)
	}
}

// copyApproved copies the file you allowed, and answers with it.
func (a API) copyApproved(w http.ResponseWriter, r *http.Request, id string, f tasks.Fetch) {
	if a.Fleet == nil {
		http.Error(w, "this server has no machines to copy from", http.StatusConflict)
		return
	}
	m, online := a.Fleet.Get(f.Machine)
	if !online {
		http.Error(w, f.Machine+" is offline; answer again once it is back", http.StatusConflict)
		return
	}
	files, err := m.SharedFetch(r.Context(), f.Path, f.As, true)
	if err != nil {
		http.Error(w, "copying "+f.Machine+":"+f.Path+": "+err.Error(), http.StatusConflict)
		return
	}
	httpapi.Propose(w, a.Log, events.Proposal{Stream: id, Actor: httpapi.You, Type: events.FilesFetched, Data: coordinator.Fetched(f, files, true)})
}
