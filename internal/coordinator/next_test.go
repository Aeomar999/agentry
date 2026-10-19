package coordinator

import (
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

var (
	you         = events.Actor{Kind: events.Human, ID: "you"}
	coordinator = events.Actor{Kind: events.Agent, ID: "tech-lead"}
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
	// Clipped: histories that share a start don't write over each other.
	return append(slices.Clip(h), events.Event{Proposal: events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Correlation: correlation, Data: data}})
}

func (h history) turn(id, agent, report string) history {
	return h.add(worker, events.TurnStarted, id, map[string]any{"agent": agent, "hash": "h"}).
		add(worker, events.TurnEnded, id, map[string]any{"agent": agent, "report": report})
}

func (h history) task() *tasks.Task { return tasks.FromEvents(h) }

func started() history {
	return history{}.
		add(you, events.TaskCreated, "", map[string]any{"title": "Remove the TODOs", "repo": "/r", "coordinator": "tech-lead"}).
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
		{"a new task needs a worktree", history{}.add(you, events.TaskCreated, "", map[string]any{"title": "t", "repo": "/r", "coordinator": "tech-lead"}).task(),
			PrepareWorktree, ""},
		{"then goes to the coordinator", started().task(), StartTurn, "tech-lead"},
		{"wait while an agent works", started().add(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "tech-lead", "hash": "h"}).task(),
			Wait, ""},
		{"apply the coordinator's decision", started().turn("turn-1", "tech-lead", assignEngineer).task(), ApplyDecision, ""},
		{"a reply with no decision is an issue", started().turn("turn-1", "tech-lead", "I think the engineer.").task(), RaiseIssue, "no usable decision"},
		{"so is no reply at all", started().turn("turn-1", "tech-lead", "").task(), RaiseIssue, "no usable decision"},
		{"assigning to nobody is an issue", started().turn("turn-1", "tech-lead", `{"action":"assign","agent":"designer","instructions":"x"}`).task(),
			RaiseIssue, "no agent by that name"},
		{"the assigned agent goes next", started().turn("turn-1", "tech-lead", assignEngineer).
			add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove them."}).task(),
			StartTurn, "Remove them."},
		{"its report goes back to the coordinator", started().turn("turn-1", "tech-lead", assignEngineer).
			add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove them."}).
			turn("turn-2", "engineer", "Removed 3.").task(),
			StartTurn, "Removed 3."},
		{"so does an issue", started().add(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "tech-lead", "hash": "h"}).
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

