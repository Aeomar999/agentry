package tasks

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// pushThenDone is a task whose engineer asks to git push, the decision
// coming as given, then the push's result if it had one, and the task done.
func pushThenDone(decision events.Type, result bool) []events.Event {
	steps := []events.Proposal{
		said(byYou, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead"}),
		said(byLead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"}),
		said(byWorker, events.TurnStarted, "turn-1", map[string]any{"agent": "engineer", "hash": "h"}),
		said(byEngineer, events.ActionRequested, "toolu_1", map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"}),
		said(byRules, decision, "toolu_1", map[string]any{"reason": "otherwise"}),
	}
	if result {
		steps = append(steps, said(byWorker, events.ActionResult, "toolu_1", nil))
	}
	steps = append(steps,
		said(byWorker, events.TurnEnded, "turn-1", map[string]any{"agent": "engineer", "report": "Pushed.", "tokens": 10}),
		said(byLead, events.TaskDone, "", map[string]any{"summary": "Done."}))
	return logOf(steps...)
}

func TestOnlyATaskWithAFinishedApprovedPushIsPushed(t *testing.T) {
	for name, c := range map[string]struct {
		log  []events.Event
		want bool
	}{
		"approved and finished": {pushThenDone(events.ActionApproved, true), true},
		"denied":                {pushThenDone(events.ActionDenied, false), false},
		"no result":             {pushThenDone(events.ActionApproved, false), false},
	} {
		if got := SummaryOf(FromEvents(c.log), nil).Pushed; got != c.want {
			t.Errorf("%s: pushed %v, want %v", name, got, c.want)
		}
	}
}

func TestATaskYouMovedToDoneIsDoneByYouUntilReopened(t *testing.T) {
	log := logOf(
		said(byYou, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead"}),
		said(byYou, events.TaskDone, "", map[string]any{"summary": "Moved.", "moved": true}))
	if summary := SummaryOf(FromEvents(log), nil); !summary.DoneByYou || summary.Pushed {
		t.Errorf("moved to Done: done by you %v, pushed %v; want done by you, not pushed", summary.DoneByYou, summary.Pushed)
	}
	log = append(log, events.Event{Position: 3, Proposal: said(byYou, events.TaskReopened, "", map[string]any{"text": "Again."})})
	log[2].Stream = "task-1"
	if summary := SummaryOf(FromEvents(log), nil); summary.DoneByYou {
		t.Error("reopened: still done by you")
	}
}
