package tasks

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// story is a story of epic-1 with the given turns.
func story(id string, turns ...*Turn) *Task {
	return &Task{ID: id, Epic: "epic-1", Turns: turns}
}

// idOf is the task's id, "<nil>" for none.
func idOf(task *Task) string {
	if task == nil {
		return "<nil>"
	}
	return task.ID
}

// endedAt is a resumable engineer's turn with session, started at minute.
func endedAt(minute int, session string) *Turn {
	turn := resumable()
	turn.EngineSession, turn.StartedAt = session, time.Date(2026, 10, 25, 9, minute, 0, 0, time.UTC)
	return turn
}

func TestAFirstTurnOnAStoryGoesOnWithTheLatestSessionOfASibling(t *testing.T) {
	first := story("task-2", endedAt(1, "s-old"), endedAt(20, "s-2"))
	second := story("task-3", endedAt(10, "s-3"))
	running := endedAt(40, "")
	running.Ended, running.FromSession = false, "s-2" // a sibling's turn in progress, passed over
	second.Turns = append(second.Turns, running)
	reviewer := story("task-4", &Turn{Agent: "reviewer", Engine: "claude", Worker: "tower", EngineSession: "r-1", Ended: true, StartedAt: endedAt(30, "").StartedAt})
	next := story("task-5")
	session, from, fresh := next.SessionToShare([]*Task{first, second, reviewer, next}, "engineer", "claude", "tower")
	if session != "s-2" || from != first {
		t.Errorf("got %q from %s, fresh because %q; want s-2, the engineer's latest across the stories, from task-2", session, idOf(from), fresh)
	}
}

// The sibling's latest turn is held to story 2's rules; a failed or
// stopped one isn't passed over for an older one.
func TestASiblingsSessionIsHeldToTheSameRules(t *testing.T) {
	for _, c := range []struct {
		name   string
		change func(*Turn)
		engine string
		worker string
		want   string
	}{
		{"failed, context 0", func(turn *Turn) { turn.Issue, turn.Context = "the session ended without a report", 0 }, "claude", "tower", "its last turn failed"},
		{"resume failed", func(turn *Turn) { turn.Issue, turn.FromSession = "its resumed session didn't start", "s-0" }, "claude", "tower", "its resume failed"},
		{"stopped", func(turn *Turn) { turn.Stopped = true }, "claude", "tower", "its last turn was stopped"},
		{"another engine", func(*Turn) {}, "codex", "tower", "another engine"},
		{"another machine", func(*Turn) {}, "claude", "workstation", "the work moved to workstation"},
		{"no session id", func(turn *Turn) { turn.EngineSession = "" }, "claude", "tower", "no session id"},
		{"too big", func(turn *Turn) { turn.Context = MaxResumedContext + 1 }, "claude", "tower", "context too big (151k tokens)"},
	} {
		latest := endedAt(20, "s-2")
		c.change(latest)
		next := story("task-5")
		stories := []*Task{story("task-2", endedAt(1, "s-1")), story("task-3", latest), next}
		if session, from, fresh := next.SessionToShare(stories, "engineer", c.engine, c.worker); session != "" || from != nil || fresh != c.want {
			t.Errorf("%s: got %q from %s, fresh because %q; want fresh because %q", c.name, session, idOf(from), fresh, c.want)
		}
	}
}

func TestAStorysOwnSessionWinsOverANewerSiblings(t *testing.T) {
	own := story("task-5", endedAt(1, "s-own"))
	stories := []*Task{story("task-2", endedAt(20, "s-2")), own}
	if session, from, fresh := own.SessionToShare(stories, "engineer", "claude", "tower"); session != "s-own" || from != own {
		t.Errorf("got %q from %s, fresh because %q; want the story's own s-own", session, idOf(from), fresh)
	}

	own.Turns[0].Stopped = true
	if session, _, fresh := own.SessionToShare(stories, "engineer", "claude", "tower"); session != "" || fresh != "its last turn was stopped" {
		t.Errorf("got %q, fresh because %q; want its own last turn's reason, not a sibling's session", session, fresh)
	}
}

func TestATaskThatIsNoStoryHasOnlyItsOwnSession(t *testing.T) {
	task := &Task{ID: "task-1"}
	if session, from, fresh := task.SessionToShare(nil, "engineer", "claude", "tower"); session != "" || from != nil || fresh != "first turn" {
		t.Errorf("got %q from %s, fresh because %q; want a first turn", session, idOf(from), fresh)
	}
}

