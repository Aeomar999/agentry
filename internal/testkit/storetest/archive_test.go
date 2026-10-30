package storetest

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
)

// A task the state can't hand out, archived already or with its stream
// broken, is refused by the rules: Archive summarises it before they run,
// and the summariser gives nil rather than fail under the store's lock.
func TestArchivingATaskTheStateCantHandOutIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	unchecked, err := store.Open(path, events.Core(), nil, nil) // no rules, so task-2 can break
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range append(agents.SeedProposals(nil),
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "One", "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Two", "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-2", Actor: worker, Type: events.WorktreeMoved, Data: map[string]any{"worker": "laptop", "path": "/w", "port": 7901}},
	) {
		if _, rejection, err := unchecked.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	unchecked.Close()

	s, state := Open(t, path)
	if state.Broken("task-2") == nil {
		t.Fatal("task-2 should be broken")
	}
	Archive(t, s, state, "task-1")
	for id, reason := range map[string]string{"task-1": "is archived", "task-2": "could not read the task"} {
		_, rejection, err := s.Archive(id, summarise(state, id), sharedfiles.Disk{Dir: t.TempDir()})
		if err != nil || rejection == nil || !strings.Contains(rejection.Reason, reason) {
			t.Errorf("archiving %s: %v, %v; want refused as %q", id, rejection, err, reason)
		}
	}
}

// A marker the rules refuse writes nothing: a task that isn't done keeps
// every row, the stream takes no marker and no rejection, and the state is
// given nothing, so the task lists as it did.
func TestARefusedMarkerLeavesTheTaskAsItWas(t *testing.T) {
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range append(agents.SeedProposals(nil),
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "One", "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "One, renamed"}},
	) {
		mustPropose(t, s, p)
	}
	log, err := s.Log("task-1")
	if err != nil {
		t.Fatal(err)
	}
	last, applied := s.LastPosition(), state.Position()

	_, rejection, err := s.Archive("task-1", summarise(state, "task-1"), sharedfiles.Disk{Dir: t.TempDir()})
	if err != nil || rejection == nil || !strings.Contains(rejection.Reason, "isn't done") {
		t.Fatalf("archiving a task that isn't done: %v, %v; want refused", rejection, err)
	}
	if now, err := s.Log("task-1"); err != nil || !reflect.DeepEqual(now, log) {
		t.Errorf("task-1's rows after the refusal: %+v, %v; want them as they were, %+v", now, err, log)
	}
	if s.LastPosition() != last || state.Position() != applied {
		t.Errorf("the store is at %d and the state at %d; want %d and %d, nothing written or applied", s.LastPosition(), state.Position(), last, applied)
	}
	if task, err := state.Task("task-1"); task == nil || err != nil || task.Title != "One, renamed" {
		t.Errorf("task-1 should still be whole: %v, %v", task, err)
	}
}
