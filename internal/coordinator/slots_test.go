package coordinator

import (
	"path/filepath"
	"slices"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/settings"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// named is the task h builds, as the board's task id at rank.
func named(h history, id string, rank float64) *tasks.Task {
	task := h.task()
	task.ID, task.Rank = id, rank
	return task
}

func queuedOf(slots Slots, all []*tasks.Task) []string {
	var queued []string
	for _, task := range all {
		if slots.Queued(task.ID) {
			queued = append(queued, task.ID)
		}
	}
	return queued
}

func TestOnlyTasksInProgressTakeASlot(t *testing.T) {
	working := started().add(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "tech-lead", "hash": "h"})
	asked := started().turn("turn-1", "tech-lead", `{"action": "ask", "question": "Which?"}`).
		add(coordinator, events.TaskNeedsYou, "", map[string]any{"question": "Which?"})
	parked := working.add(you, events.TaskHeld, "", map[string]any{"place": "todo"})
	done := working.add(you, events.TaskDone, "", map[string]any{"summary": "Moved.", "moved": true})
	all := []*tasks.Task{named(working, "task-1", 1), named(asked, "task-2", 2), named(parked, "task-3", 3), named(done, "task-4", 4),
		named(started(), "task-5", 5)}

	slots := SlotsOf(all, defaultRoster(), 1)
	if slots.Running != 1 {
		t.Errorf("only the task in progress with a turn open runs, and a wrap-up never counts: %d running", slots.Running)
	}
	if queued := queuedOf(slots, all); !slices.Equal(queued, []string{"task-5"}) {
		t.Errorf("only the task wanting a turn waits for a slot; waiting for you, parked or done waits for nothing: %v", queued)
	}
}

func TestSlotsGoInYourPriorityOrder(t *testing.T) {
	// Nothing runs yet: the first by rank is admitted, and holds its slot
	// until its turn is recorded, so a turn being started counts.
	all := []*tasks.Task{named(started(), "task-1", 3), named(started(), "task-2", 1), named(started(), "task-3", 2)}
	slots := SlotsOf(all, defaultRoster(), 2)
	if queued := queuedOf(slots, all); !slices.Equal(queued, []string{"task-1"}) || slots.Running != 0 {
		t.Errorf("two slots: the two first by rank should be admitted: queued %v, %d running", queued, slots.Running)
	}
	if queued := queuedOf(SlotsOf(all, defaultRoster(), 0), all); queued != nil {
		t.Errorf("with no limit nothing waits: %v", queued)
	}
	if queued := queuedOf(SlotsOf(all, defaultRoster(), 3), all); queued != nil {
		t.Errorf("a slot for each: a task between turns isn't queued: %v", queued)
	}
}

// On a tie of rank the newer task comes first, as on the board, which
// lists tasks in the same order.
func TestATieOfRankAdmitsTheNewerTask(t *testing.T) {
	older, newer := named(started(), "task-1", 1), named(started(), "task-2", 1)
	newer.CreatedAt = older.CreatedAt.Add(time.Second)
	all := []*tasks.Task{older, newer}
	if queued := queuedOf(SlotsOf(all, defaultRoster(), 1), all); !slices.Equal(queued, []string{"task-1"}) {
		t.Errorf("the newer task should take the one slot: queued %v", queued)
	}
}

func TestALimitUnderWhatRunsQueuesEveryOneWaiting(t *testing.T) {
	working := started().add(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "tech-lead", "hash": "h"})
	all := []*tasks.Task{named(working, "task-1", 1), named(working, "task-2", 2), named(started(), "task-3", 0)}
	slots := SlotsOf(all, defaultRoster(), 1)
	if queued := queuedOf(slots, all); !slices.Equal(queued, []string{"task-3"}) || slots.Running != 2 {
		t.Errorf("two running over a limit of one: queued %v, %d running", queued, slots.Running)
	}
}

