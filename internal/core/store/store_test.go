package store

import (
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

var you = events.Actor{Kind: events.Human, ID: "you"}

func newTask(id string) events.Proposal {
	return events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Remove the TODOs", "repo": "/src/app", "coordinator": "tech-lead"}}
}

// created is a stand-in model: the streams that have a task.created.
type created struct {
	mu      sync.Mutex
	streams map[string]bool
	loaded  int64 // the position Loaded was given
	applied int   // how many events it was given
}

func newCreated() *created { return &created{streams: map[string]bool{}} }

func (c *created) Since() int64 { return 0 }

func (c *created) Load(stream string, accepted []events.Event) {
	for _, e := range accepted {
		c.Apply(e)
	}
}

func (c *created) Loaded(position int64) { c.loaded = position }

func (c *created) Apply(e events.Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.applied++
	if e.Type == events.TaskCreated {
		c.streams[e.Stream] = true
	}
}

func (c *created) Refold(stream string, before []events.Event, e events.Event) {
	c.Load(stream, append(before, e))
}

func (c *created) has(stream string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.streams[stream]
}

// onlyOnce is a stand-in order check: a task can be created once.
func onlyOnce(model *created) OrderCheck {
	return func(p events.Proposal) *events.Rejection {
		if p.Type == events.TaskCreated && model.has(p.Stream) {
			return &events.Rejection{Check: "order", Reason: "task already exists"}
		}
		return nil
	}
}

// noteWritten is the one type of an extension's streams, note-N, that the
// test stores know beside the core's.
const noteWritten events.Type = "note.written"

var withNotes = func() *events.Catalogue {
	catalogue, err := events.NewCatalogue(events.Registration{Prefix: "note-",
		Types: map[events.Type]events.Spec{noteWritten: {Writers: []events.ActorKind{events.System}}}})
	if err != nil {
		panic(err)
	}
	return catalogue
}()

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, _ := openAt(t, filepath.Join(t.TempDir(), "agentry.db"))
	return s
}

// openAt opens the log at path with the stand-in model and check.
func openAt(t *testing.T, path string) (*Store, *created) {
	t.Helper()
	model := newCreated()
	s, err := Open(path, withNotes, model, onlyOnce(model))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, model
}

func TestProposeAppendsOrRecordsTheRejection(t *testing.T) {
	s := openTestStore(t)

	accepted, rejection, err := s.Propose(newTask("task-1"))
	if err != nil || rejection != nil || accepted.Position != 1 {
		t.Fatalf("first: %+v %v %v", accepted, rejection, err)
	}
	refused, rejection, err := s.Propose(newTask("task-1"))
	if err != nil || rejection == nil || rejection.Check != "order" || refused.Type != events.ProposalRejected {
		t.Fatalf("second: %+v %v %v", refused, rejection, err)
	}
	if _, rejection, _ := s.Propose(events.Proposal{Stream: "task-1", Actor: you, Type: "task.exploded"}); rejection == nil || rejection.Check != "shape" {
		t.Errorf("unknown type: %v", rejection)
	}

	stream, _ := s.Stream("task-1")
	if len(stream) != 1 || stream[0].Text("title") != "Remove the TODOs" {
		t.Errorf("the stream should hold only the accepted event, got %+v", stream)
	}
	all, _ := s.After(0, 100)
	if len(all) != 3 || all[1].Text("reason") != "task already exists" || all[1].Text("by") != "human:you" {
		t.Errorf("the log should hold both rejections, got %+v", all)
	}
}

func TestEventsCannotBeChangedOrDeleted(t *testing.T) {
	s := openTestStore(t)
	s.Propose(newTask("task-1"))
	for _, statement := range []string{`UPDATE events SET type = 'x'`, `DELETE FROM events`} {
		if _, err := s.db.Exec(statement); err == nil {
			t.Errorf("%s succeeded; events must be append-only", statement)
		}
	}
}

// Proof 05: twenty proposals racing for the same task, exactly one accepted.
func TestRacingProposalsAcceptExactlyOne(t *testing.T) {
	s := openTestStore(t)
	var wait sync.WaitGroup
	var mu sync.Mutex
	accepted := 0
	for range 20 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			_, rejection, err := s.Propose(newTask("task-1"))
			if err != nil {
				t.Error(err)
			}
			if rejection == nil {
				mu.Lock()
				accepted++
				mu.Unlock()
			}
		}()
	}
	wait.Wait()
	if accepted != 1 {
		t.Errorf("%d of 20 accepted, want exactly 1", accepted)
	}
}

func TestReopenKeepsTheLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	first, _ := openAt(t, path)
	first.Propose(newTask("task-1"))
	first.Propose(newTask("task-2"))
	first.Close()

	second, model := openAt(t, path)
	log, _ := second.After(0, 10)
	if len(log) != 2 || log[0].Stream != "task-1" || log[1].Stream != "task-2" {
		t.Errorf("the log after reopening: %v", log)
	}
	if !model.has("task-1") || !model.has("task-2") || model.loaded != 2 {
		t.Errorf("the model wasn't loaded from the log: %v, loaded at %d", model.streams, model.loaded)
	}
	if _, rejection, _ := second.Propose(newTask("task-1")); rejection == nil {
		t.Error("the check didn't read the loaded model")
	}
}

func TestCreateNamesTheNextStreamOfItsKind(t *testing.T) {
	s := openTestStore(t)
	note := events.Proposal{Actor: events.Actor{Kind: events.System, ID: "agentry"}, Type: noteWritten}
	var names []string
	for _, p := range []events.Proposal{newTask(""), note, newTask(""), note} {
		prefix := "task-"
		if p.Type == noteWritten {
			prefix = "note-"
		}
		created, rejection, err := s.Create(prefix, p)
		if rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
		names = append(names, created.Stream)
	}
	if want := []string{"task-1", "note-1", "task-2", "note-2"}; !slices.Equal(names, want) {
		t.Errorf("made %v, want %v", names, want)
	}
}

// Creates racing each other each take a name of their own.
func TestRacingCreatesTakeDistinctNames(t *testing.T) {
	s := openTestStore(t)
	var wait sync.WaitGroup
	var mu sync.Mutex
	var names []string
	for range 20 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			created, rejection, err := s.Create("task-", newTask(""))
			if rejection != nil || err != nil {
				t.Error(rejection, err)
				return
			}
			mu.Lock()
			names = append(names, created.Stream)
			mu.Unlock()
		}()
	}
	wait.Wait()
	slices.Sort(names)
	if distinct := len(slices.Compact(names)); distinct != 20 {
		t.Errorf("20 Creates made %d distinct names", distinct)
	}
}

// A refused Create leaves its name for the next one.
func TestARefusedCreateTakesNoName(t *testing.T) {
	s := openTestStore(t)
	untitled := newTask("")
	delete(untitled.Data, "title")
	refused, rejection, err := s.Create("task-", untitled)
	if err != nil || rejection == nil || refused.Stream != "task-1" {
		t.Fatalf("a task.created without a title should be refused in task-1: %+v %v %v", refused, rejection, err)
	}
	if created, rejection, err := s.Create("task-", newTask("")); err != nil || rejection != nil || created.Stream != "task-1" {
		t.Errorf("the next Create should take task-1: %+v %v %v", created, rejection, err)
	}
}
