package archive

import (
	"errors"
	"path/filepath"
	"reflect"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

const age = 30 * 24 * time.Hour

var you = events.Actor{Kind: events.Human, ID: "you"}

// counted is the store with its calls after a round counted.
type counted struct {
	*store.Store
	givenBack, vacuumed atomic.Int32
}

func (c *counted) GiveBackSpace() error {
	c.givenBack.Add(1)
	return c.Store.GiveBackSpace()
}

func (c *counted) VacuumOnce() (bool, error) {
	c.vacuumed.Add(1)
	return c.Store.VacuumOnce()
}

// asked counts the snapshots asked for.
type asked struct{ n atomic.Int32 }

func (a *asked) WriteSoon() { a.n.Add(1) }

// refusing is a folder of blobs that says the first blob put isn't there,
// so Store.Archive refuses the first task it is given.
type refusing struct {
	sharedfiles.Disk
	refused bool
}

func (r *refusing) Has(hash string) bool {
	if !r.refused {
		r.refused = true
		return false
	}
	return r.Disk.Has(hash)
}

// fixture is a log with an archiver of it, whose clock the test sets.
type fixture struct {
	log       *counted
	state     *current.State
	blobs     sharedfiles.Disk
	snapshots *asked
	archiver  *Archiver
	clock     time.Time
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	f := &fixture{log: &counted{Store: s}, state: state, blobs: sharedfiles.Disk{Dir: t.TempDir()}, snapshots: &asked{}}
	f.archiver = New(f.log, state, f.blobs, f.snapshots, age)
	f.archiver.now = func() time.Time { return f.clock }
	for _, p := range agents.SeedProposals(nil) {
		f.propose(t, p)
	}
	return f
}

func (f *fixture) propose(t *testing.T, p events.Proposal) {
	t.Helper()
	if _, rejection, err := f.log.Propose(p); rejection != nil || err != nil {
		t.Fatalf("%s on %s: %v, %v", p.Type, p.Stream, rejection, err)
	}
}

// task makes a task of yours, an epic if kind says so.
func (f *fixture) task(t *testing.T, id string, kind tasks.Kind) {
	t.Helper()
	f.propose(t, events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Title of " + id, "repo": "/r", "coordinator": "tech-lead", "kind": string(kind)}})
}

// story makes a task of yours a story of the epic.
func (f *fixture) story(t *testing.T, id, epic string) {
	t.Helper()
	f.task(t, id, tasks.Story)
	f.propose(t, events.Proposal{Stream: epic, Actor: you, Type: events.StoryAdded, Data: map[string]any{"story": id, "title": "Title of " + id}})
	f.propose(t, events.Proposal{Stream: id, Actor: you, Type: events.EpicJoined, Data: map[string]any{"epic": epic}})
}

// done moves a task to Done, and says when.
func (f *fixture) done(t *testing.T, id string) time.Time {
	t.Helper()
	f.propose(t, events.Proposal{Stream: id, Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": id + " is done.", "moved": true}})
	task, err := f.state.Task(id)
	if err != nil || task == nil || task.DoneAt.IsZero() {
		t.Fatalf("%s should be done: %v, %v", id, task, err)
	}
	return task.DoneAt
}

// isArchived says whether the state holds a task as its summary alone.
func (f *fixture) isArchived(id string) bool {
	_, err := f.state.Task(id)
	return errors.Is(err, current.ErrArchived)
}

// The age counts from task.done: a task done for the age is archived, one
// done for less isn't, and a task reopened since isn't chosen at all.
func TestATaskIsArchivedOnceDoneForTheAge(t *testing.T) {
	f := newFixture(t)
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		f.task(t, id, tasks.Story)
	}
	first := f.done(t, "task-1")
	f.done(t, "task-2") // later than task-1, so done for less than the age
	f.done(t, "task-3")
	f.propose(t, events.Proposal{Stream: "task-3", Actor: you, Type: events.TaskReopened, Data: map[string]any{"text": "One more thing."}})

	f.clock = first.Add(age)
	if archived := f.archiver.round(); archived != 1 || !f.isArchived("task-1") || f.isArchived("task-2") || f.isArchived("task-3") {
		t.Errorf("archived %d; task-1 %v, task-2 %v, task-3 %v; want only task-1", archived, f.isArchived("task-1"), f.isArchived("task-2"), f.isArchived("task-3"))
	}
	f.clock = f.clock.Add(age)
	if archived := f.archiver.round(); archived != 1 || !f.isArchived("task-2") || f.isArchived("task-3") {
		t.Errorf("a month on: archived %d, task-2 %v, task-3 %v; want task-2 alone, the reopened task-3 left", archived, f.isArchived("task-2"), f.isArchived("task-3"))
	}
}

// Stories go first: an epic with a story still in the state waits, and
// the round after its last story is archived archives it.
func TestAnEpicWaitsForItsStories(t *testing.T) {
	f := newFixture(t)
	f.task(t, "task-1", tasks.Epic)
	f.story(t, "task-2", "task-1")
	f.story(t, "task-3", "task-1")
	f.done(t, "task-2")
	f.done(t, "task-1") // closed with task-3 still open
	f.clock = time.Now().Add(age)

	if archived := f.archiver.round(); archived != 1 || !f.isArchived("task-2") || f.isArchived("task-1") {
		t.Fatalf("with task-3 open: archived %d, task-2 %v, epic %v; want task-2 alone", archived, f.isArchived("task-2"), f.isArchived("task-1"))
	}
	f.clock = f.done(t, "task-3").Add(age)
	if archived := f.archiver.round(); archived != 1 || !f.isArchived("task-3") || f.isArchived("task-1") {
		t.Fatalf("task-3 done: archived %d, task-3 %v, epic %v; want task-3, the epic a round later", archived, f.isArchived("task-3"), f.isArchived("task-1"))
	}
	if archived := f.archiver.round(); archived != 1 || !f.isArchived("task-1") {
		t.Errorf("its last story archived: archived %d, epic %v; want the epic", archived, f.isArchived("task-1"))
	}
}

// Equivalence after a round (log-growth.md section 2): a replay of the
// blob's rows with the live ones, and the state from before with the
// marker applied, both equal the store's state, in which the task is its
// summary as the list showed it.
func TestARoundReplaysFromTheBlobAndTheLiveRows(t *testing.T) {
	f := newFixture(t)
	f.task(t, "task-1", tasks.Story)
	f.task(t, "task-2", tasks.Story)
	f.clock = f.done(t, "task-1").Add(age)
	task, _ := f.state.Task("task-1")
	summary := tasks.SummaryOf(task, f.state.Roster())
	all := everyEvent(t, f.log.Store)

	if archived := f.archiver.round(); archived != 1 {
		t.Fatalf("archived %d, want task-1", archived)
	}
	live := everyEvent(t, f.log.Store)
	marker := live[len(live)-1]
	if marker.Type != events.TaskArchived || marker.Stream != "task-1" {
		t.Fatalf("the last event is %s on %s, want task-1's marker", marker.Type, marker.Stream)
	}
	whole := withTheBlob(t, live, f.blobs, marker)
	if !reflect.DeepEqual(whole[:len(whole)-1], all) {
		t.Fatal("the blob's rows and the live ones aren't the log from before the marker")
	}
	before := replayed(all)
	before.Apply(marker)
	for name, held := range map[string]*current.State{"the store's": f.state, "the one from before": before, "a replay with the blob": replayed(whole)} {
		if err := current.Equal(f.log.Store, held); err != nil {
			t.Errorf("%s state: %v", name, err)
		}
	}
	if kept := f.state.Summaries(); !slices.ContainsFunc(kept, func(s tasks.Summary) bool { return reflect.DeepEqual(s, summary) }) {
		t.Errorf("the list doesn't show task-1 as it was: %+v", summary)
	}
}

// A task the store refuses is left for the next round, and the round goes
// on with the others; the next round archives it.
func TestARefusedTaskWaitsForTheNextRound(t *testing.T) {
	f := newFixture(t)
	f.task(t, "task-1", tasks.Story)
	f.task(t, "task-2", tasks.Story)
	f.done(t, "task-1")
	f.clock = f.done(t, "task-2").Add(age)
	f.archiver.blobs = &refusing{Disk: f.blobs}

	if archived := f.archiver.round(); archived != 1 || f.isArchived("task-1") || !f.isArchived("task-2") {
		t.Fatalf("archived %d, task-1 %v, task-2 %v; want task-1 refused and task-2 archived", archived, f.isArchived("task-1"), f.isArchived("task-2"))
	}
	if archived := f.archiver.round(); archived != 1 || !f.isArchived("task-1") {
		t.Errorf("the next round: archived %d, task-1 %v; want task-1", archived, f.isArchived("task-1"))
	}
}

// A round that archived a task gives the space back, vacuums once and asks
// for a snapshot; a round that archived nothing does none of it.
func TestOnlyARoundThatArchivedGivesBackSpaceAndSnapshots(t *testing.T) {
	f := newFixture(t)
	f.task(t, "task-1", tasks.Story)
	f.clock = f.done(t, "task-1").Add(age)

	f.archiver.round()
	if given, vacuumed, snapshots := f.log.givenBack.Load(), f.log.vacuumed.Load(), f.snapshots.n.Load(); given != 1 || vacuumed != 1 || snapshots != 1 {
		t.Errorf("after archiving: space given back %d times, vacuumed %d, %d snapshots; want 1 each", given, vacuumed, snapshots)
	}
	f.archiver.round()
	if given, vacuumed, snapshots := f.log.givenBack.Load(), f.log.vacuumed.Load(), f.snapshots.n.Load(); given != 1 || vacuumed != 1 || snapshots != 1 {
		t.Errorf("after archiving nothing: space given back %d times, vacuumed %d, %d snapshots; want still 1 each", given, vacuumed, snapshots)
	}
}

// Start runs a round at once and then one each time the wait ends; Close
// stops it.
func TestTheArchiverRunsAtStartAndThenHourly(t *testing.T) {
	f := newFixture(t)
	f.task(t, "task-1", tasks.Story)
	f.task(t, "task-2", tasks.Story)
	f.clock = f.done(t, "task-1").Add(age)
	waits := make(chan time.Duration)
	tick := make(chan time.Time)
	f.archiver.after = func(d time.Duration) <-chan time.Time {
		waits <- d
		return tick
	}

	f.archiver.Start()
	if wait := <-waits; wait != time.Hour || !f.isArchived("task-1") {
		t.Fatalf("after the first round: waits %v, task-1 archived %v; want an hour, and archived", wait, f.isArchived("task-1"))
	}
	f.clock = f.done(t, "task-2").Add(age)
	tick <- f.clock
	if <-waits; !f.isArchived("task-2") {
		t.Error("task-2 should be archived in the round an hour on")
	}
	f.archiver.Close()
}

// everyEvent is the whole live log, rejections included.
func everyEvent(t *testing.T, s *store.Store) []events.Event {
	t.Helper()
	var all []events.Event
	for {
		var after int64
		if len(all) > 0 {
			after = all[len(all)-1].Position
		}
		page, err := s.After(after, 1000)
		if err != nil {
			t.Fatal(err)
		}
		if len(page) == 0 {
			return all
		}
		all = append(all, page...)
	}
}

// withTheBlob is the log with an archived stream's blob put back: every
// event, in order, as before the archive, and the marker.
func withTheBlob(t *testing.T, live []events.Event, blobs sharedfiles.Disk, marker events.Event) []events.Event {
	t.Helper()
	blob, err := blobs.Open(marker.Text("archive"))
	if err != nil {
		t.Fatal(err)
	}
	defer blob.Close()
	archived, err := store.ReadArchive(blob)
	if err != nil {
		t.Fatal(err)
	}
	whole := append(archived, live...)
	slices.SortFunc(whole, func(a, b events.Event) int { return int(a.Position - b.Position) })
	return whole
}

// replayed is a new state loaded with a log, as Open loads a whole one.
func replayed(log []events.Event) *current.State {
	state := storetest.NewState(events.Core())
	var streams []string
	byStream := map[string][]events.Event{}
	for _, e := range log {
		if e.Type == events.ProposalRejected {
			continue
		}
		if _, seen := byStream[e.Stream]; !seen {
			streams = append(streams, e.Stream)
		}
		byStream[e.Stream] = append(byStream[e.Stream], e)
	}
	for _, stream := range streams {
		state.Load(stream, byStream[stream])
	}
	state.Loaded(log[len(log)-1].Position)
	return state
}
