package store

import (
	"context"
	"database/sql"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// blocking is a model whose Apply says it has started, then waits to be released.
type blocking struct{ started, release chan struct{} }

func (blocking) Since() int64                                { return 0 }
func (blocking) Load(string, []events.Event)                 {}
func (blocking) Loaded(int64)                                {}
func (blocking) Refold(string, []events.Event, events.Event) {}

func (b blocking) Apply(events.Event) {
	b.started <- struct{}{}
	<-b.release
}

// seeded writes two tasks and a refusal to a new log, closes it and
// returns its path.
func seeded(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	s.Propose(newTask("task-1"))
	s.Propose(newTask("task-2"))
	if _, rejection, _ := s.Propose(newTask("task-1")); rejection == nil {
		t.Fatal("seeding: the second task-1 should be refused")
	}
	s.Close()
	return path
}

// Test 8: while the model is applying event N, N is committed but no read
// of the log returns it, and LastPosition is still N-1.
func TestNoEventIsReadBeforeTheModelHasIt(t *testing.T) {
	model := blocking{started: make(chan struct{}, 1), release: make(chan struct{})}
	path := seeded(t)
	s, err := Open(path, events.Core(), model, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	var released sync.Once
	release := func() { released.Do(func() { close(model.release) }) }
	t.Cleanup(release) // a failing check must not leave Propose blocked
	// A connection of the test's own shows that event 4 is committed: in WAL
	// mode it couldn't see an uncommitted one.
	other, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Close() })
	reader := s.NewReader(3)
	defer reader.Close()

	proposed := make(chan error, 1)
	go func() {
		_, _, err := s.Propose(newTask("task-3"))
		proposed <- err
	}()
	<-model.started

	var committed int64
	other.QueryRow(`SELECT MAX(position) FROM events`).Scan(&committed)
	if committed != 4 {
		t.Fatalf("event 4 should be committed before it is applied, the log ends at %d", committed)
	}
	if found, err := s.After(3, 10); err != nil || len(found) != 0 {
		t.Errorf("After returned an event the model doesn't have yet: %+v, %v", found, err)
	}
	if stream, _ := s.Stream("task-3"); len(stream) != 0 {
		t.Errorf("Stream returned an event the model doesn't have yet: %+v", stream)
	}
	if log, _ := s.Log("task-3"); len(log) != 0 {
		t.Errorf("Log returned an event the model doesn't have yet: %+v", log)
	}
	for _, types := range [][]events.Type{nil, {events.TaskCreated}} {
		if page, _ := s.LogAfter("task-3", types, 0, 10, 10); len(page) != 0 {
			t.Errorf("LogAfter of types %v returned an event the model doesn't have yet: %+v", types, page)
		}
		if page, _ := s.LogBefore("task-3", types, 0, 10, 10); len(page) != 0 {
			t.Errorf("LogBefore of types %v returned an event the model doesn't have yet: %+v", types, page)
		}
	}
	if position := s.LastPosition(); position != 3 {
		t.Errorf("LastPosition is %d while event 4 is being applied, want 3", position)
	}
	early, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if batch, err := reader.Next(early); err != context.DeadlineExceeded {
		t.Errorf("the reader returned before the model had event 4: %+v, %v", batch, err)
	}

	release()
	if err := <-proposed; err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if batch, err := reader.Next(ctx); err != nil || len(batch) != 1 || batch[0].Position != 4 {
		t.Errorf("once applied, the reader should return event 4: %+v, %v", batch, err)
	}
	if position := s.LastPosition(); position != 4 {
		t.Errorf("LastPosition is %d once event 4 is applied, want 4", position)
	}
}

// Opening an existing log starts at its last position, a refusal included,
// before any proposal, with a model or without one.
func TestOpenStartsAtTheLastPosition(t *testing.T) {
	path := seeded(t)
	model := newCreated()
	for name, model := range map[string]Model{"with a model": model, "without one": nil} {
		s, err := Open(path, events.Core(), model, nil)
		if err != nil {
			t.Fatal(err)
		}
		if position := s.LastPosition(); position != 3 {
			t.Errorf("%s: LastPosition is %d, want 3", name, position)
		}
		if found, _ := s.After(0, 10); len(found) != 3 {
			t.Errorf("%s: After(0) found %d events, want 3", name, len(found))
		}
		s.Close()
	}
	if model.loaded != 3 {
		t.Errorf("Loaded was given %d, want 3", model.loaded)
	}
}

// partWay is a model that starts at a given position, as one read from a
// snapshot would, and records what Open calls.
type partWay struct {
	since   int64
	calls   []string
	applied []int64
}

func (p *partWay) Since() int64                         { return p.since }
func (p *partWay) Load(stream string, _ []events.Event) { p.calls = append(p.calls, "Load "+stream) }
func (p *partWay) Loaded(int64)                         { p.calls = append(p.calls, "Loaded") }
func (p *partWay) Apply(e events.Event)                 { p.applied = append(p.applied, e.Position) }
func (p *partWay) Refold(stream string, _ []events.Event, _ events.Event) {
	p.calls = append(p.calls, "Refold "+stream)
}

// A model that starts part way is given only the events after its position,
// the refusal among them, and no Load or Loaded; the log's reads start at
// its last position.
func TestOpenAppliesOnlyWhatComesAfterAModelThatStartsPartWay(t *testing.T) {
	path := seeded(t)
	for since, want := range map[int64][]int64{1: {2, 3}, 2: {3}, 3: nil} {
		model := &partWay{since: since}
		s, err := Open(path, events.Core(), model, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(model.calls) > 0 {
			t.Errorf("since %d: Open called %v", since, model.calls)
		}
		if !slices.Equal(model.applied, want) {
			t.Errorf("since %d: Open applied %v, want %v", since, model.applied, want)
		}
		if position := s.LastPosition(); position != 3 {
			t.Errorf("since %d: LastPosition is %d, want 3", since, position)
		}
		s.Close()
	}
}

// A model past the log's end was made from another log: Open refuses it
// rather than read on from the wrong place.
func TestOpenRefusesAModelPastTheLogsEnd(t *testing.T) {
	if s, err := Open(seeded(t), events.Core(), &partWay{since: 4}, nil); err == nil {
		s.Close()
		t.Error("Open took a model at position 4 over a log of 3 events")
	}
}

// A write that fails never reaches the model, and the reads don't move.
// The trigger fails the insert: a failed commit can't be forced as cheaply,
// and leaves Propose by the same return. Test 8 covers the order of Apply
// and Commit.
func TestAFailedWriteNeverReachesTheModel(t *testing.T) {
	s, model := openAt(t, filepath.Join(t.TempDir(), "agentry.db"))
	s.Propose(newTask("task-1"))
	if _, err := s.db.Exec(`CREATE TRIGGER no_task_9 BEFORE INSERT ON events WHEN NEW.stream = 'task-9'
		BEGIN SELECT RAISE(ABORT, 'the disk is full'); END`); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Propose(newTask("task-9")); err == nil {
		t.Fatal("the write should have failed")
	}
	if model.has("task-9") {
		t.Error("the model applied an event that was never written")
	}
	if position := s.LastPosition(); position != 1 {
		t.Errorf("LastPosition moved to %d on a failed write", position)
	}
	if _, rejection, err := s.Propose(newTask("task-2")); err != nil || rejection != nil {
		t.Errorf("the store should carry on after a failed write: %v, %v", rejection, err)
	}
}
