// Package tasksapi is the tasks' HTTP API: the tasks and each one's
// detail and events, an archived task's read back from its blob, your
// answers and approvals, your moves on the board, erasing a task for
// good, what waits for you in Questions, your chat with a
// task's coordinator, a task's Workspace, and the task's settings you
// change on its page: its epic, labels, rank, budget, limit of turns and
// repo. Web mounts its Routes as it mounts an extension's.
package tasksapi

import (
	"net/http"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/settings"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
)

// State is what the routes read: each task, every task and every task's
// id, and an erased or archived one's erasure and summary; every task's
// summary, and the roster and settings that say how many tasks may run.
type State interface {
	Task(id string) (*tasks.Task, error)
	Tasks() []*tasks.Task
	TaskIDs() []string
	Erased(id string) (tasks.Erasure, bool)
	Summary(id string) (tasks.Summary, bool)
	Summaries() []tasks.Summary
	Roster() agents.Roster
	Settings() settings.Settings
}

// Store is the log's side of the routes beyond proposing: making a task,
// reading a task's events a page at a time, reading an archived task back
// from its blob, or unarchiving it; and erasing a task, redacting the
// copies of its words other tasks keep, and giving the space back.
type Store interface {
	historian
	Create(prefix string, p events.Proposal) (events.Event, *events.Rejection, error)
	LogAfter(name string, types []events.Type, after, before int64, limit int) ([]events.Event, error)
	LogBefore(name string, types []events.Type, after, before int64, limit int) ([]events.Event, error)
	Unarchive(stream string, blobs store.Opener) (events.Event, *events.Rejection, error)
	Erase(stream string, who events.Actor, why string, blobs store.Remover) (events.Event, *events.Rejection, error)
	RedactRows(stream, secret string, positions []int64, who events.Actor, why string) (events.Event, *events.Rejection, error)
	RedactArchivedRows(stream, secret string, positions []int64, who events.Actor, why string, summarise func() any, blobs store.ArchiveBlobs) (events.Event, *events.Rejection, error)
	GiveBackSpace() error
}

// API holds what the tasks' routes need: the log to write their changes
// to and read their events from, the state to read them back from, the
// cache of archived tasks read, the roster, where tasks' work is, the repo
// and limits a new task gets when it names none, where a repo named by its
// address is cloned, and whether a request carries the UI token; and, to
// erase a task, where its shared files and blob are kept, how its session
// is stopped and the snapshot to write again.
type API struct {
	Log         extensions.Proposer
	Store       Store
	State       State
	Archives    *Archives
	Roster      func() (agents.Roster, error) // and whether the agents' stream can't be read
	Fleet       *machines.Fleet               // nil for none: a Workspace is read on this machine, a done task's repo changes without a worktree, and no file is copied
	DefaultRepo string
	Defaults    tasks.Limits
	Clones      string
	Blobs       sharedfiles.Disk
	StopSession func(task string)       // ends a task's agent session, wherever it runs; nil for none
	Snapshots   *current.SnapshotWriter // nil for none
	// SharedFiles are a task's shared files as the page shows them, in its
	// detail: the shared files domain's view.
	SharedFiles func(t *tasks.Task) any
	// CarriesUIToken is whether a request carries the UI token, which only
	// the page and Claude Code connections have: the desk's answer approves
	// nothing.
	CarriesUIToken func(r *http.Request) bool
}

// Routes are the tasks' endpoints, for the page, and for the desk too
// those that hand work on or read what waits.
func (a API) Routes() []extensions.Route {
	return []extensions.Route{
		{Pattern: "GET /api/tasks", Auth: extensions.Desk, Handler: a.listTasks},
		{Pattern: "POST /api/tasks", Auth: extensions.Desk, Handler: a.createTask},
		{Pattern: "GET /api/tasks/{task}", Auth: extensions.Desk, Handler: a.showTask},
		{Pattern: "GET /api/tasks/{task}/events", Auth: extensions.Desk, Handler: a.listEvents},
		{Pattern: "POST /api/tasks/{task}/answer", Auth: extensions.Desk, Handler: a.answer},
		{Pattern: "POST /api/tasks/{task}/erase", Auth: extensions.UI, Handler: a.erase}, // never the desk's: nothing undoes it
		{Pattern: "POST /api/tasks/{task}/direct", Auth: extensions.UI, Handler: a.direct},
		{Pattern: "POST /api/tasks/{task}/actions/{action}/approve", Auth: extensions.UI, Handler: a.approve},
		{Pattern: "POST /api/tasks/{task}/actions/{action}/deny", Auth: extensions.UI, Handler: a.deny},
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

// noTask answers that there is no task id to act on: that an erased one
// is gone for good, and when it went, 410, rather than that there was
// never such a task, 404. Each names id, as setEpic's may be the task's
// or the epic's.
func (a API) noTask(w http.ResponseWriter, id string) {
	if erasure, erased := a.State.Erased(id); erased {
		http.Error(w, id+" was erased on "+erasure.At.Format("2 January 2006"), http.StatusGone)
		return
	}
	http.Error(w, "no such task "+id, http.StatusNotFound)
}
