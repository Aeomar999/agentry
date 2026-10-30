package storetest

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// Archive archives a task through Store.Archive, as the archiver does
// (docs/design/log-growth.md section 4): its rows moved to a blob in a
// folder of the test's, and a marker that keeps the task's summary as the
// list showed it. Only a done task is archived, so a task that isn't done
// is moved to Done first.
func Archive(t testing.TB, s *store.Store, state *current.State, id string) {
	t.Helper()
	ArchiveIn(t, s, state, id, sharedfiles.Disk{Dir: t.TempDir()})
}

// ArchiveIn is Archive with the blob put in blobs, where a reader of the
// task's history finds it.
func ArchiveIn(t testing.TB, s *store.Store, state *current.State, id string, blobs store.Blobs) {
	t.Helper()
	task, err := state.Task(id)
	if err != nil || task == nil {
		t.Fatalf("archiving %s: %v, %v", id, task, err)
	}
	if !task.IsDone {
		mustPropose(t, s, events.Proposal{Stream: id, Actor: events.Actor{Kind: events.Human, ID: "you"}, Type: events.TaskDone,
			Data: map[string]any{"summary": "Done, to archive.", "moved": true}})
	}
	if _, rejection, err := s.Archive(id, summarise(state, id), blobs); rejection != nil || err != nil {
		t.Fatalf("archiving %s: %v, %v", id, rejection, err)
	}
}

// summarise is the summariser Archive calls for a task: its summary as the
// state holds it then.
func summarise(state *current.State, id string) func() any {
	return func() any {
		task, err := state.Task(id)
		if task == nil || err != nil {
			return nil // archived, broken or missing: the rules refuse the marker
		}
		return tasks.SummaryOf(task, state.Roster())
	}
}

func mustPropose(t testing.TB, s *store.Store, p events.Proposal) {
	t.Helper()
	if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
		t.Fatalf("%s on %s: %v, %v", p.Type, p.Stream, rejection, err)
	}
}