// An epic waits while its stories are going, and, once they are all done,
// after it has asked you to close it: you may leave it open.
func TestWhenAnEpicMayWait(t *testing.T) {
	const wait = `{"action": "wait"}`
	epic := history{}.
		add(you, events.TaskCreated, "", map[string]any{"title": "Ship it", "repo": "/r", "coordinator": "tech-lead", "kind": "epic"}).
		add(worker, events.WorktreeReady, "", map[string]any{"path": "/w", "branch": "agentry/task-1", "port": 7901})
	split := epic.turn("turn-1", "tech-lead", `{"action": "split"}`).
		add(coordinator, events.TaskSplit, "", map[string]any{"stories": []string{"task-2"}, "titles": "The store"})
	allDone := split.add(watchdog, events.StoryDone, "", map[string]any{"story": "task-2", "summary": "In.", "coordinator": "tech-lead"})
	asked := allDone.turn("turn-2", "tech-lead", `{"action": "done", "summary": "Shipped."}`).
		add(coordinator, events.TaskNeedsYou, "", map[string]any{"question": "Close it?", "options": []string{CloseEpic}, "closing": "Shipped."})
	leftOpen := asked.add(you, events.TaskAnswered, "", map[string]any{"answer": "Leave it open"})
	cases := []struct {
		name string
		task *tasks.Task
		want MoveKind
		says string
	}{
		{"not a story", started().turn("turn-1", "tech-lead", wait).task(), RaiseIssue, "only an epic with stories can wait"},
		{"not an epic with no stories", epic.turn("turn-1", "tech-lead", wait).task(), RaiseIssue, "only an epic with stories can wait"},
		{"an epic with stories going", split.turn("turn-2", "tech-lead", wait).task(), ApplyDecision, ""},
		{"not before asking you to close it", allDone.turn("turn-2", "tech-lead", wait).task(), RaiseIssue, "finish it with a summary"},
		{"an epic you left open", leftOpen.turn("turn-3", "tech-lead", wait).task(), ApplyDecision, ""},
		{"not once a new story is going: it is asked again when that is done", leftOpen.turn("turn-3", "tech-lead", wait).
			add(coordinator, events.EpicWaits, "", nil).
			add(you, events.StoryAdded, "", map[string]any{"story": "task-3", "title": "The command"}).
			add(watchdog, events.StoryDone, "", map[string]any{"story": "task-3", "summary": "In.", "coordinator": "tech-lead"}).
			turn("turn-4", "tech-lead", wait).task(), RaiseIssue, "finish it with a summary"},
	}
	for _, c := range cases {
		move := Next(c.task, defaultRoster())
		if move.Kind != c.want || !strings.Contains(move.Reason, c.says) {
			t.Errorf("%s: got %s (%q), want %s with %q", c.name, move.Kind, move.Reason, c.want, c.says)
		}
	}

	// Waiting with every story done, it is open and quiet, and still closes.
	idle := leftOpen.turn("turn-3", "tech-lead", wait).add(coordinator, events.EpicWaits, "", nil).task()
	if move := Next(idle, defaultRoster()); move.Kind != Wait || idle.Status() != tasks.Working || idle.Closing != "Shipped." {
		t.Errorf("left open: move %s, status %s, closing %q", move.Kind, idle.Status(), idle.Closing)
	}
	if last := idle.Trail[len(idle.Trail)-1]; !strings.Contains(last.Text, "stays open until you close it or add a story") {
		t.Errorf("the trail should say it stays open: %+v", last)
	}

	// Taking out its last story leaves nothing done to close with.
	empty := idle
	if empty = leftOpen.turn("turn-3", "tech-lead", wait).add(coordinator, events.EpicWaits, "", nil).
		add(you, events.StoryRemoved, "", map[string]any{"story": "task-2"}).task(); empty.Closing != "" || len(empty.Stories) != 0 {
		t.Errorf("no stories left: closing %q, stories %v", empty.Closing, empty.Stories)
	}
}

func TestTheSafetyStopsSendTheTaskToYou(t *testing.T) {
	h := started()
	for i := 0; i < DefaultMaxTurns/2; i++ {
		h = h.turn(turnID(len(h)), "tech-lead", `{"action":"assign","agent":"engineer","instructions":"again"}`).
			add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "again"}).
			turn(turnID(len(h)+1), "engineer", "still broken")
	}
	if move := Next(h.task(), defaultRoster()); move.Kind != AskYou || move.Limit != LimitTurns || !strings.Contains(move.Reason, "24 of its 24 turns") {
		t.Errorf("after %d turns: %+v", DefaultMaxTurns, move)
	}
	h = h.add(watchdog, events.TaskNeedsYou, "", map[string]any{"question": "?"}).add(you, events.TaskAnswered, "", map[string]any{"answer": "try X"})
	if move := Next(h.task(), defaultRoster()); move.Kind != StartTurn {
		t.Errorf("your answer resets the count: %+v", move)
	}

	issues := started()
	for range MaxIssuesRun {
		issues = issues.add(watchdog, events.IssueRaised, "", map[string]any{"reason": "the engine is missing"})
	}
	if move := Next(issues.task(), defaultRoster()); move.Kind != AskYou || move.Limit != "" || !strings.Contains(move.Reason, "engine is missing") {
		t.Errorf("after %d issues in a row: %+v", MaxIssuesRun, move)
	}
}