// The loop with a limit of two and four tasks ready to start, as a restart
// finds them. They start by rank, two at a time, as slots free.
func TestTheLoopRunsNoMoreTasksThanTheLimit(t *testing.T) {
	fakeClaudeOnPath(t)
	repo := gitRepo(t)
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	setLimit(t, s, 2)
	// Made in the Backlog, ranked, then all started before the loop runs:
	// the loop's first pass reads them in log order, not in rank order.
	ranks := map[string]float64{"task-1": 2, "task-2": 4, "task-3": 1, "task-4": 3}
	for _, id := range []string{"task-1", "task-2", "task-3", "task-4"} {
		propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
			Data: map[string]any{"title": id, "repo": repo, "coordinator": "tech-lead", "parked": "backlog"}})
		propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskRanked, Data: map[string]any{"rank": ranks[id]}})
	}
	for _, id := range []string{"task-1", "task-2", "task-3", "task-4"} {
		propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskHeld,
			Data: map[string]any{"place": "", "column": "in-progress", "sentBack": false}})
	}
	run(t, newLoop(s, state, t.TempDir()), state)

	waitForTurn(t, s, "task-3", "turn-1", "tech-lead")
	waitForTurn(t, s, "task-1", "turn-1", "tech-lead")
	assertSlots(t, state, 2, "task-4", "task-2")

	// task-3 now waits for you: its slot goes to task-4, next by rank.
	endTurn(t, s, "task-3", "turn-1", "tech-lead", `{"action": "ask", "question": "Which?"}`)
	waitForTurn(t, s, "task-4", "turn-1", "tech-lead")
	assertSlots(t, state, 2, "task-2")

	// task-1's turn ends and it wants the next: it comes before task-2.
	endTurn(t, s, "task-1", "turn-1", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Go."}`)
	waitForTurn(t, s, "task-1", "turn-2", "engineer")
	assertSlots(t, state, 2, "task-2")

	// Lowered under what runs: nothing stops, and nothing starts.
	lowered := setLimit(t, s, 1)
	endTurn(t, s, "task-4", "turn-1", "tech-lead", `{"action": "ask", "question": "Which?"}`)
	waitUntil(t, s, "task-4", "a question", func(task *tasks.Task) bool { return task.Question != "" })
	assertSlots(t, state, 1, "task-2")

	// Raised: task-2 starts at last, only after it was raised, and the
	// engineer's turn ran on all along.
	raised := setLimit(t, s, 3)
	waitForTurn(t, s, "task-2", "turn-1", "tech-lead")
	if started := turnStartedAt(t, s, "task-2", "turn-1"); started < raised {
		t.Errorf("task-2 started at %d, before the limit was raised at %d (lowered at %d)", started, raised, lowered)
	}
	if task := waitForTurn(t, s, "task-1", "turn-2", "engineer"); len(task.Turns) != 2 {
		t.Errorf("the engineer's turn should still be the open one: %+v", task.Turns)
	}
	assertSlots(t, state, 2)
}

func TestClearingTheLimitStartsEveryTaskWaiting(t *testing.T) {
	fakeClaudeOnPath(t)
	repo := gitRepo(t)
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	setLimit(t, s, 1)
	run(t, newLoop(s, state, t.TempDir()), state)
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
			Data: map[string]any{"title": id, "repo": repo, "coordinator": "tech-lead"}})
	}
	// Which starts first depends on how far the loop had read when the
	// others came; the other two wait, the newest first.
	for deadline := time.Now().Add(10 * time.Second); len(waiting(state)) != 2 && time.Now().Before(deadline); {
		time.Sleep(20 * time.Millisecond)
	}
	assertSlots(t, state, 1, waiting(state)...)

	setLimit(t, s, 0)
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		waitForTurn(t, s, id, "turn-1", "tech-lead")
	}
	assertSlots(t, state, 3)
}

// setLimit sets the most tasks running at once, and is the position it was set at.
func setLimit(t *testing.T, s *store.Store, limit int) int64 {
	t.Helper()
	e, rejection, err := s.Propose(events.Proposal{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged,
		Data: map[string]any{settings.MaxRunningTasks: limit}})
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	return e.Position
}

// assertSlots waits until running tasks run and queued wait for a slot,
// by rank, as the loop catches up with the log.
func assertSlots(t *testing.T, state *current.State, running int, queued ...string) {
	t.Helper()
	var slots Slots
	var got []string
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		all := state.Tasks()
		slices.SortStableFunc(all, tasks.InPriorityOrder)
		slots = SlotsOf(all, state.Roster(), state.Settings().MaxRunningTasks)
		if got = queuedOf(slots, all); slots.Running == running && slices.Equal(got, queued) {
			return
		}
	}
	t.Errorf("%d running, queued %v; want %d running, queued %v", slots.Running, got, running, queued)
}