// Busy is an open turn in the session, or one that started from it: a
// fork's own turn has a new session id but still says where it came from.
func TestASessionIsBusyWhileATurnIsOpenInItOrFromIt(t *testing.T) {
	open := func(change func(*Turn)) *Task {
		turn := &Turn{Agent: "engineer", Engine: "claude", Worker: "tower"}
		change(turn)
		return story("task-9", turn)
	}
	for _, c := range []struct {
		name string
		task *Task
		busy bool
	}{
		{"open in it", open(func(turn *Turn) { turn.EngineSession = "s-1" }), true},
		{"open from it", open(func(turn *Turn) { turn.FromSession, turn.EngineSession = "s-1", "s-fork" }), true},
		{"open from it, before the session says its id", open(func(turn *Turn) { turn.FromSession = "s-1" }), true},
		{"ended in it", story("task-9", endedAt(1, "s-1")), false},
		{"open in another", open(func(turn *Turn) { turn.EngineSession, turn.FromSession = "s-2", "s-0" }), false},
		{"no turns", story("task-9"), false},
	} {
		if busy := SessionBusy("s-1", []*Task{story("task-2", endedAt(1, "s-1")), c.task}); busy != c.busy {
			t.Errorf("%s: busy %v, want %v", c.name, busy, c.busy)
		}
	}
}

// Logs from before story 3 have no sessionFrom or forked: both apply empty.
func TestTurnStartedReadsWhereItsSessionCameFrom(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	worker := events.Actor{Kind: events.Worker, ID: "tower"}
	started := func(data map[string]any) *Turn {
		event := func(actor events.Actor, kind events.Type, correlation string, data map[string]any) events.Event {
			return events.Event{At: time.Now(), Proposal: events.Proposal{Stream: "task-3", Actor: actor, Type: kind, Correlation: correlation, Data: data}}
		}
		return FromEvents([]events.Event{
			event(you, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead"}),
			event(lead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"}),
			event(worker, events.TurnStarted, "turn-1", data),
		}).Turn("turn-1")
	}
	old := started(map[string]any{"agent": "engineer", "hash": "h", "worker": "tower", "engine": "claude", "fromSession": "s-1"})
	if old.SessionFrom != "" || old.Forked {
		t.Errorf("an old turn.started applies empty: from %q, forked %v", old.SessionFrom, old.Forked)
	}
	forked := started(map[string]any{"agent": "engineer", "hash": "h", "worker": "tower", "engine": "claude", "fromSession": "s-1", "sessionFrom": "task-2", "forked": true})
	if forked.SessionFrom != "task-2" || !forked.Forked || forked.FromSession != "s-1" {
		t.Errorf("a forked turn from task-2: from %q, forked %v, session %q", forked.SessionFrom, forked.Forked, forked.FromSession)
	}
}

// A story's first turn went on with task-2's s-1, in place or forked, and
// its process exited before it took its prompt. The retry is that story's
// own failed turn's, so it starts fresh and doesn't go back to task-2's
// session; the fresh turn's own session is then the one to go on with.
func TestAFailedResumeFromASiblingRetriesFreshNotTheSiblingAgain(t *testing.T) {
	for _, forked := range []bool{false, true} {
		you := events.Actor{Kind: events.Human, ID: "you"}
		lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
		worker := events.Actor{Kind: events.Worker, ID: "tower"}
		dog := events.Actor{Kind: events.System, ID: "watchdog"}
		event := func(actor events.Actor, kind events.Type, correlation string, data map[string]any) events.Event {
			return events.Event{At: time.Now(), Proposal: events.Proposal{Stream: "task-3", Actor: actor, Type: kind, Correlation: correlation, Data: data}}
		}
		started := map[string]any{"agent": "engineer", "hash": "h2", "worker": "tower", "engine": "claude", "fromSession": "s-1", "sessionFrom": "task-2"}
		if forked {
			started["forked"] = true
		}
		log := []events.Event{
			event(you, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead", "epic": "epic-1"}),
			event(lead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Add the lint."}),
			event(worker, events.TurnStarted, "turn-1", started),
			event(dog, events.IssueRaised, "", map[string]any{"reason": "engineer's session didn't start", "retryFresh": true}),
		}
		sibling := story("task-2", endedAt(1, "s-1"))
		task := FromEvents(log)
		if task.Handoff == nil || task.Handoff.To != "engineer" || task.Handoff.Text != "Add the lint." {
			t.Errorf("forked %v: handoff %+v, want the same step back to the engineer", forked, task.Handoff)
		}
		if session, from, fresh := task.SessionToShare([]*Task{sibling, task}, "engineer", "claude", "tower"); session != "" || fresh != "its resume failed" {
			t.Errorf("forked %v: the retry goes on with %q from %s, fresh because %q; want fresh because its resume failed", forked, session, idOf(from), fresh)
		}

		log = append(log,
			event(worker, events.TurnStarted, "turn-2", map[string]any{"agent": "engineer", "hash": "h3", "worker": "tower", "engine": "claude", "freshBecause": "its resume failed"}),
			event(worker, events.SessionStarted, "turn-2", map[string]any{"engineSession": "s-3"}),
			event(worker, events.TurnEnded, "turn-2", map[string]any{"agent": "engineer", "report": "Added it."}))
		task = FromEvents(log)
		if session, from, fresh := task.SessionToShare([]*Task{sibling, task}, "engineer", "claude", "tower"); session != "s-3" || from != task {
			t.Errorf("forked %v: after the fresh retry, got %q from %s, fresh because %q; want its own s-3", forked, session, idOf(from), fresh)
		}
	}
}