// A task made with its own limit of turns stops at it; made unlimited, or
// limited to 0 later, it doesn't stop. One made before limits were each
// task's own keeps the old one.
func TestEachTaskHasItsOwnLimitOfTurns(t *testing.T) {
	if old := started().task(); old.MaxTurns != DefaultMaxTurns {
		t.Errorf("a task created without maxTurns: %d", old.MaxTurns)
	}
	withLimit := func(maxTurns int) history {
		return history{}.
			add(you, events.TaskCreated, "", map[string]any{"title": "x", "repo": "/r", "coordinator": "tech-lead", "maxTurns": maxTurns}).
			add(worker, events.WorktreeReady, "", map[string]any{"path": "/w", "branch": "b", "port": 7901})
	}
	busy := func(h history, turns int) history {
		for range turns / 2 {
			h = h.turn(turnID(len(h)), "tech-lead", `{"action":"assign","agent":"engineer","instructions":"again"}`).
				add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "again"}).
				turn(turnID(len(h)+1), "engineer", "still broken")
		}
		return h
	}

	h := busy(withLimit(4), 4)
	if move := Next(h.task(), defaultRoster()); move.Kind != AskYou || !strings.Contains(move.Reason, "4 of its 4 turns") {
		t.Errorf("at its own limit: %+v", move)
	}
	h = h.add(you, events.TaskTurnsLimited, "", map[string]any{"maxTurns": 0})
	if move := Next(h.task(), defaultRoster()); move.Kind != StartTurn {
		t.Errorf("made unlimited: %+v", move)
	}
	if move := Next(busy(withLimit(0), 40).task(), defaultRoster()); move.Kind != StartTurn {
		t.Errorf("created unlimited, after 40 turns: %+v", move)
	}
}

func turnID(n int) string { return "turn-" + strings.Repeat("x", n) } // unique; Next doesn't read turn numbers

// At its budget a task comes to you before another turn starts; after your
// answer one more turn runs, so the coordinator can wrap up.
func TestTheBudgetSendsTheTaskToYou(t *testing.T) {
	h := history{}.
		add(you, events.TaskCreated, "", map[string]any{"title": "x", "repo": "/r", "coordinator": "tech-lead", "budget": 100_000}).
		add(worker, events.WorktreeReady, "", map[string]any{"path": "/w", "branch": "b", "port": 7901}).
		add(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "tech-lead", "hash": "h"}).
		add(worker, events.TurnEnded, "turn-1", map[string]any{"agent": "tech-lead", "report": "r", "tokens": 60_000}).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"})
	if move := Next(h.task(), defaultRoster()); move.Kind != StartTurn {
		t.Errorf("under budget: %+v", move)
	}
	h = h.add(worker, events.TurnStarted, "turn-2", map[string]any{"agent": "engineer", "hash": "h"}).
		add(worker, events.TurnEnded, "turn-2", map[string]any{"agent": "engineer", "report": "half done", "tokens": 50_000})
	move := Next(h.task(), defaultRoster())
	if move.Kind != AskYou || move.Limit != LimitBudget || !strings.Contains(move.Reason, "110k tokens of its 100k budget") {
		t.Errorf("over budget: %+v", move)
	}
	h = h.add(agentry, events.TaskNeedsYou, "", map[string]any{"question": move.Reason, "limit": move.Limit})
	if task := h.task(); StoppedAt(task) != LimitBudget {
		t.Errorf("the stop's limit: %q", StoppedAt(task))
	}
	h = h.add(you, events.TaskAnswered, "", map[string]any{"answer": "Wrap up."})
	if task := h.task(); task.Asked.Limit != "" || StoppedAt(task) != "" {
		t.Errorf("after your answer the limit is cleared: %q", task.Asked.Limit)
	}
	if move := Next(h.task(), defaultRoster()); move.Kind != StartTurn || move.Agent.Name != "tech-lead" {
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
