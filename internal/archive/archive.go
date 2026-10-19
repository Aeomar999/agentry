// Package archive moves done tasks out of the live log: at start and then
// hourly, a round chooses the tasks done for long enough and archives them
// one at a time through Store.Archive, then gives the space they held back
// to the disk and asks for a snapshot of the smaller state
// (docs/design/log-growth.md section 4).
package archive

import (
	"errors"
	"log/slog"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// roundEvery is how long the archiver waits between rounds.
const roundEvery = time.Hour

// Log is the store as the archiver uses it: *store.Store.
type Log interface {
	Archive(stream string, summarise func() any, blobs store.Blobs) (events.Event, *events.Rejection, error)
	GiveBackSpace() error
	VacuumOnce() (bool, error)
}

// Snapshots is what writes the state's snapshots: *current.SnapshotWriter.
type Snapshots interface {
	WriteSoon()
}

// An Archiver archives the tasks done for at least its age.
type Archiver struct {
	log       Log
	state     *current.State
	blobs     store.Blobs
	snapshots Snapshots
	age       time.Duration

	now   func() time.Time                     // the clock
	after func(time.Duration) <-chan time.Time // the wait between rounds

	stop chan struct{}
	done chan struct{}
}

// New makes an archiver of the tasks in state done for at least age, which
// keeps their rows in blobs.
func New(log Log, state *current.State, blobs store.Blobs, snapshots Snapshots, age time.Duration) *Archiver {
	return &Archiver{log: log, state: state, blobs: blobs, snapshots: snapshots, age: age,
		now: time.Now, after: time.After, stop: make(chan struct{}), done: make(chan struct{})}
}

// Start runs a round now, and then one every hour, in the background until
// Close.
func (a *Archiver) Start() { go a.run() }

// Close stops the archiver and waits for a round under way. Start must
// have been called first.
func (a *Archiver) Close() {
	close(a.stop)
	<-a.done
}

func (a *Archiver) run() {
	defer close(a.done)
	for {
		a.round()
		select {
		case <-a.stop:
			return
		case <-a.after(roundEvery):
		}
	}
}

// round archives every task chosen, and says how many. A task the store
// refuses or fails to archive is logged and left for the next round, and
// so are the tasks after it when Close stops the round. After a round that
// archived any, the space their rows held goes back to the disk and a
// snapshot is asked for. Every round then tries the one full VACUUM, so
// one that failed runs again, but not while stopping: it can take a while.
func (a *Archiver) round() int {
	archived := 0
	for _, id := range a.chosen() {
		if a.stopping() {
			break
		}
		_, rejection, err := a.log.Archive(id, a.summarise(id), a.blobs)
		switch {
		case err != nil:
			slog.Warn("archive: the task can't be archived, so it waits for the next round", "task", id, "err", err)
		case rejection != nil:
			slog.Info("archive: the task was refused, so it waits for the next round", "task", id, "check", rejection.Check, "reason", rejection.Reason)
		default:
			archived++
		}
	}
	if archived > 0 {
		a.afterArchiving()
	}
	if !a.stopping() {
		a.vacuumOnce()
	}
	return archived
}

// stopping says whether Close has been called.
func (a *Archiver) stopping() bool {
	select {
	case <-a.stop:
		return true
	default:
		return false
	}
}

// afterArchiving gives the archived rows' space back and asks for a
// snapshot.
func (a *Archiver) afterArchiving() {
	if err := a.log.GiveBackSpace(); err != nil {
		slog.Warn("archive: the space can't all be given back yet", "err", err)
	}
	a.snapshots.WriteSoon()
}

// vacuumOnce switches an older file to incremental mode once a task has
// been archived. A VACUUM that fails, as on a full disk, is tried again at
// the next round: VacuumOnce runs until the file is incremental.
func (a *Archiver) vacuumOnce() {
	if ran, err := a.log.VacuumOnce(); err != nil {
		slog.Warn("archive: the log can't be vacuumed, so it is tried again at the next round", "err", err)
	} else if ran {
		slog.Info("archive: vacuumed the log, which now gives space back after each round")
	}
}

// chosen is every task done for at least the age and not archived yet,
// in the order they were made, but no epic with a story still in the state: an epic goes
// in a round after its last story's.
func (a *Archiver) chosen() []string {
	now := a.now()
	var chosen []string
	for _, id := range a.state.TaskIDs() {
		task, err := a.state.Task(id)
		if task == nil || err != nil { // archived, broken or erased
			continue
		}
		// A zero DoneAt says the task is done without saying when.
		if !task.IsDone || task.DoneAt.IsZero() || now.Sub(task.DoneAt) < a.age {
			continue
		}
		if task.Kind == tasks.Epic && a.anyInTheState(task.Stories) {
			continue
		}
		chosen = append(chosen, id)
	}
	return chosen
}

// anyInTheState says whether any of the tasks is still in the state: not
// archived, nor erased. A broken one is still there.
func (a *Archiver) anyInTheState(ids []string) bool {
	for _, id := range ids {
		task, err := a.state.Task(id)
		if task != nil || (err != nil && !errors.Is(err, current.ErrArchived)) {
			return true
		}
	}
	return false
}

// summarise is the summariser Store.Archive calls for a task, under its
// lock: the task's summary as the list shows it, or nil for a task the
// state can't hand out, which the rules refuse.
func (a *Archiver) summarise(id string) func() any {
	return func() any {
		task, err := a.state.Task(id)
		if task == nil || err != nil {
			return nil
		}
		return tasks.SummaryOf(task, a.state.Roster())
	}
}
