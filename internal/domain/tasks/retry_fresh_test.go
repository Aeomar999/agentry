package tasks

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// resumeThatFails is a log whose engineer's turn-2 resumed s-1 on the
// machine its coordinator chose, and an issue.raised with data after it.
func resumeThatFails(issue map[string]any) *Task {
	you := events.Actor{Kind: events.Human, ID: "you"}
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	worker := events.Actor{Kind: events.Worker, ID: "tower"}
	dog := events.Actor{Kind: events.System, ID: "watchdog"}
	event := func(actor events.Actor, kind events.Type, correlation string, data map[string]any) events.Event {
		return events.Event{At: time.Now(), Proposal: events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Correlation: correlation, Data: data}}
	}
	return FromEvents([]events.Event{
		event(you, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead"}),
		event(lead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove every TODO."}),
		event(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "engineer", "hash": "h1", "worker": "tower", "engine": "claude"}),
		event(worker, events.SessionStarted, "turn-1", map[string]any{"engineSession": "s-1"}),
		event(worker, events.TurnEnded, "turn-1", map[string]any{"agent": "engineer", "report": "Removed 3."}),
		event(lead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove the one in src/b.go.", "machine": "tower"}),
		event(worker, events.TurnStarted, "turn-2", map[string]any{"agent": "engineer", "hash": "h2", "worker": "tower", "engine": "claude", "fromSession": "s-1"}),
		event(dog, events.IssueRaised, "", issue),
	})
}

// A resumed session that didn't start ends its turn as an issue, but the
// same agent gets the same step again, machine and all, and the
// coordinator is only told why.
func TestARetryFreshIssueHandsTheSameStepBack(t *testing.T) {
	const why = "engineer's resumed session didn't start (its process exited before taking the prompt); starting it fresh"
	task := resumeThatFails(map[string]any{"reason": why, "retryFresh": true})

	if turn := task.Turn("turn-2"); !turn.Ended || turn.Issue != why {
		t.Errorf("the failed turn ends with its issue: %+v", turn)
	}
	want := Step{From: "tech-lead", To: "engineer", Text: "Remove the one in src/b.go.", Machine: "tower"}
	if got := task.Handoff; got == nil || got.From != want.From || got.To != want.To || got.Text != want.Text || got.Machine != want.Machine {
		t.Errorf("handoff %+v, want the failed turn's own %+v", got, want)
	}
	if note := task.Trail[len(task.Trail)-1]; note.From != Agentry || note.To != "tech-lead" || note.Text != why {
		t.Errorf("the coordinator is told why in the trail: %+v", note)
	}
	if task.IssuesInARow != 1 {
		t.Errorf("issues in a row %d, want 1: a retry still counts, so retries are bounded", task.IssuesInARow)
	}
	if session, fresh := task.SessionToResume("engineer", "claude", "tower"); session != "" || fresh != "its resume failed" {
		t.Errorf("the retry resumes %q, fresh because %q; want fresh because its resume failed", session, fresh)
	}
}

// Any other issue, such as an old log's, goes to the coordinator as it
// always did, even after a turn that resumed.
func TestAPlainIssueStillGoesToTheCoordinator(t *testing.T) {
	const why = "engineer's process exited in turn-2 (crashed or was killed)"
	task := resumeThatFails(map[string]any{"reason": why})
	if got := task.Handoff; got == nil || got.From != Agentry || got.To != "tech-lead" || got.Text != why || got.Machine != "" {
		t.Errorf("handoff %+v, want the issue to the coordinator", got)
	}
	if task.Turn("turn-2").Issue != why || task.IssuesInARow != 1 {
		t.Errorf("turn %+v, issues in a row %d", task.Turn("turn-2"), task.IssuesInARow)
	}
}
