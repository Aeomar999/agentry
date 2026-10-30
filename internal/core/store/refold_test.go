package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// notePinned is a task type that refolds its stream, as no core type does yet.
const notePinned events.Type = "note.pinned"

var withRefold = func() *events.Catalogue {
	catalogue, err := events.NewCatalogue(events.Registration{
		TaskTypes: map[events.Type]events.Spec{notePinned: {Writers: []events.ActorKind{events.Human}, Refolds: true}}})
	if err != nil {
		panic(err)
	}
	return catalogue
}()

// folds is a model that records what the store gives it, and, at each
// Refold, the position the log's readers are at.
type folds struct {
	since  int64
	store  *Store // set once Open returns
	given  []string
	readAt []int64
}

func (f *folds) Since() int64                { return f.since }
func (f *folds) Load(string, []events.Event) {}
func (f *folds) Loaded(int64)                {}

func (f *folds) Apply(e events.Event) { f.given = append(f.given, fmt.Sprintf("Apply %d", e.Position)) }

func (f *folds) Refold(stream string, before []events.Event, e events.Event) {
	var positions []int64
	for _, b := range before {
		positions = append(positions, b.Position)
	}
	f.given = append(f.given, fmt.Sprintf("Refold %s %v %d", stream, positions, e.Position))
	if f.store != nil {
		f.readAt = append(f.readAt, f.store.LastPosition())
	}
}

// refoldLog is the log both refold tests write, with note.pinned refolding:
// a refusal in task-1 at 4, a refold of each task at 5 and 6, and an
// event in task-1 after its refold, at 7.
var refoldLog = []events.Proposal{
	newTask("task-1"),
	newTask("task-2"),
	{Stream: "task-1", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Renamed"}},
	{Stream: "task-1", Actor: you, Type: events.TaskRenamed, Data: map[string]any{}}, // no title: refused
	{Stream: "task-2", Actor: you, Type: notePinned, Data: map[string]any{}},
	{Stream: "task-1", Actor: you, Type: notePinned, Data: map[string]any{}},
	{Stream: "task-1", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Again"}},
}

// What the model is given for refoldLog: each refold has exactly the
// stream's accepted events below it, the refusal left out, and is never
// also applied.
var refoldsGiven = []string{"Apply 3", "Apply 4", "Refold task-2 [2] 5", "Refold task-1 [1 3] 6", "Apply 7"}

func proposeAll(t *testing.T, s *Store, proposals []events.Proposal) {
	t.Helper()
	for _, p := range proposals {
		if _, _, err := s.Propose(p); err != nil {
			t.Fatal(err)
		}
	}
}

// A committed event that refolds its stream goes to Refold in place of
// Apply, and the reads move past it only once Refold has returned.
func TestAProposalThatRefoldsGivesTheStreamBeforeIt(t *testing.T) {
	model := &folds{}
	s, err := Open(filepath.Join(t.TempDir(), "agentry.db"), withRefold, model, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	model.store = s
	proposeAll(t, s, refoldLog)

	if want := append([]string{"Apply 1", "Apply 2"}, refoldsGiven...); !slices.Equal(model.given, want) {
		t.Errorf("the model was given %q, want %q", model.given, want)
	}
	if !slices.Equal(model.readAt, []int64{4, 5}) {
		t.Errorf("the reads were at %v during the refolds of 5 and 6, want 4 and 5", model.readAt)
	}
}

// Open's tail takes a refold as Propose does: from the stream's events
// below it, never the rows after it, which the log already holds.
func TestOpensTailRefoldsWithOnlyTheEventsBefore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, err := Open(path, withRefold, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	proposeAll(t, s, refoldLog)
	s.Close()

	model := &folds{since: 2}
	s, err = Open(path, withRefold, model, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if !slices.Equal(model.given, refoldsGiven) {
		t.Errorf("Open gave the model %q, want %q", model.given, refoldsGiven)
	}
}

// refoldBlocking is a model whose Refold says it has started, then waits
// to be released.
type refoldBlocking struct{ started, release chan struct{} }

func (refoldBlocking) Since() int64                { return 0 }
func (refoldBlocking) Load(string, []events.Event) {}
func (refoldBlocking) Loaded(int64)                {}
func (refoldBlocking) Apply(events.Event)          {}

func (b refoldBlocking) Refold(string, []events.Event, events.Event) {
	b.started <- struct{}{}
	<-b.release
}

// As test 8 for Apply: while the model refolds for event N, N is committed
// but no read returns it, LastPosition is N-1, and a reader waiting at N-1
// wakes only once Refold has returned.
func TestNoEventIsReadBeforeTheModelHasRefolded(t *testing.T) {
	model := refoldBlocking{started: make(chan struct{}, 1), release: make(chan struct{})}
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, err := Open(path, withRefold, model, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	var released sync.Once
	release := func() { released.Do(func() { close(model.release) }) }
	t.Cleanup(release) // a failing check must not leave Propose blocked
	other, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { other.Close() })
	proposeAll(t, s, refoldLog[:1])
	reader := s.NewReader(1)
	defer reader.Close()

	proposed := make(chan error, 1)
	go func() {
		_, _, err := s.Propose(events.Proposal{Stream: "task-1", Actor: you, Type: notePinned, Data: map[string]any{}})
		proposed <- err
	}()
	select {
	case <-model.started:
	case <-time.After(5 * time.Second):
		t.Fatal("the model was never asked to refold")
	}

	var committed int64
	other.QueryRow(`SELECT MAX(position) FROM events`).Scan(&committed)
	if committed != 2 {
		t.Fatalf("event 2 should be committed before the refold, the log ends at %d", committed)
	}
	if found, err := s.After(1, 10); err != nil || len(found) != 0 {
		t.Errorf("After returned an event the model hasn't refolded for yet: %+v, %v", found, err)
	}
	if position := s.LastPosition(); position != 1 {
		t.Errorf("LastPosition is %d during the refold for event 2, want 1", position)
	}
	early, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if batch, err := reader.Next(early); err != context.DeadlineExceeded {
		t.Errorf("the reader returned before the refold ended: %+v, %v", batch, err)
	}

	release()
	select {
	case err := <-proposed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Propose didn't return once the refold ended")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if batch, err := reader.Next(ctx); err != nil || len(batch) != 1 || batch[0].Position != 2 {
		t.Errorf("once refolded, the reader should return event 2: %+v, %v", batch, err)
	}
}
