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

// shipTurn is a turn of the deployer's, making the tool calls steps add.
func (h history) shipTurn(id, report string, steps ...func(h history, turn string) history) history {
	h = h.add(worker, events.TurnStarted, id, map[string]any{"agent": "deployer", "hash": "h"})
	for _, step := range steps {
		h = step(h, id)
	}
	return h.add(worker, events.TurnEnded, id, map[string]any{"agent": "deployer", "report": report})
}

// push asks to push the work to dev, decided as decision, and finished or not.
func push(decision events.Type, finished bool) func(history, string) history {
	return func(h history, turn string) history {
		id := turn + "-push"
		h = h.add(worker, events.ActionRequested, id, map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"}).
			add(you, decision, id, nil)
		if finished {
			h = h.add(worker, events.ActionResult, id, nil)
		}
		return h
	}
}

var pushed = push(events.ActionApproved, true)

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

// A handoff that undoes your move of the card is an issue before it is
// applied: the coordinator hears why and decides again. Sent again and
// again, it comes to you at the turn limit.
func TestAHandoffFitsTheColumnYouMovedItTo(t *testing.T) {
	assign := func(agent string) string {
		return `{"action": "assign", "agent": "` + agent + `", "instructions": "go"}`
	}
	review := started().add(you, events.TaskDirected, "", map[string]any{"text": "Have this task reviewed next.", "column": "review"})
	sentBack := started().add(you, events.TaskDirected, "", map[string]any{"text": "This needs more work.", "column": "in-progress", "sentBack": true})
	shipping := started().add(you, events.TaskDirected, "", map[string]any{"text": "Ship this task now.", "column": "shipping"})
	const toShip = "the user moved this task to Shipping; hand it to an agent that ships work"
	cases := []struct {
		name string
		task *tasks.Task
		want MoveKind
		says string
	}{
		{"moved to Review, not to a builder", review.turn("turn-1", "tech-lead", assign("engineer")).task(),
			RaiseIssue, "the user moved this task to Review; hand it to an agent that checks work"},
		{"but to a checker", review.turn("turn-1", "tech-lead", assign("reviewer")).task(), ApplyDecision, ""},
		{"sent back, not to a checker", sentBack.turn("turn-1", "tech-lead", assign("reviewer")).task(),
			RaiseIssue, "the user moved this task back to In progress; hand it to an agent that builds it"},
		{"but to a builder", sentBack.turn("turn-1", "tech-lead", assign("engineer")).task(), ApplyDecision, ""},
		{"sent back, not to a shipper", sentBack.turn("turn-1", "tech-lead", assign("deployer")).task(),
			RaiseIssue, "the user moved this task back to In progress; hand it to an agent that builds it"},
		{"moved to Shipping, not to a builder", shipping.turn("turn-1", "tech-lead", assign("engineer")).task(), RaiseIssue, toShip},
		{"nor to a checker", shipping.turn("turn-1", "tech-lead", assign("reviewer")).task(), RaiseIssue, toShip},
		{"nor finished first", shipping.turn("turn-1", "tech-lead", `{"action": "done", "summary": "x"}`).task(),
			RaiseIssue, toShip + " before you finish"},
		{"but to a shipper", shipping.turn("turn-1", "tech-lead", assign("deployer")).task(), ApplyDecision, ""},
	}
	for _, c := range cases {
		if move := Next(c.task, defaultRoster()); move.Kind != c.want || move.Reason != c.says {
			t.Errorf("%s: got %s (%q), want %s with %q", c.name, move.Kind, move.Reason, c.want, c.says)
		}
	}

	h := review
	for i := range DefaultMaxTurns {
		h = h.turn(turnID(len(h)), "tech-lead", assign("engineer"))
		move := Next(h.task(), defaultRoster())
		if move.Kind != RaiseIssue {
			t.Fatalf("refused handoff %d: %+v", i+1, move)
		}
		h = h.add(watchdog, events.IssueRaised, "", map[string]any{"reason": move.Reason})
	}
	if move := Next(h.task(), defaultRoster()); move.Kind != AskYou || !strings.Contains(move.Reason, "24 turns") {
		t.Errorf("after %d refused handoffs: %+v", DefaultMaxTurns, move)
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

// After you move a task to Done, To do or the Backlog, its coordinator has
// one turn to wrap up, whose only outcome is its summary; then nothing runs.
func TestAWrapUpIsOneTurn(t *testing.T) {
	working := started().turn("turn-1", "tech-lead", `{"action":"assign","agent":"engineer","instructions":"go"}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"}).
		add(worker, events.TurnStarted, "turn-2", map[string]any{"agent": "engineer", "hash": "h"})
	done := working.add(you, events.TaskDone, "", map[string]any{"summary": "Moved.", "moved": true})
	parked := working.add(you, events.TaskHeld, "", map[string]any{"place": "backlog"})
	wrapUp := func(h history, report string) *tasks.Task { return h.turn("turn-3", "tech-lead", report).task() }
	cases := []struct {
		name string
		task *tasks.Task
		want MoveKind
		says string
	}{
		{"moved to Done: the coordinator wraps up", done.task(), StartTurn, "I moved this task to Done"},
		{"moved to the Backlog: the same", parked.task(), StartTurn, "I moved this task to the Backlog"},
		{"its summary ends it", wrapUp(done, `{"action":"done","summary":"Half done."}`), EndWrapUp, ""},
		{"anything else ends it too", wrapUp(done, `{"action":"assign","agent":"engineer","instructions":"more"}`), EndWrapUp,
			`decided "assign", and the user moved this task to Done: it stays done`},
		{"as does no decision", wrapUp(parked, "I tidied up."), EndWrapUp, "no usable decision"},
		{"or a turn that failed", done.add(worker, events.TurnStarted, "turn-3", map[string]any{"agent": "tech-lead", "hash": "h"}).
			add(watchdog, events.IssueRaised, "", map[string]any{"reason": "the engine crashed"}).task(), EndWrapUp, "ended without a reply"},
		{"then nothing runs", done.turn("turn-3", "tech-lead", "x").
			add(coordinator, events.TaskWrappedUp, "", map[string]any{"summary": "Half done."}).task(), Wait, ""},
		{"nor in the Backlog", parked.turn("turn-3", "tech-lead", "x").
			add(watchdog, events.TaskWrappedUp, "", map[string]any{"reason": "no usable decision"}).task(), Wait, ""},
	}
	for _, c := range cases {
		move := Next(c.task, defaultRoster())
		said := move.Agent.Name + " " + move.Handoff + " " + move.Reason
		if move.Kind != c.want || !strings.Contains(said, c.says) {
			t.Errorf("%s: got %s (%q), want %s with %q", c.name, move.Kind, said, c.want, c.says)
		}
	}
	if move := Next(wrapUp(done, `{"action":"done","summary":"Half done."}`), defaultRoster()); move.Decision.Summary != "Half done." {
		t.Errorf("the summary: %+v", move)
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

// Work you moved to Shipping ships as it is: once an agent that ships work
// has pushed it, the coordinator can finish without another check. Only a
// change after that needs checking again, and a ship the coordinator chose
// waives nothing. A ship turn that crashed, that you stopped or that ended
// without a finished push hasn't shipped it, but the next ship turn is
// still yours if nothing came between.
func TestYourShipTakesTheWorkAsItIs(t *testing.T) {
	const finish = `{"action": "done", "summary": "Shipped."}`
	built := started().
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove them."}).
		turn("turn-1", "engineer", "Removed 3.").
		turn("turn-2", "tech-lead", `{"action": "assign", "agent": "deployer", "instructions": "ship"}`)
	toDeployer := map[string]any{"agent": "deployer", "instructions": "Ship it.", "ships": true}
	yours := built.add(you, events.TaskDirected, "", map[string]any{"text": "Ship this task now.", "column": "shipping"}).
		add(coordinator, events.TaskAssigned, "", toDeployer).
		shipTurn("turn-3", "Pushed.", pushed)
	theirs := built.add(coordinator, events.TaskAssigned, "", toDeployer).
		shipTurn("turn-3", "Pushed.", pushed)
	changed := yours.add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "One more."}).
		turn("turn-4", "engineer", "Changed it.")
	cutShort := built.add(you, events.TaskDirected, "", map[string]any{"text": "Ship this task now.", "column": "shipping"}).
		add(coordinator, events.TaskAssigned, "", toDeployer).
		add(worker, events.TurnStarted, "turn-3", map[string]any{"agent": "deployer", "hash": "h"})
	crashed := cutShort.add(watchdog, events.IssueRaised, "", map[string]any{"reason": "the agent's process exited"})
	stopped := cutShort.add(you, events.TurnStopped, "turn-3", map[string]any{"text": "Wait."})
	// The coordinator hands it to the deployer again: no "ships" flag, as the
	// handoff cleared the column you moved it to.
	again := map[string]any{"agent": "deployer", "instructions": "Ship it."}
	retried := crashed.turn("turn-4", "tech-lead", `{"action": "assign", "agent": "deployer", "instructions": "Ship it."}`).
		add(coordinator, events.TaskAssigned, "", again).
		shipTurn("turn-5", "Pushed.", pushed)
	retriedAfterAChange := crashed.add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "One more."}).
		turn("turn-4", "engineer", "Changed it.").
		add(coordinator, events.TaskAssigned, "", again).
		shipTurn("turn-5", "Pushed.", pushed)
	yoursUntil := func(report string, steps ...func(history, string) history) history {
		return built.add(you, events.TaskDirected, "", map[string]any{"text": "Ship this task now.", "column": "shipping"}).
			add(coordinator, events.TaskAssigned, "", toDeployer).
			shipTurn("turn-3", report, steps...)
	}
	unfinished := yoursUntil("The push hung.", push(events.ActionApproved, false))
	denied := yoursUntil("The push was denied.", push(events.ActionDenied, false))
	testsFailed := yoursUntil("TestParse failed after the merge, so I didn't push.")
	retriedAfterADenial := denied.turn("turn-4", "tech-lead", `{"action": "assign", "agent": "deployer", "instructions": "Ship it."}`).
		add(coordinator, events.TaskAssigned, "", again).
		shipTurn("turn-5", "Pushed.", pushed)
	cases := []struct {
		name string
		h    history
		want MoveKind
	}{
		{"shipped as you chose", yours.turn("turn-9", "tech-lead", finish), ApplyDecision},
		{"your ship's push never finished", unfinished.turn("turn-9", "tech-lead", finish), RaiseIssue},
		{"you denied the push", denied.turn("turn-9", "tech-lead", finish), RaiseIssue},
		{"the tests after the merge failed", testsFailed.turn("turn-9", "tech-lead", finish), RaiseIssue},
		{"pushed on the try after your denial", retriedAfterADenial.turn("turn-9", "tech-lead", finish), ApplyDecision},
		{"shipped as the coordinator chose", theirs.turn("turn-9", "tech-lead", finish), RaiseIssue},
		{"changed after your ship", changed.turn("turn-9", "tech-lead", finish), RaiseIssue},
		{"your ship crashed", crashed.turn("turn-9", "tech-lead", finish), RaiseIssue},
		{"you stopped your ship", stopped.turn("turn-9", "tech-lead", finish), RaiseIssue},
		{"shipped on the next try", retried.turn("turn-9", "tech-lead", finish), ApplyDecision},
		{"shipped again after a change", retriedAfterAChange.turn("turn-9", "tech-lead", finish), RaiseIssue},
	}
	for _, c := range cases {
		if move := Next(c.h.task(), defaultRoster()); move.Kind != c.want || c.want == RaiseIssue && !strings.Contains(move.Reason, "the work isn't checked yet") {
			t.Errorf("%s: got %s (%q), want %s", c.name, move.Kind, move.Reason, c.want)
		}
	}

	// What the coordinator reads after a ship that didn't push: the
	// deployer's reason, and that the work still waits on a check or a push.
	roster := defaultRoster()
	lead, _ := roster.Get("tech-lead")
	for name, h := range map[string]history{"denied": denied, "tests failed": testsFailed} {
		task := h.task()
		brief := coordinatorBrief(task, lead, roster, family{}, nil, nil, nil)
		report := task.Turns[len(task.Turns)-1].Report
		if !strings.Contains(brief, "deployer → tech-lead: "+report) || !strings.Contains(brief, "Now: the work isn't checked yet") {
			t.Errorf("%s: the coordinator's brief should give the deployer's reason and say the work isn't checked:\n%s", name, brief)
		}
	}
}

// Only a ship you chose tells the agent that ships it to skip the earlier
// review: a ship the coordinator chose, or a builder, gets no such line.
func TestYourShipTellsTheShipperToShipItAsItIs(t *testing.T) {
	roster := defaultRoster()
	deployer, _ := roster.Get("deployer")
	engineer, _ := roster.Get("engineer")
	built := started().
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove them."}).
		turn("turn-1", "engineer", "Removed 3.").
		turn("turn-2", "tech-lead", `{"action": "assign", "agent": "deployer", "instructions": "ship"}`)
	yours := built.add(you, events.TaskDirected, "", map[string]any{"text": "Ship this task now.", "column": "shipping"}).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "deployer", "instructions": "Ship it.", "ships": true}).task()
	theirs := built.add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "deployer", "instructions": "Ship it."}).task()

	if brief := workerBrief(yours, deployer, "Ship it.", nil); !strings.Contains(brief, "Ship it.\n\n"+YouChoseToShip) {
		t.Errorf("your ship's brief should say you chose to ship it:\n%s", brief)
	}
	for name, brief := range map[string]string{
		"the coordinator's ship": workerBrief(theirs, deployer, "Ship it.", nil),
		"a builder":              workerBrief(yours, engineer, "Ship it.", nil),
	} {
		if strings.Contains(brief, YouChoseToShip) {
			t.Errorf("%s's brief shouldn't say you chose to ship it:\n%s", name, brief)
		}
	}
}
