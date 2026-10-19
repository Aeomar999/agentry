package tasks

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Logs written before your board moves stopped the turn in progress hold
// that turn's events after the move: the agent went on working. Each must
// replay under today's apply, with the turn stopped and your move kept.

var (
	byYou      = events.Actor{Kind: events.Human, ID: "you"}
	byLead     = events.Actor{Kind: events.Agent, ID: "tech-lead"}
	byEngineer = events.Actor{Kind: events.Agent, ID: "engineer"}
	byWorker   = events.Actor{Kind: events.Worker, ID: "local"}
	byRules    = events.Actor{Kind: events.Policy, ID: "rules"}
)

func logOf(steps ...events.Proposal) []events.Event {
	log := make([]events.Event, len(steps))
	for i, p := range steps {
		p.Stream = "task-1"
		log[i] = events.Event{Position: int64(i + 1), At: time.Now(), Proposal: p}
	}
	return log
}

func said(actor events.Actor, kind events.Type, correlation string, data map[string]any) events.Proposal {
	return events.Proposal{Actor: actor, Type: kind, Correlation: correlation, Data: data}
}

// The engineer's turn-1 runs and asks for toolu_1; then move happens, and
// the engineer asks for toolu_2, which gets its result, and ends the turn.
func engineerWorksOnAfter(move events.Proposal) []events.Event {
	return logOf(
		said(byYou, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead"}),
		said(byLead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"}),
		said(byWorker, events.TurnStarted, "turn-1", map[string]any{"agent": "engineer", "hash": "h"}),
		said(byWorker, events.PromptAcknowledged, "turn-1", map[string]any{"hash": "h"}),
		said(byEngineer, events.ActionRequested, "toolu_1", map[string]any{"tool": "Bash", "command": "ls"}),
		said(byRules, events.ActionApproved, "toolu_1", map[string]any{"reason": "otherwise"}),
		move,
		said(byEngineer, events.ActionRequested, "toolu_2", map[string]any{"tool": "Bash", "command": "pwd"}),
		said(byRules, events.ActionApproved, "toolu_2", map[string]any{"reason": "otherwise"}),
		said(byWorker, events.ActionResult, "toolu_2", nil),
		said(byWorker, events.ActionResult, "toolu_1", nil),
		said(byWorker, events.PlanUpdated, "turn-1", map[string]any{"plan": "- [x] ls"}),
		said(byWorker, events.AgentWaiting, "turn-1", map[string]any{"question": "more?"}),
		said(byWorker, events.TurnEnded, "turn-1", map[string]any{"agent": "engineer", "report": "Listed.", "tokens": 10}),
	)
}

// replayed applies the log and fails the test if any part of the replay
// leaves a turn running or keeps an action asked after the move.
func replayed(t *testing.T, log []events.Event) *Task {
	t.Helper()
	task := FromEvents(log)
	if task == nil {
		t.Fatal("no task")
	}
	if turn := task.OpenTurn(); turn != nil {
		t.Errorf("%s is still running", turn.ID)
	}
	if len(task.Actions) != 1 || task.Actions[0].ID != "toolu_1" || !task.Actions[0].HasResult {
		t.Errorf("only the action asked before the move is there, with its result: %+v", task.Actions)
	}
	if turn := task.Turn("turn-1"); !turn.Stopped || turn.Plan != "- [x] ls" {
		t.Errorf("the move stopped turn-1, which keeps its plan: %+v", turn)
	}
	return task
}

func TestAHoldInToDoStandsOverTheTurnsLaterEvents(t *testing.T) {
	task := replayed(t, engineerWorksOnAfter(said(byYou, events.TaskHeld, "", map[string]any{"place": "todo"})))
	if task.Parked != "todo" || task.Status() != Held || task.WrapUp == nil || task.WrapUp.To != "todo" {
		t.Errorf("parked %q, status %s, wrap-up %+v: want To do, waiting for the coordinator's wrap-up", task.Parked, task.Status(), task.WrapUp)
	}
}

func TestAHoldInTheBacklogStandsOverTheTurnsLaterEvents(t *testing.T) {
	task := replayed(t, engineerWorksOnAfter(said(byYou, events.TaskHeld, "", map[string]any{"place": "backlog"})))
	if task.Parked != "backlog" || task.Status() != Held || task.WrapUp == nil || task.WrapUp.To != "backlog" {
		t.Errorf("parked %q, status %s, wrap-up %+v: want the Backlog, waiting for the coordinator's wrap-up", task.Parked, task.Status(), task.WrapUp)
	}
}

func TestAMoveToDoneStandsOverTheTurnsLaterEvents(t *testing.T) {
	task := replayed(t, engineerWorksOnAfter(said(byYou, events.TaskDone, "", map[string]any{"summary": "Moved.", "moved": true})))
	if !task.IsDone || !task.DoneByYou || task.WrapUp == nil || task.WrapUp.To != "done" {
		t.Errorf("done %v, by you %v, wrap-up %+v: want done by you, waiting for the coordinator's wrap-up", task.IsDone, task.DoneByYou, task.WrapUp)
	}
}

// The coordinator's wrap-up turn after a move to Done, with the stopped
// engineer's events still coming in around it.
func TestADoneWrapUpReplaysWithTheStoppedTurnsLaterEvents(t *testing.T) {
	log := engineerWorksOnAfter(said(byYou, events.TaskDone, "", map[string]any{"summary": "Moved.", "moved": true}))
	log = append(log, logOf(
		said(byWorker, events.TurnStarted, "turn-2", map[string]any{"agent": "tech-lead", "hash": "h2"}),
		said(byLead, events.ActionRequested, "toolu_3", map[string]any{"tool": "Bash", "command": "git log"}),
		said(byRules, events.ActionApproved, "toolu_3", map[string]any{"reason": "otherwise"}),
		said(byWorker, events.ActionResult, "toolu_3", nil),
		said(byWorker, events.TurnEnded, "turn-2", map[string]any{"agent": "tech-lead", "report": `{"action":"done"}`}),
		said(byLead, events.TaskWrappedUp, "", map[string]any{"summary": "Half done, committed."}),
		said(byWorker, events.TurnStopped, "turn-1", nil),
		said(byEngineer, events.ActionRequested, "toolu_4", map[string]any{"tool": "Bash", "command": "ls"}),
		said(byWorker, events.ActionResult, "toolu_4", nil),
	)...)

	task := FromEvents(log)
	if turn := task.OpenTurn(); turn != nil {
		t.Errorf("%s is still running", turn.ID)
	}
	if !task.IsDone || !task.DoneByYou || task.WrapUp != nil || task.Summary != "Half done, committed." {
		t.Errorf("done %v, by you %v, wrap-up %+v, summary %q: want done by you and wrapped up", task.IsDone, task.DoneByYou, task.WrapUp, task.Summary)
	}
	if ids := actionIDs(task); len(ids) != 2 || ids[0] != "toolu_1" || ids[1] != "toolu_3" {
		t.Errorf("actions %v, want toolu_1 before the move and the wrap-up's toolu_3", ids)
	}
	if turn := task.Turn("turn-2"); turn == nil || turn.Agent != "tech-lead" || !turn.Ended {
		t.Errorf("the wrap-up turn ended: %+v", turn)
	}
}

func actionIDs(task *Task) []string {
	var ids []string
	for _, a := range task.Actions {
		ids = append(ids, a.ID)
	}
	return ids
}
