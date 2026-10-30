package storetest

import (
	"path/filepath"
	"reflect"
	"slices"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
)

// Unarchiving, as the state sees it (docs/design/log-growth.md section 4,
// "Reopening an archived task"): the task comes back as a log never
// archived has it. The store's side, the rows and the refusals, is in
// store's unarchive_test.go.

// twoTasks seeds the roster and two tasks, task-1 with a few events.
func twoTasks(t testing.TB, s *store.Store) {
	t.Helper()
	for _, p := range append(agents.SeedProposals(nil),
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "One", "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Two", "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "One, renamed"}},
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskLabeled, Data: map[string]any{"labels": []any{"web"}}},
	) {
		mustPropose(t, s, p)
	}
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskRenamed, Data: map[string]any{}}); rejection == nil || err != nil {
		t.Fatal("a rename with no title should be refused:", rejection, err)
	}
}

func unarchive(t testing.TB, s *store.Store, id string, blobs store.Opener) events.Event {
	t.Helper()
	event, rejection, err := s.Unarchive(id, blobs)
	if rejection != nil || err != nil {
		t.Fatalf("unarchiving %s: %v, %v", id, rejection, err)
	}
	return event
}

func reopen(t testing.TB, s *store.Store, id string) events.Event {
	t.Helper()
	event, rejection, err := s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskReopened, Data: map[string]any{"text": "Once more."}})
	if rejection != nil || err != nil {
		t.Fatalf("reopening %s: %v, %v", id, rejection, err)
	}
	return event
}

// neverArchived is a replay of the log as if it had never been archived:
// its events now, the markers and the unarchivings left out.
func neverArchived(t testing.TB, s *store.Store) *current.State {
	t.Helper()
	all, err := s.After(0, 1_000_000)
	if err != nil {
		t.Fatal(err)
	}
	var streams []string
	byStream := map[string][]events.Event{}
	for _, e := range all {
		if slices.Contains([]events.Type{events.ProposalRejected, events.TaskArchived, events.TaskUnarchived}, e.Type) {
			continue
		}
		if _, seen := byStream[e.Stream]; !seen {
			streams = append(streams, e.Stream)
		}
		byStream[e.Stream] = append(byStream[e.Stream], e)
	}
	state := NewState(s.Catalogue())
	for _, stream := range streams {
		state.Load(stream, byStream[stream])
	}
	state.Loaded(all[len(all)-1].Position)
	return state
}

// asNeverArchived fails the test unless the state's task id is the one a
// log never archived folds to, but for the time of its last
// task.unarchived, which it keeps, and the time it was last active, which
// task.unarchived moves when it is the task's last event: active, when it
// isn't zero. The other tasks and the list must be the same too.
func asNeverArchived(t testing.TB, s *store.Store, state *current.State, id string, active time.Time) {
	t.Helper()
	never := neverArchived(t, s)
	got, err := state.Task(id)
	if err != nil || got == nil {
		t.Fatalf("%s should be whole: %v, %v", id, got, err)
	}
	unarchived, err := never.Task(id)
	if err != nil || unarchived == nil {
		t.Fatal(err)
	}
	want := unarchived.Clone()
	want.UnarchivedAt = lastUnarchived(t, s, id)
	if !active.IsZero() {
		want.LastActiveAt = active
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("%s differs from the task a log never archived has:\n%+v\nwant\n%+v", id, got, want)
	}
	if !reflect.DeepEqual(state.TaskIDs(), never.TaskIDs()) {
		t.Errorf("the tasks are %v, want %v", state.TaskIDs(), never.TaskIDs())
	}
	if !reflect.DeepEqual(mustTask(t, state, "task-2"), mustTask(t, never, "task-2")) {
		t.Error("task-2 differs from a log never archived")
	}
	if active.IsZero() && !reflect.DeepEqual(state.Summaries(), never.Summaries()) {
		t.Errorf("the list is %+v, want %+v", state.Summaries(), never.Summaries())
	}
}

// lastUnarchived is when the stream was last unarchived, from its log.
func lastUnarchived(t testing.TB, s *store.Store, id string) time.Time {
	t.Helper()
	log, err := s.Log(id)
	if err != nil {
		t.Fatal(err)
	}
	var at time.Time
	for _, e := range log {
		if e.Type == events.TaskUnarchived {
			at = e.At
		}
	}
	return at
}

