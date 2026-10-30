package tasks

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// Task-8's log, written before task.held stopped the turn in progress: the
// engineer's actions carried on after you parked the task. Replayed now,
// they find no open turn, and change nothing.
func TestAnOldLogWithActionsAfterAHoldReplays(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	engineer := events.Actor{Kind: events.Agent, ID: "engineer"}
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	rules := events.Actor{Kind: events.Policy, ID: "rules"}
	at := time.Now()
	event := func(actor events.Actor, kind events.Type, correlation string, data map[string]any) events.Event {
		return events.Event{At: at, Proposal: events.Proposal{Stream: "task-8", Actor: actor, Type: kind, Correlation: correlation, Data: data}}
	}
	log := []events.Event{
		event(you, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead"}),
		event(lead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"}),
		event(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "engineer", "hash": "h"}),
		event(worker, events.PromptAcknowledged, "turn-1", map[string]any{"hash": "h"}),
		event(engineer, events.ActionRequested, "toolu_1", map[string]any{"tool": "Bash", "command": "ls"}),
		event(rules, events.ActionApproved, "toolu_1", map[string]any{"reason": "otherwise"}),
		event(you, events.TaskHeld, "", map[string]any{"place": "todo"}),
		event(worker, events.ActionResult, "toolu_1", nil),
		event(engineer, events.ActionRequested, "toolu_2", map[string]any{"tool": "Bash", "command": "rm x"}),
		event(rules, events.ActionAsked, "toolu_2", map[string]any{"reason": "ask"}),
		event(you, events.ActionDenied, "toolu_2", map[string]any{"reason": "no"}),
		event(worker, events.ActionResult, "toolu_2", nil),
		event(engineer, events.ActionRequested, "toolu_3", map[string]any{"tool": "Bash", "command": "pwd"}),
		event(rules, events.ActionApproved, "toolu_3", map[string]any{"reason": "otherwise"}),
		event(worker, events.ActionResult, "toolu_3", nil),
		event(worker, events.PlanUpdated, "turn-1", map[string]any{"plan": "- [x] ls"}),
		event(worker, events.IssueRaised, "", map[string]any{"reason": "engineer's process exited in turn-1"}),
	}

	task := FromEvents(log)
	if len(task.Actions) != 1 || task.Actions[0].ID != "toolu_1" || !task.Actions[0].HasResult {
		t.Errorf("only the action asked before the hold is there, with its result: %+v", task.Actions)
	}
	if len(task.WaitingForYou()) != 0 {
		t.Errorf("an action asked after the hold doesn't wait for you: %+v", task.WaitingForYou())
	}
	if turn := task.Turn("turn-1"); !turn.Ended || !turn.Stopped || turn.Issue != "" || turn.Plan != "- [x] ls" {
		t.Errorf("the hold stopped the turn, which keeps its plan: %+v", turn)
	}
	if task.Parked != "todo" {
		t.Errorf("parked in %q, want todo", task.Parked)
	}
}

// An old log names the idea a task was made from only as "idea"; a new one
// says "from" as well, for older binaries. Both read as the same MadeFrom.
func TestATaskIsMadeFromItsIdeaInOldAndNewLogs(t *testing.T) {
	agentry := events.Actor{Kind: events.System, ID: "agentry"}
	for _, c := range []struct {
		name string
		data map[string]any
		want string
	}{
		{"old: only idea", map[string]any{"idea": "idea-3"}, "idea-3"},
		{"new: from and idea", map[string]any{"from": "idea-3", "idea": "idea-3"}, "idea-3"},
		{"only from", map[string]any{"from": "idea-3"}, "idea-3"},
		{"neither", map[string]any{}, ""},
	} {
		c.data["title"], c.data["coordinator"] = "t", "tech-lead"
		task := FromEvents([]events.Event{{At: time.Now(), Proposal: events.Proposal{Stream: "task-1", Actor: agentry, Type: events.TaskCreated, Data: c.data}}})
		if task.MadeFrom != c.want {
			t.Errorf("%s: made from %q, want %q", c.name, task.MadeFrom, c.want)
		}
	}
}
