package tasks

import (
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// A worktree.moved before any worktree.ready panics in apply (the rules
// never let one through): the replay stops there and says where.
func TestAStreamThatCantBeReplayedSaysWhere(t *testing.T) {
	log := logOf(
		said(byYou, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead"}),
		said(byWorker, events.WorktreeMoved, "", map[string]any{"worker": "laptop", "path": "/w", "port": 7901}),
		said(byLead, events.TaskRenamed, "", map[string]any{"title": "never applied"}),
	)
	task, err := Replay(log)
	if task != nil || err == nil {
		t.Fatalf("got %+v, %v: want no task and the error", task, err)
	}
	for _, want := range []string{"worktree.moved", "task-1", "at 2"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("%q doesn't say %q", err, want)
		}
	}
	if task := FromEvents(log); task != nil {
		t.Errorf("FromEvents gave %+v, want no task", task)
	}
	if task, err := Replay(log[:1]); err != nil || task == nil || task.Title != "t" {
		t.Errorf("the stream up to the bad event replays: %+v, %v", task, err)
	}
}
