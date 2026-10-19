package web

import (
	"slices"
	"sync"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// openedMax is how many archived tasks the cache keeps.
const openedMax = 8

// archives reads archived tasks back from their blobs, keeping the last
// few opened, so the Events tab and repeated reads of one task neither
// gunzip its blob nor fold its events again (docs/design/log-growth.md
// section 4). An entry is a stream's at one marker: a task unarchived
// and archived again has a new marker, so it is read afresh. Nothing goes
// into the state.
type archives struct {
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

// history is an archived task's whole record, rejections included: the
// store's History, with its blob read through the cache.
func (a *archives) history(log *store.Store, id string) ([]events.Event, error) {
	return log.History(id, a.events)
}

// events is a marker's events from its blob: store.Archived with the
// cache in front.
func (a *archives) events(marker events.Event) ([]events.Event, error) {
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
func (a *archives) task(marker events.Event, history []events.Event) (*tasks.Task, error) {
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
func (a *archives) find(marker events.Event) *opened {
	at := slices.IndexFunc(a.recent, func(o *opened) bool { return o.stream == marker.Stream && o.marker == marker.Position })
	if at < 0 {
		return nil
	}
	found := a.recent[at]
	a.recent = append(slices.Delete(a.recent, at, at+1), found)
	return found
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
