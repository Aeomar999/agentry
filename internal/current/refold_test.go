package current

import (
	"slices"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
)

// A refold rebuilds its stream from the events before it and itself, as a
// replay does, whatever the state held for the stream: for each stream, the
// state is loaded without the stream's last event, a task archived as well,
// and that event comes last, as a refold.
func TestARefoldEqualsAReplay(t *testing.T) {
	for _, stream := range streamsOf(everyStream()) {
		log := everyStream()
		own := accepted(log, stream)
		if len(own) == 0 {
			continue // task-9 has only a refusal
		}
		last := own[len(own)-1]
		log = slices.DeleteFunc(log, func(e events.Event) bool { return e.Position == last.Position })
		state := loaded(log)
		if withIdeas.IsTaskStream(stream) {
			state.Archive(stream)
		}
		last.Position = everyStream()[len(everyStream())-1].Position + 1
		state.Refold(stream, own[:len(own)-1], last)
		t.Run(stream, func(t *testing.T) { sameAsReplay(t, state, append(log, last)) })
	}
}

// A refold of a broken stream folds the event that broke it again, so the
// stream stays broken, as its replay does.
func TestARefoldOfABrokenStreamStaysBroken(t *testing.T) {
	log := logOf(
		proposal("task-1", you, events.TaskCreated, map[string]any{"title": "One", "repo": "/r", "coordinator": "tech-lead"}),
		proposal("task-1", worker, events.WorktreeMoved, map[string]any{"worker": "laptop", "path": "/w", "port": float64(7901)}),
		proposal("task-1", you, events.TaskRenamed, map[string]any{"title": "Renamed"}),
	)
	state := loaded(log[:2])
	if state.Broken("task-1") == nil {
		t.Fatal("task-1 should be broken")
	}
	state.Refold("task-1", log[:2], log[2])
	if _, err := state.Task("task-1"); err == nil {
		t.Error("the refold mended a stream its replay can't build")
	}
	if state.Position() != 3 || state.StreamPosition("task-1") != 3 {
		t.Errorf("the refold left the position at %d, task-1's at %d", state.Position(), state.StreamPosition("task-1"))
	}
}

// After a refold the state still equals a replay of the log: an archived
// task is back in full, and a broken one is still broken.
func TestEqualHoldsAfterARefold(t *testing.T) {
	s := openStore(t, someProposals()...)
	state := New(withIdeas, ideas.Streams())
	state.Loaded(0)
	fromTheLog(t, s, state)
	state.Archive("task-1")
	for _, stream := range []string{"task-1", "task-3"} {
		before, err := s.Stream(stream)
		if err != nil {
			t.Fatal(err)
		}
		e, _, err := s.Propose(proposal(stream, you, events.TaskLabeled, map[string]any{"labels": []any{"refolded"}}))
		if err != nil {
			t.Fatal(err)
		}
		state.Refold(stream, before, e)
	}
	if err := Equal(s, state); err != nil {
		t.Error(err)
	}
	if task, err := state.Task("task-1"); err != nil || task == nil || !slices.Equal(task.Labels, []string{"refolded"}) {
		t.Errorf("task-1 after its refold: %+v, %v", task, err)
	}
	if state.Broken("task-3") == nil {
		t.Error("task-3 should still be broken")
	}
}
