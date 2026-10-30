package tasks

import "testing"

func TestOnlyAFinishedApprovedPushIsPushed(t *testing.T) {
	push := "git push origin HEAD:dev"
	task := &Task{Actions: []*Action{
		{Turn: "turn-1", Command: push, Decision: Approved, HasResult: true},
		{Turn: "turn-2", Command: push, Decision: Approved}, // failed, or still running
		{Turn: "turn-3", Command: push, Decision: Denied},
		{Turn: "turn-4", Command: "git status", Decision: Approved, HasResult: true},
		{Turn: "turn-5", Command: "  " + push, Decision: Approved, HasResult: true},
	}}
	for turn, want := range map[string]bool{"turn-1": true, "turn-2": false, "turn-3": false, "turn-4": false, "turn-5": true, "turn-6": false} {
		if got := task.Pushed(turn); got != want {
			t.Errorf("Pushed(%s) = %v, want %v", turn, got, want)
		}
	}
}