func mustTask(t testing.TB, state *current.State, id string) *tasks.Task {
	t.Helper()
	task, err := state.Task(id)
	if err != nil {
		t.Fatal(err)
	}
	return task
}

// The step's proof: archiving a task and unarchiving it gives the task a
// log never archived has, task.unarchived's time aside, and the same again
// once it is reopened, which every reopening of an archived task does.
// Open's checks then hold the state equal to a replay of the log, and to
// snapshots of it at every position.
func TestAnUnarchivedTaskIsAsIfNeverArchived(t *testing.T) {
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	twoTasks(t, s)
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	ArchiveIn(t, s, state, "task-1", blobs)

	unarchived := unarchive(t, s, "task-1", blobs)
	asNeverArchived(t, s, state, "task-1", unarchived.At)
	reopen(t, s, "task-1")
	asNeverArchived(t, s, state, "task-1", time.Time{})
}

// openChecked opens the log at path with the rules, as serve does, for a
// test that closes it itself.
func openChecked(t testing.TB, path string, state *current.State) *store.Store {
	t.Helper()
	catalogue := events.Core()
	s, err := store.Open(path, catalogue, state, func(p events.Proposal) *events.Rejection { return rules.Check(p, state, catalogue) })
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// A snapshot taken while the task was archived, and a restart after it was
// unarchived and reopened: Open goes on from the snapshot, refolds the
// task from the restored rows at task.unarchived in the tail, and ends
// equal to a full replay of the log.
func TestARestartWithTaskUnarchivedInTheSnapshotsTail(t *testing.T) {
	dir := t.TempDir()
	path, snapshot := filepath.Join(dir, "agentry.db"), filepath.Join(dir, "state.snapshot")
	state := NewState(events.Core())
	s := openChecked(t, path, state)
	twoTasks(t, s)
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	ArchiveIn(t, s, state, "task-1", blobs)
	if err := current.WriteSnapshot(snapshot, state, s, "build-1"); err != nil {
		t.Fatal(err)
	}
	taken := s.LastPosition()
	unarchive(t, s, "task-1", blobs)
	reopen(t, s, "task-1")
	s.Close()

	restored := current.Restore(snapshot, "build-1", events.Core(), ideas.Streams())
	s = openChecked(t, path, restored)
	defer s.Close()
	if restored.Resumed() != taken {
		t.Fatalf("Open went on from %d, want the snapshot's %d", restored.Resumed(), taken)
	}
	if err := current.Equal(s, restored); err != nil {
		t.Errorf("the restarted state differs from a replay of the log: %v", err)
	}
	asNeverArchived(t, s, restored, "task-1", time.Time{})
}

// Archived, unarchived, changed, archived again and unarchived: the second
// marker takes every row, the first marker and task.unarchived among them,
// into a new blob; the first blob stays, named by the first marker, which
// is back among the rows; and the task is as a log never archived has it.
func TestArchivingTwiceAndUnarchiving(t *testing.T) {
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	twoTasks(t, s)
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	ArchiveIn(t, s, state, "task-1", blobs)
	first, _ := store.MarkerOf(mustLog(t, s, "task-1"))
	unarchive(t, s, "task-1", blobs)
	reopen(t, s, "task-1")

	rows := mustLog(t, s, "task-1")
	ArchiveIn(t, s, state, "task-1", blobs) // moves it to Done first
	second, _ := store.MarkerOf(mustLog(t, s, "task-1"))
	if second.Number("count") != len(rows)+1 || second.Text("archive") == first.Text("archive") {
		t.Errorf("the second marker: %+v; want all %d rows and the move to Done in a new blob", second, len(rows))
	}

	unarchived := unarchive(t, s, "task-1", blobs)
	asNeverArchived(t, s, state, "task-1", unarchived.At)
	log := mustLog(t, s, "task-1")
	at := slices.IndexFunc(log, func(e events.Event) bool { return e.Position == first.Position })
	if at < 0 || !reflect.DeepEqual(log[at], first) || !blobs.Has(first.Text("archive")) {
		t.Errorf("the first marker should be back among the rows, and its blob kept: %v", at)
	}
}

func mustLog(t testing.TB, s *store.Store, stream string) []events.Event {
	t.Helper()
	log, err := s.Log(stream)
	if err != nil {
		t.Fatal(err)
	}
	return log
}
