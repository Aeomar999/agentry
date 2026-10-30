package watchdog

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

func TestDifferentSearchesAreNotALoop(t *testing.T) {
	turn := &tasks.Turn{ID: "turn-7"}
	calls := func(commands ...string) *tasks.Task {
		task := &tasks.Task{}
		for _, c := range commands {
			task.Actions = append(task.Actions, &tasks.Action{Turn: "turn-7", Tool: "WebSearch", Command: c})
		}
		return task
	}
	if _, times := repeatedCommand(calls("search: a", "search: b", "search: c", "search: d", "search: e"), turn); times != 1 {
		t.Errorf("five different searches counted as %d repeats", times)
	}
	if _, times := repeatedCommand(calls("", "", "", "", ""), turn); times != 0 {
		t.Errorf("calls Agentry can't read counted as %d repeats", times)
	}
	if command, times := repeatedCommand(calls("search: a", "search: a", "search: a", "search: a", "search: a"), turn); times != 5 || command != "WebSearch: search: a" {
		t.Errorf("the same search five times: %q %d", command, times)
	}
}
