package archive

import (
	"database/sql"
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

// counted is the store with its calls after a round counted, whose next
// VacuumOnce fails when failVacuum is set. vacuumRan counts the VACUUMs
// that ran.
type counted struct {
	*store.Store
	givenBack, vacuumed, vacuumRan atomic.Int32
	failVacuum                     atomic.Bool
}

func (c *counted) GiveBackSpace() error {
	c.givenBack.Add(1)
	return c.Store.GiveBackSpace()
}

func (c *counted) VacuumOnce() (bool, error) {
	c.vacuumed.Add(1)
	if c.failVacuum.CompareAndSwap(true, false) {
		return false, errors.New("database or disk is full")
	}
	ran, err := c.Store.VacuumOnce()
	if ran {
		c.vacuumRan.Add(1)
	}
	return ran, err
}

// stalling is the counted store whose first Archive tells entered and then
// waits for stop before it archives: a round under way when Close comes.
type stalling struct {
	*counted
	archives atomic.Int32
	entered  chan struct{}
	stop     <-chan struct{}
}

func (s *stalling) Archive(stream string, summarise func() any, blobs store.Blobs) (events.Event, *events.Rejection, error) {
	if s.archives.Add(1) == 1 {
		close(s.entered)
		<-s.stop
	}
	return s.counted.Archive(stream, summarise, blobs)
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
	f := openFixture(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		f.propose(t, p)
	}
	return f
}

// oldFile is a log made before incremental mode: a table made before the
// store opens it fixes the file's auto_vacuum at NONE.
func oldFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agentry.db")
	db, err := sql.Open(store.Driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE made_before (x)`); err != nil {
		t.Fatal(err)
	}
	return path
}

// openFixture is the fixture of the log at path, as it is.
func openFixture(t *testing.T, path string) *fixture {
	t.Helper()
	s, state := storetest.Open(t, path)
	f := &fixture{log: &counted{Store: s}, state: state, blobs: sharedfiles.Disk{Dir: t.TempDir()}, snapshots: &asked{}}
	f.archiver = New(f.log, state, f.blobs, f.snapshots, age)
	f.archiver.now = func() time.Time { return f.clock }
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

// A task unarchived and not reopened is still done, as long ago as
// before, so its age counts again from its task.unarchived: a round when
// it is done for more than the age, but less than the age after the
// unarchiving, leaves the task whole, and a round the age after the
// unarchiving archives it again.
func TestAnUnarchivedTaskWaitsTheAgeAgain(t *testing.T) {
	f := newFixture(t)
	f.task(t, "task-1", tasks.Story)
	done := f.done(t, "task-1")
	f.clock = done.Add(age)
	if archived := f.archiver.round(); archived != 1 || !f.isArchived("task-1") {
		t.Fatalf("archived %d, task-1 %v; want task-1", archived, f.isArchived("task-1"))
	}
	unarchived, rejection, err := f.log.Unarchive("task-1", f.blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}

	if !unarchived.At.After(done) {
		t.Fatalf("unarchived at %v, not after done at %v", unarchived.At, done)
	}
	f.clock = done.Add(age).Add(unarchived.At.Sub(done) / 2)
	if archived := f.archiver.round(); archived != 0 || f.isArchived("task-1") {
		t.Errorf("just unarchived: archived %d, task-1 %v; want it left whole", archived, f.isArchived("task-1"))
	}
	f.clock = unarchived.At.Add(age)
	if archived := f.archiver.round(); archived != 1 || !f.isArchived("task-1") {
		t.Errorf("the age after the unarchiving: archived %d, task-1 %v; want it archived again", archived, f.isArchived("task-1"))
	}
}

// Stories go first: an epic with a story still in the state waits, and
// the round after its last story is archived archives it. A broken story
// is still in the state, so its epic waits too.
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

	f = withABrokenStory(t)
	f.clock = f.done(t, "task-1").Add(age)
	if archived := f.archiver.round(); archived != 0 || f.isArchived("task-1") {
		t.Errorf("with task-2 broken: archived %d, epic %v; want the epic left", archived, f.isArchived("task-1"))
	}
}

// An erased story is gone from the state for good, so its epic doesn't
// wait for it, as it waits for a broken one.
func TestAnEpicDoesNotWaitForAnErasedStory(t *testing.T) {
	f := newFixture(t)
	f.task(t, "task-1", tasks.Epic)
	f.story(t, "task-2", "task-1")
	if _, rejection, err := f.log.Erase("task-2", you, "Asked to be forgotten.", f.blobs); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	f.clock = f.done(t, "task-1").Add(age)
	if archived := f.archiver.round(); archived != 1 || !f.isArchived("task-1") {
		t.Errorf("with task-2 erased: archived %d, epic %v; want the epic", archived, f.isArchived("task-1"))
	}
}

// withABrokenStory is a fixture whose epic task-1 has one story, task-2,
// broken by a worktree on a worker it doesn't have, which only a log
// without rules takes.
func withABrokenStory(t *testing.T) *fixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agentry.db")
	unchecked, err := store.Open(path, events.Core(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	created := func(id string, kind tasks.Kind) events.Proposal {
		return events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
			Data: map[string]any{"title": "Title of " + id, "repo": "/r", "coordinator": "tech-lead", "kind": string(kind)}}
	}
	for _, p := range append(agents.SeedProposals(nil),
		created("task-1", tasks.Epic),
		created("task-2", tasks.Story),
		events.Proposal{Stream: "task-1", Actor: you, Type: events.StoryAdded, Data: map[string]any{"story": "task-2", "title": "Title of task-2"}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.EpicJoined, Data: map[string]any{"epic": "task-1"}},
		events.Proposal{Stream: "task-2", Actor: events.Actor{Kind: events.Worker, ID: "local"}, Type: events.WorktreeMoved,
			Data: map[string]any{"worker": "laptop", "path": "/w", "port": 7901}},
	) {
		if _, rejection, err := unchecked.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	unchecked.Close()
	f := openFixture(t, path)
	if f.state.Broken("task-2") == nil {
		t.Fatal("task-2 should be broken")
	}
	return f
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

// A done task whose wrap-up turn is still open isn't chosen, as the rules
// would refuse it; the first round after the turn ends archives it.
func TestATaskWithATurnRunningWaitsForTheRoundAfterItEnds(t *testing.T) {
	f := newFixture(t)
	f.task(t, "task-1", tasks.Story)
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	f.propose(t, events.Proposal{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/task-1", "branch": "b", "port": 7901}})
	f.propose(t, events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}})
	f.clock = f.done(t, "task-1").Add(age) // stops turn-1, and turn-2 wraps up
	f.propose(t, events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-2", Data: map[string]any{"agent": "tech-lead", "hash": "h"}})

	if chosen := f.archiver.chosen(); len(chosen) != 0 {
		t.Errorf("chosen %v while task-1's turn runs, want none", chosen)
	}
	if archived := f.archiver.round(); archived != 0 || f.isArchived("task-1") {
		t.Fatalf("archived %d while task-1's turn runs, want none", archived)
	}
	f.propose(t, events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-2", Data: map[string]any{"agent": "tech-lead", "report": "Wrapped up."}})
	if archived := f.archiver.round(); archived != 1 || !f.isArchived("task-1") {
		t.Errorf("the round after the turn ended: archived %d, task-1 %v; want task-1", archived, f.isArchived("task-1"))
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

// A round that archived a task gives the space back and asks for a
// snapshot; a round that archived nothing does neither. Both try the
// VACUUM.
func TestOnlyARoundThatArchivedGivesBackSpaceAndSnapshots(t *testing.T) {
	f := newFixture(t)
	f.task(t, "task-1", tasks.Story)
	f.clock = f.done(t, "task-1").Add(age)

	f.archiver.round()
	if given, vacuumed, snapshots := f.log.givenBack.Load(), f.log.vacuumed.Load(), f.snapshots.n.Load(); given != 1 || vacuumed != 1 || snapshots != 1 {
		t.Errorf("after archiving: space given back %d times, vacuumed %d, %d snapshots; want 1 each", given, vacuumed, snapshots)
	}
	f.archiver.round()
	if given, vacuumed, snapshots := f.log.givenBack.Load(), f.log.vacuumed.Load(), f.snapshots.n.Load(); given != 1 || vacuumed != 2 || snapshots != 1 {
		t.Errorf("after archiving nothing: space given back %d times, vacuumed %d, %d snapshots; want still 1, vacuumed 2, 1", given, vacuumed, snapshots)
	}
}

// A VACUUM that fails is tried again at the next round, even one that
// archives nothing, and runs then.
func TestAFailedVacuumIsTriedAgainAtTheNextRound(t *testing.T) {
	f := openFixture(t, oldFile(t))
	for _, p := range agents.SeedProposals(nil) {
		f.propose(t, p)
	}
	f.task(t, "task-1", tasks.Story)
	f.clock = f.done(t, "task-1").Add(age)
	f.log.failVacuum.Store(true)

	if archived := f.archiver.round(); archived != 1 || f.log.vacuumed.Load() != 1 || f.log.failVacuum.Load() {
		t.Fatalf("archived %d, vacuumed %d, failed %v; want task-1 archived and the VACUUM tried and failed", archived, f.log.vacuumed.Load(), !f.log.failVacuum.Load())
	}
	if archived := f.archiver.round(); archived != 0 || f.log.vacuumed.Load() != 2 || f.log.vacuumRan.Load() != 1 {
		t.Errorf("the next round: archived %d, vacuumed %d, ran %d; want nothing archived and the VACUUM tried again and run", archived, f.log.vacuumed.Load(), f.log.vacuumRan.Load())
	}
}

// Close during a round stops it after the task under way: the tasks after
// it wait for the next start, and so does the VACUUM. The task under way
// still gives its space back and asks for a snapshot.
func TestCloseStopsARoundAfterTheTaskUnderWay(t *testing.T) {
	f := newFixture(t)
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		f.task(t, id, tasks.Story)
		f.clock = f.done(t, id).Add(age)
	}
	log := &stalling{counted: f.log, entered: make(chan struct{}), stop: f.archiver.stop}
	f.archiver.log = log

	f.archiver.Start()
	<-log.entered
	closed := make(chan struct{})
	go func() {
		f.archiver.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(10 * time.Second):
		t.Fatal("Close didn't return")
	}
	if archives, vacuumed, given, snapshots := log.archives.Load(), f.log.vacuumed.Load(), f.log.givenBack.Load(), f.snapshots.n.Load(); archives != 1 || vacuumed != 0 || given != 1 || snapshots != 1 {
		t.Errorf("Archive called %d times, vacuumed %d, space given back %d, %d snapshots; want 1, 0, 1, 1", archives, vacuumed, given, snapshots)
	}
	if !f.isArchived("task-1") || f.isArchived("task-2") || f.isArchived("task-3") {
		t.Errorf("task-1 %v, task-2 %v, task-3 %v; want task-1 archived alone", f.isArchived("task-1"), f.isArchived("task-2"), f.isArchived("task-3"))
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
