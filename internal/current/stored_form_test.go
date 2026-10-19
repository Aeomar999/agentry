package current

import (
	"path/filepath"
	"reflect"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// Test 6: proposals carry Go values that the log keeps as JSON ([]string{}
// comes back as []any{}, an int64 as a float64). The state the store builds
// live from them equals a replay of the log, and the state a restart loads.
func TestTheLiveStateEqualsARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	live := New()
	s, err := store.Open(path, withIdeas, live, nil)
	if err != nil {
		t.Fatal(err)
	}
	nested := map[string]any{"ports": []int64{7901, 7902}, "inner": map[string]any{"labels": []string{}, "at": int64(3)}}
	for _, p := range []events.Proposal{
		proposal(events.AgentsStream, you, events.AgentCreated, map[string]any{"name": "tech-lead", "prompt": "Lead.", "engine": "claude",
			"coordinates": true, "workers": []string{}, "extra": nested}),
		proposal("task-1", you, events.TaskCreated, map[string]any{"title": "Stored form", "repo": "/r", "coordinator": "tech-lead",
			"labels": []string{}, "budget": int64(5_000_000), "extra": nested}),
		proposal("task-1", you, events.TaskLabeled, map[string]any{"labels": []string{"web", "api"}}),
		proposal("task-1", you, events.TaskLabeled, map[string]any{"labels": []string{}}),
		proposal("task-1", worker, events.WorktreeReady, map[string]any{"path": "/w", "branch": "agentry/task-1", "port": int64(7901)}),
	} {
		propose(t, s, p)
	}
	if err := Equal(s, live); err != nil {
		t.Errorf("live: %v", err)
	}
	s.Close()

	restarted := New()
	again, err := store.Open(path, withIdeas, restarted, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { again.Close() })
	if err := Equal(again, restarted); err != nil {
		t.Errorf("restarted: %v", err)
	}
	liveTask, _ := live.Task("task-1")
	restartedTask, _ := restarted.Task("task-1")
	if !reflect.DeepEqual(liveTask, restartedTask) {
		t.Errorf("task-1 live:\n%+v\nafter a restart:\n%+v", liveTask, restartedTask)
	}
	if !reflect.DeepEqual(live.Roster(), restarted.Roster()) {
		t.Errorf("agents live:\n%+v\nafter a restart:\n%+v", live.Roster(), restarted.Roster())
	}
}
