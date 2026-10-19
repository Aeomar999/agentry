package tasks

import (
	"fmt"
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

// engineerTurn is the engineer's turn-N, which runs git push to the end
// when push is set and only ls otherwise.
func engineerTurn(n int, push bool) []events.Proposal {
	turn, action, command := fmt.Sprintf("turn-%d", n), fmt.Sprintf("toolu_%d", n), "ls"
	if push {
		command = "git push origin HEAD:dev"
	}
	return []events.Proposal{
		said(byLead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"}),
		said(byWorker, events.TurnStarted, turn, map[string]any{"agent": "engineer", "hash": "h"}),
		said(byEngineer, events.ActionRequested, action, map[string]any{"tool": "Bash", "command": command}),
		said(byRules, events.ActionApproved, action, map[string]any{"reason": "otherwise"}),
		said(byWorker, events.ActionResult, action, nil),
		said(byWorker, events.TurnEnded, turn, map[string]any{"agent": "engineer", "report": "Done.", "tokens": 10}),
	}
}

func TestOnlyPushesSinceTheLastReopenCount(t *testing.T) {
	byLeadDone := said(byLead, events.TaskDone, "", map[string]any{"summary": "Done."})
	byYouDone := said(byYou, events.TaskDone, "", map[string]any{"summary": "Moved.", "moved": true})
	for name, c := range map[string]struct {
		pushAgain bool
		done      events.Proposal
		want      bool
	}{
		"no new push, done by its coordinator": {false, byLeadDone, false},
		"no new push, moved to Done by you":    {false, byYouDone, false},
		"pushed again":                         {true, byLeadDone, true},
	} {
		steps := []events.Proposal{said(byYou, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead"})}
		steps = append(steps, engineerTurn(1, true)...)
		steps = append(steps, byLeadDone, said(byYou, events.TaskReopened, "", map[string]any{"text": "Again."}))
		steps = append(steps, engineerTurn(2, c.pushAgain)...)
		task := FromEvents(logOf(append(steps, c.done)...))
		if !task.IsDone {
			t.Fatalf("%s: the task isn't done", name)
		}
		if got := SummaryOf(task, nil).Pushed; got != c.want {
			t.Errorf("%s: pushed %v, want %v", name, got, c.want)
		}
	}
}
