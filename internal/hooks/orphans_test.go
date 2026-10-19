package hooks

import (
	"path/filepath"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// A broken task before the one with an orphaned ask doesn't stop the start:
// it is skipped, and the ask after it is still held.
func TestARestartSkipsABrokenTask(t *testing.T) {
	state := current.New()
	// No rules: they would refuse the worktree.moved that breaks task-1.
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), events.Core(), state, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	you := events.Actor{Kind: events.Human, ID: "you"}
	engineer := events.Actor{Kind: events.Agent, ID: "engineer"}
	created := map[string]any{"title": "t", "repo": "/r", "coordinator": "tech-lead"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: created},
		{Stream: "task-1", Actor: worker, Type: events.WorktreeMoved, Data: map[string]any{"worker": "local", "path": "/w", "port": 7901}},
		{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: created},
		{Stream: "task-2", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/task-2", "branch": "b", "port": 7902}},
		{Stream: "task-2", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "engineer", "hash": "h"}},
		{Stream: "task-2", Actor: engineer, Type: events.ActionRequested, Correlation: "toolu_1", Data: map[string]any{"tool": "Bash", "command": "curl x"}},
		{Stream: "task-2", Actor: ruleset, Type: events.ActionAsked, Correlation: "toolu_1", Data: map[string]any{"reason": "may reach outside"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	if state.Broken("task-1") == nil {
		t.Fatal("task-1 should be broken")
	}

	if err := HoldOrphanedAsks(s, state); err != nil {
		t.Fatal(err)
	}
	task, _ := state.Task("task-2")
	if action := task.Action("toolu_1"); action == nil || !action.Held {
		t.Errorf("task-2's ask should be held, got %+v", action)
	}
}
