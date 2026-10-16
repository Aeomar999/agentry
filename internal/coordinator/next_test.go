package coordinator

import (
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

var (
	you         = events.Actor{Kind: events.Human, ID: "you"}
	coordinator = events.Actor{Kind: events.Agent, ID: "coordinator"}
	watchdog    = events.Actor{Kind: events.System, ID: "watchdog"}
)

func defaultRoster() agents.Roster {
	var stream []events.Event
	for _, p := range agents.SeedProposals(nil) {
		stream = append(stream, events.Event{Proposal: p})
	}
	return agents.FromEvents(stream)
}

// history builds a task from events, without the store: Next only reads state.
type history []events.Event

func (h history) add(actor events.Actor, kind events.Type, correlation string, data map[string]any) history {
	return append(h, events.Event{Proposal: events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Correlation: correlation, Data: data}})
}

func (h history) turn(id, agent, report string) history {
	return h.add(worker, events.TurnStarted, id, map[string]any{"agent": agent, "hash": "h"}).
		add(worker, events.TurnEnded, id, map[string]any{"agent": agent, "report": report})
}

func (h history) task() *tasks.Task { return tasks.FromEvents(h) }

func started() history {
	return history{}.
		add(you, events.TaskCreated, "", map[string]any{"title": "Remove the TODOs", "repo": "/r", "coordinator": "coordinator"}).
		add(worker, events.WorktreeReady, "", map[string]any{"path": "/w", "branch": "agentry/task-1", "port": 7901})
}

func TestNext(t *testing.T) {
	assignEngineer := `Start with the engineer. {"action": "assign", "agent": "engineer", "instructions": "Remove them."}`
	cases := []struct {
		name string
		task *tasks.Task
		want MoveKind
		says string // in the move's agent, handoff or reason
	}{
		{"a new task needs a worktree", history{}.add(you, events.TaskCreated, "", map[string]any{"title": "t", "repo": "/r", "coordinator": "coordinator"}).task(),
			PrepareWorktree, ""},
		{"then goes to the coordinator", started().task(), StartTurn, "coordinator"},
		{"wait while an agent works", started().add(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "coordinator", "hash": "h"}).task(),
			Wait, ""},
		{"apply the coordinator's decision", started().turn("turn-1", "coordinator", assignEngineer).task(), ApplyDecision, ""},
		{"a reply with no decision is an issue", started().turn("turn-1", "coordinator", "I think the engineer.").task(), RaiseIssue, "no usable decision"},
		{"assigning to nobody is an issue", started().turn("turn-1", "coordinator", `{"action":"assign","agent":"designer","instructions":"x"}`).task(),
			RaiseIssue, "no agent by that name"},
		{"the assigned agent goes next", started().turn("turn-1", "coordinator", assignEngineer).
			add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove them."}).task(),
			StartTurn, "Remove them."},
		{"its report goes back to the coordinator", started().turn("turn-1", "coordinator", assignEngineer).
			add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove them."}).
			turn("turn-2", "engineer", "Removed 3.").task(),
			StartTurn, "Removed 3."},
		{"so does an issue", started().add(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "coordinator", "hash": "h"}).
			add(watchdog, events.IssueRaised, "", map[string]any{"reason": "the agent's process exited"}).task(),
			StartTurn, "process exited"},
		{"wait for your answer", started().add(coordinator, events.TaskNeedsYou, "", map[string]any{"question": "Deploy?"}).task(), Wait, ""},
		{"your answer goes to the coordinator", started().add(coordinator, events.TaskNeedsYou, "", map[string]any{"question": "Deploy?"}).
			add(you, events.TaskAnswered, "", map[string]any{"answer": "Yes"}).task(),
			StartTurn, "Yes"},
	}
	for _, c := range cases {
		move := Next(c.task, defaultRoster())
		said := move.Agent.Name + " " + move.Handoff + " " + move.Reason
		if move.Kind != c.want || !strings.Contains(said, c.says) {
			t.Errorf("%s: got %s (%q), want %s with %q", c.name, move.Kind, said, c.want, c.says)
		}
	}
}

func TestTheSafetyStopsSendTheTaskToYou(t *testing.T) {
	h := started()
	for i := 0; i < MaxTurns/2; i++ {
		h = h.turn(turnID(len(h)), "coordinator", `{"action":"assign","agent":"engineer","instructions":"again"}`).
			add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "again"}).
			turn(turnID(len(h)+1), "engineer", "still broken")
	}
	if move := Next(h.task(), defaultRoster()); move.Kind != AskYou || !strings.Contains(move.Reason, "24 turns") {
		t.Errorf("after %d turns: %+v", MaxTurns, move)
	}
	h = h.add(watchdog, events.TaskNeedsYou, "", map[string]any{"question": "?"}).add(you, events.TaskAnswered, "", map[string]any{"answer": "try X"})
	if move := Next(h.task(), defaultRoster()); move.Kind != StartTurn {
		t.Errorf("your answer resets the count: %+v", move)
	}

	issues := started()
	for range MaxIssuesRun {
		issues = issues.add(watchdog, events.IssueRaised, "", map[string]any{"reason": "the engine is missing"})
	}
	if move := Next(issues.task(), defaultRoster()); move.Kind != AskYou || !strings.Contains(move.Reason, "engine is missing") {
		t.Errorf("after %d issues in a row: %+v", MaxIssuesRun, move)
	}
}

func turnID(n int) string { return "turn-" + strings.Repeat("x", n) } // unique; Next doesn't read turn numbers

// At its budget a task comes to you before another turn starts; after your
// answer one more turn runs, so the coordinator can wrap up.
func TestTheBudgetSendsTheTaskToYou(t *testing.T) {
	h := history{}.
		add(you, events.TaskCreated, "", map[string]any{"title": "x", "repo": "/r", "coordinator": "coordinator", "budget": 100_000}).
		add(worker, events.WorktreeReady, "", map[string]any{"path": "/w", "branch": "b", "port": 7901}).
		add(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "coordinator", "hash": "h"}).
		add(worker, events.TurnEnded, "turn-1", map[string]any{"agent": "coordinator", "report": "r", "tokens": 60_000}).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"})
	if move := Next(h.task(), defaultRoster()); move.Kind != StartTurn {
		t.Errorf("under budget: %+v", move)
	}
	h = h.add(worker, events.TurnStarted, "turn-2", map[string]any{"agent": "engineer", "hash": "h"}).
		add(worker, events.TurnEnded, "turn-2", map[string]any{"agent": "engineer", "report": "half done", "tokens": 50_000})
	move := Next(h.task(), defaultRoster())
	if move.Kind != AskYou || !strings.Contains(move.Reason, "110k tokens of its 100k budget") {
		t.Errorf("over budget: %+v", move)
	}
	h = h.add(watchdog, events.TaskNeedsYou, "", map[string]any{"question": move.Reason}).
		add(you, events.TaskAnswered, "", map[string]any{"answer": "Wrap up."})
	if move := Next(h.task(), defaultRoster()); move.Kind != StartTurn || move.Agent.Name != "coordinator" {
		t.Errorf("after your answer the coordinator gets one turn: %+v", move)
	}
}

func TestTokens(t *testing.T) {
	for n, want := range map[int]string{950: "950", 12_400: "12.4k", 5_000_000: "5M", 3_140_000: "3.1M"} {
		if got := Tokens(n); got != want {
			t.Errorf("Tokens(%d) = %q, want %q", n, got, want)
		}
	}
}
