package tasksapi

import (
	"errors"
	"fmt"
	"net/http"
	"slices"
	"sync"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// openedMax is how many archived tasks the cache keeps.
const openedMax = 8

// Archives reads archived tasks back from their blobs, keeping the last
// few opened, so the Events tab and repeated reads of one task neither
// gunzip its blob nor fold its events again (docs/design/log-growth.md
// section 4). An entry is a stream's at one marker: a task unarchived
// and archived again has a new marker, so it is read afresh. Nothing goes
// into the state.
type Archives struct {
	blobs store.Opener

	mu     sync.Mutex
	recent []*opened // the most recently used last
}

// opened is an archived task as the cache keeps it.
type opened struct {
	stream string
	marker int64
	events []events.Event // its blob's, up to the marker's through
	task   *tasks.Task    // what its history folds to, once a read asked
}

// NewArchives is an empty cache of the archived tasks read from blobs.
func NewArchives(blobs store.Opener) *Archives { return &Archives{blobs: blobs} }

// historian is the log's side of an archived task's read: its whole record.
type historian interface {
	History(stream string, archived store.Archived) ([]events.Event, error)
}

// history is an archived task's whole record, rejections included: the
// store's History, with its blob read through the cache.
func (a *Archives) history(log historian, id string) ([]events.Event, error) {
	return log.History(id, a.events)
}

// events is a marker's events from its blob: store.Archived with the
// cache in front.
func (a *Archives) events(marker events.Event) ([]events.Event, error) {
	a.mu.Lock()
	found := a.find(marker)
	a.mu.Unlock()
	if found != nil {
		return found.events, nil
	}
	read, err := store.FromBlobs(a.blobs)(marker)
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.find(marker) == nil { // another read may have kept it meanwhile
		a.recent = append(a.recent, &opened{stream: marker.Stream, marker: marker.Position, events: read})
		if len(a.recent) > openedMax {
			a.recent = slices.Delete(a.recent, 0, 1)
		}
	}
	return read, nil
}

// task is the task an archived task's history folds to: as it was before
// its marker. It is kept with the marker's events while they are cached.
func (a *Archives) task(marker events.Event, history []events.Event) (*tasks.Task, error) {
	a.mu.Lock()
	var kept *tasks.Task
	if found := a.find(marker); found != nil {
		kept = found.task
	}
	a.mu.Unlock()
	if kept != nil {
		return kept, nil
	}
	task, err := tasks.Replay(acceptedBefore(marker.Position, history))
	if err != nil {
		return nil, err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if found := a.find(marker); found != nil {
		found.task = task
	}
	return task, nil
}

// find is the entry kept for a marker, made the most recently used, or
// nil. It is called with a.mu held, which also guards the entry's task.
func (a *Archives) find(marker events.Event) *opened {
	at := slices.IndexFunc(a.recent, func(o *opened) bool { return o.stream == marker.Stream && o.marker == marker.Position })
	if at < 0 {
		return nil
	}
	found := a.recent[at]
	a.recent = append(slices.Delete(a.recent, at, at+1), found)
	return found
}

// Forget drops what the cache keeps of a stream, as once it is erased.
func (a *Archives) Forget(stream string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.recent = slices.DeleteFunc(a.recent, func(o *opened) bool { return o.stream == stream })
}

// acceptedBefore is the accepted events before a position.
func acceptedBefore(position int64, history []events.Event) []events.Event {
	var accepted []events.Event
	for _, e := range history {
		if e.Position < position && e.Type != events.ProposalRejected {
			accepted = append(accepted, e)
		}
	}
	return accepted
}

// shownTask is the task a read of its own is about, or nil once it has
// answered that there is none, or that it can't be read. The state keeps
// an archived task as its summary alone, so it is read back from its
// history, and archived says what it was read from. An erased task is
// answered 410, with the day it was erased.
func (a API) shownTask(w http.ResponseWriter, r *http.Request) (task *tasks.Task, archived *archivedRead) {
	id := r.PathValue("task")
	task, err := a.State.Task(id)
	if errors.Is(err, current.ErrArchived) {
		if archived, err = a.readArchived(id); err != nil {
			httpapi.Failed(w, err)
			return nil, nil
		}
		return archived.task, archived
	}
	if err != nil || task == nil {
		a.noTask(w, id)
		return nil, nil
	}
	return task, nil
}

// reopenable is the task a route that may reopen it acts on, or nil once
// it has answered that there is none, or that it can't be had. An archived
// task is done, so the route reopens it, and it is unarchived for that
// first: its rows come back from its blob (Store.Unarchive), and the state
// holds it whole again (docs/design/log-growth.md section 4, "Reopening an
// archived task"). When the store refuses, its reason is the answer, with
// 409, and when it fails, with 500; it has written nothing either way. An
// erased task is answered 410, as shownTask answers it.
func (a API) reopenable(w http.ResponseWriter, id string) *tasks.Task {
	task, err := a.State.Task(id)
	if errors.Is(err, current.ErrArchived) {
		return a.unarchive(w, id)
	}
	if err != nil || task == nil {
		a.noTask(w, id)
		return nil
	}
	return task
}

// unarchive makes an archived task whole again, and returns it as the
// state now holds it, or nil once it has answered why it can't. A refusal
// because another request unarchived it first isn't one: the task is
// whole.
func (a API) unarchive(w http.ResponseWriter, id string) *tasks.Task {
	_, rejection, err := a.Store.Unarchive(id, a.Archives.blobs)
	if err != nil {
		httpapi.Failed(w, err)
		return nil
	}
	task, err := a.State.Task(id)
	switch {
	case err == nil && task != nil:
		return task
	case rejection != nil:
		http.Error(w, rejection.Reason, http.StatusConflict)
	default:
		httpapi.Failed(w, fmt.Errorf("%s was unarchived, but can't be read: %v", id, err))
	}
	return nil
}

// archivedRead is an archived task read back from its history.
type archivedRead struct {
	task    *tasks.Task    // as it was before it was archived
	marker  events.Event   // its task.archived
	history []events.Event // its whole record, rejections included
}

// readArchived reads an archived task back from its history, through the
// cache of the last few read.
func (a API) readArchived(id string) (*archivedRead, error) {
	history, err := a.Archives.history(a.Store, id)
	if err != nil {
		return nil, err
	}
	marker, archived := store.MarkerOf(history)
	if !archived { // unarchived since the state was read
		return nil, fmt.Errorf("%s was unarchived as it was read: read it again", id)
	}
	task, err := a.Archives.task(marker, history)
	if err != nil {
		return nil, fmt.Errorf("%s's archived events can't be replayed: %w", id, err)
	}
	if task == nil {
		return nil, fmt.Errorf("%s's archived events have no task.created", id)
	}
	return &archivedRead{task: task, marker: marker, history: history}, nil
}

// Unarchive is unarchive, for web's board route until it moves here.
func (a API) Unarchive(w http.ResponseWriter, id string) *tasks.Task { return a.unarchive(w, id) }

// ArchivedTask is an archived task as it was before it was archived, read
// back from its history, for web's board route until it moves here.
func (a API) ArchivedTask(id string) (*tasks.Task, error) {
	archived, err := a.readArchived(id)
	if err != nil {
		return nil, err
	}
	return archived.task, nil
}

// Opened is the streams the cache keeps, the one used longest ago first.
func (a *Archives) Opened() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	var streams []string
	for _, o := range a.recent {
		streams = append(streams, o.stream)
	}
	return streams
}
