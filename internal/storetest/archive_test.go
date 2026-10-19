package storetest

import (
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/store"
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
