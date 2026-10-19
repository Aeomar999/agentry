package storetest

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// Archive archives a task through Store.Archive, as the archiver does
// (docs/design/log-growth.md section 4): its rows moved to a blob in a
// folder of the test's, and a marker that keeps the task's summary as the
// list showed it. Only a done task is archived, so a task that isn't done
// is moved to Done first.
func Archive(t testing.TB, s *store.Store, state *current.State, id string) {
	t.Helper()
	task, err := state.Task(id)
	if err != nil || task == nil {
		t.Fatalf("archiving %s: %v, %v", id, task, err)
	}
	if !task.IsDone {
		mustPropose(t, s, events.Proposal{Stream: id, Actor: events.Actor{Kind: events.Human, ID: "you"}, Type: events.TaskDone,
			Data: map[string]any{"summary": "Done, to archive.", "moved": true}})
	}
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	if _, rejection, err := s.Archive(id, summarise(state, id), blobs); rejection != nil || err != nil {
		t.Fatalf("archiving %s: %v, %v", id, rejection, err)
	}
}

// summarise is the summariser Archive calls for a task: its summary as the
// state holds it then.
func summarise(state *current.State, id string) func() any {
	return func() any {
		task, _ := state.Task(id)
		return tasks.SummaryOf(task, state.Roster())
	}
}

func mustPropose(t testing.TB, s *store.Store, p events.Proposal) {
	t.Helper()
	if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
		t.Fatalf("%s on %s: %v, %v", p.Type, p.Stream, rejection, err)
	}
}