// waiting is the tasks with no turn yet, by rank.
func waiting(state *current.State) []string {
	all := state.Tasks()
	slices.SortStableFunc(all, tasks.InPriorityOrder)
	var ids []string
	for _, task := range all {
		if len(task.Turns) == 0 {
			ids = append(ids, task.ID)
		}
	}
	return ids
}

func turnStartedAt(t *testing.T, s *store.Store, id, turn string) int64 {
	t.Helper()
	for _, e := range mustStream(t, s, id) {
		if e.Type == events.TurnStarted && e.Correlation == turn {
			return e.Position
		}
	}
	t.Fatalf("%s has no %s", id, turn)
	return 0
}

// A task held for a slot whose next move is then no turn, as when you park
// it, is let go: waking it later would make that move a second time.
func TestATaskHeldIsLetGoOnceItsMoveIsNoTurn(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	setLimit(t, s, 1)
	for i, id := range []string{"task-1", "task-2"} {
		propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
			Data: map[string]any{"title": id, "repo": "/r", "coordinator": "tech-lead"}})
		propose(t, s, events.Proposal{Stream: id, Actor: worker, Type: events.WorktreeReady,
			Data: map[string]any{"path": "/w/" + id, "branch": "b", "port": 7901 + i}})
	}
	propose(t, s, events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1",
		Data: map[string]any{"agent": "tech-lead", "hash": "h"}})
	loop := newLoop(s, state, t.TempDir())

	loop.act("task-2")
	if !loop.held["task-2"] {
		t.Fatal("task-2 wants a turn and task-1 has the one slot: it should be held")
	}
	propose(t, s, events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskHeld, Data: map[string]any{"place": "todo"}})
	loop.act("task-2")
	if len(loop.held) != 0 {
		t.Errorf("task-2 is parked: nothing should be held, got %v", loop.held)
	}
}

// A turn stopped at a usage limit stays open, so its task keeps its slot
// while it waits for the reset, and the resume goes on in that same slot:
// nothing waiting starts in between. Once the resumed turn ends, the slot
// goes to the first waiting by rank.
func TestATurnAtAUsageLimitKeepsItsSlot(t *testing.T) {
	fakeClaudeOnPath(t)
	repo := gitRepo(t)
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	setLimit(t, s, 1)
	ranks := map[string]float64{"task-1": 1, "task-2": 3, "task-3": 2}
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
			Data: map[string]any{"title": id, "repo": repo, "coordinator": "tech-lead", "parked": "backlog"}})
		propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskRanked, Data: map[string]any{"rank": ranks[id]}})
	}
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskHeld,
			Data: map[string]any{"place": "", "column": "in-progress", "sentBack": false}})
	}
	run(t, newLoop(s, state, t.TempDir()), state)
	waitForTurn(t, s, "task-1", "turn-1", "tech-lead")
	assertSlots(t, state, 1, "task-3", "task-2")

	propose(t, s, events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnLimited, Correlation: "turn-1",
		Data: map[string]any{"agent": "tech-lead", "error": "rate_limit", "details": "limit reached", "resetsAt": ""}})
	waitUntil(t, s, "task-1", "the usage limit", func(task *tasks.Task) bool { return task.OpenTurn().Limited != "" })
	assertSlots(t, state, 1, "task-3", "task-2")

	propose(t, s, events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.System, ID: "watchdog"}, Type: events.TurnResumed,
		Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "reason": "reset"}})
	if task := waitForTurn(t, s, "task-1", "turn-1", "tech-lead"); task.OpenTurn().Resumes != 1 || len(task.Turns) != 1 {
		t.Errorf("the resume should go on in the same open turn: %+v", task.Turns)
	}
	assertSlots(t, state, 1, "task-3", "task-2")

	endTurn(t, s, "task-1", "turn-1", "tech-lead", `{"action": "ask", "question": "Which?"}`)
	waitForTurn(t, s, "task-3", "turn-1", "tech-lead")
	assertSlots(t, state, 1, "task-2")
}
