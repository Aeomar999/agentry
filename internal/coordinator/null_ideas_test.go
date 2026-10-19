package coordinator

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// A Software done whose "ideas" is null carries no ideas: it goes through
// with its summary as given, and no note.
func TestASoftwareDoneWithNullIdeasHasNoNote(t *testing.T) {
	s, state, repo := withVisionaries(t)
	const id = "task-1"
	startTask(t, s, id, repo, "tech-lead")

	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "done", "summary": "Built it.", "ideas": null}`)
	task := waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if task.Summary != "Built it." {
		t.Errorf("summary %q", task.Summary)
	}
	if kept := state.OwnedIDs(ideas.Prefix); len(kept) != 0 {
		t.Errorf("no idea should be written: %v", kept)
	}
}
