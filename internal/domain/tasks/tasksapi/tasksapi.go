// Package tasksapi is the tasks' HTTP API: what waits for you in
// Questions, your chat with a task's coordinator, a task's Workspace, and
// the task's settings you change on its page: its epic, labels, rank,
// budget, limit of turns and repo. Web mounts its Routes as it mounts an
// extension's.
package tasksapi

import (
	"net/http"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
)

// State is what the routes read: each task, every task, and an erased or
// archived one's erasure and summary.
type State interface {
	Task(id string) (*tasks.Task, error)
	Tasks() []*tasks.Task
	Erased(id string) (tasks.Erasure, bool)
	Summary(id string) (tasks.Summary, bool)
}

// API holds what the tasks' routes need: the log to write their changes
// to, the state to read them back from, where tasks' work is and where a
// repo named by its address is cloned; and, for a message that reopens a
// done task, web's own steps.
type API struct {
	Log    extensions.Proposer
	State  State
	Fleet  *machines.Fleet // nil for none: a Workspace is read on this machine, and a done task's repo changes without a worktree
	Clones string
	// Reopenable is the task a message may reopen, unarchived first if it
	// is archived, or nil once it has answered why there is none.
	Reopenable func(w http.ResponseWriter, id string) *tasks.Task
	// DropErasedStories takes the erased stories a done epic still lists
	// out of it, and is false once it has answered that it failed.
	DropErasedStories func(w http.ResponseWriter, epic *tasks.Task) bool
	// Reopen is the proposal that reopens a done task with text.
	Reopen func(task *tasks.Task, text, note string) events.Proposal
}

// Routes are the tasks' endpoints, for the page, and for the desk too
// those that hand work on or read what waits.
func (a API) Routes() []extensions.Route {
	return []extensions.Route{
		{Pattern: "PUT /api/tasks/{task}/epic", Auth: extensions.Desk, Handler: a.setEpic},
		{Pattern: "GET /api/questions", Auth: extensions.Desk, Handler: a.listQuestions},
		{Pattern: "PUT /api/tasks/{task}/labels", Auth: extensions.UI, Handler: a.label},
		{Pattern: "PUT /api/tasks/{task}/budget", Auth: extensions.UI, Handler: a.setBudget},
		{Pattern: "PUT /api/tasks/{task}/turns", Auth: extensions.UI, Handler: a.setMaxTurns},
		{Pattern: "PUT /api/tasks/{task}/repo", Auth: extensions.UI, Handler: a.changeRepo},
		{Pattern: "PUT /api/tasks/{task}/rank", Auth: extensions.UI, Handler: a.rank},
		{Pattern: "GET /api/tasks/{task}/files", Auth: extensions.UI, Handler: a.listFiles},
		{Pattern: "GET /api/tasks/{task}/file", Auth: extensions.UI, Handler: a.readFile},
		{Pattern: "GET /api/tasks/{task}/commits", Auth: extensions.UI, Handler: a.listCommits},
		{Pattern: "POST /api/tasks/{task}/message", Auth: extensions.UI, Handler: a.message}, // your chat with its coordinator
	}
}

// rank puts the task in your priority order: the page sends a rank between
// the two tasks it was dropped between.
func (a API) rank(w http.ResponseWriter, r *http.Request) {
	var request struct{ Rank float64 }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	if a.isNone(r.PathValue("task")) {
		a.noTask(w, r.PathValue("task"))
		return
	}
	httpapi.Propose(w, a.Log, events.Proposal{Stream: r.PathValue("task"), Actor: httpapi.You, Type: events.TaskRanked, Data: map[string]any{"rank": request.Rank}})
}

// label sets the task's labels, all of them.
func (a API) label(w http.ResponseWriter, r *http.Request) {
	var request struct{ Labels []string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	if a.isNone(r.PathValue("task")) {
		a.noTask(w, r.PathValue("task"))
		return
	}
	httpapi.Propose(w, a.Log, events.Proposal{Stream: r.PathValue("task"), Actor: httpapi.You, Type: events.TaskLabeled,
		Data: map[string]any{"labels": CleanLabels(request.Labels)}})
}

// CleanLabels trims labels, lowercases them so "Bug" and "bug" are one, and
// drops empty and repeated ones. The rules check what is left.
func CleanLabels(labels []string) []string {
	clean := []string{}
	for _, label := range labels {
		label = strings.ToLower(strings.Join(strings.Fields(label), " "))
		if label != "" && !slices.Contains(clean, label) {
			clean = append(clean, label)
		}
	}
	return clean
}

// noTask answers as web's noTask does (internal/web/erase_api.go): that an
// erased task is gone for good, and when it went, 410, rather than that
// there was never such a task, 404.
func (a API) noTask(w http.ResponseWriter, id string) {
	if erasure, erased := a.State.Erased(id); erased {
		http.Error(w, id+" was erased on "+erasure.At.Format("2 January 2006"), http.StatusGone)
		return
	}
	http.Error(w, "no such task "+id, http.StatusNotFound)
}
