package tasks

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// resumable is an engineer's turn its next one resumes: ended, on claude on
// omni, with a session id.
func resumable() *Turn {
	return &Turn{ID: "turn-1", Agent: "engineer", Engine: "claude", Worker: "omni", EngineSession: "s-1", Ended: true}
}

func TestATurnResumesOnlyTheSameAgentsSessionOnTheSameEngineAndMachine(t *testing.T) {
	for _, c := range []struct {
		name          string
		change        func(*Turn)
		engine        string
		worker        string
		session, want string
	}{
		{"resumes", func(*Turn) {}, "claude", "omni", "s-1", ""},
		{"first turn", func(turn *Turn) { turn.Agent = "reviewer" }, "claude", "omni", "", "first turn"},
		{"not ended", func(turn *Turn) { turn.Ended = false }, "claude", "omni", "", "its last turn hasn't ended"},
		{"work moved", func(*Turn) {}, "claude", "workstation", "", "the work moved to workstation"},
		{"another engine", func(*Turn) {}, "codex", "omni", "", "another engine"},
		{"engine unknown", func(turn *Turn) { turn.Engine = "" }, "claude", "omni", "", "engine unknown"},
		{"no session id", func(turn *Turn) { turn.EngineSession = "" }, "claude", "omni", "", "no session id"},
		{"too big", func(turn *Turn) { turn.Split = Split{Input: 2_000, CacheRead: 180_000} }, "claude", "omni", "", "context too big (182k tokens)"},
		{"unknown split", func(turn *Turn) { turn.Split = Split{} }, "claude", "omni", "s-1", ""},
	} {
		turn := resumable()
		c.change(turn)
		task := &Task{Turns: []*Turn{turn}}
		if session, fresh := task.SessionToResume("engineer", c.engine, c.worker); session != c.session || fresh != c.want {
			t.Errorf("%s: got %q, fresh because %q; want %q, %q", c.name, session, fresh, c.session, c.want)
		}
	}
}

// The size is the last turn's cache reads plus its new input; cache writes
// and output don't count.
func TestAContextAtTheLimitStillResumes(t *testing.T) {
	for _, c := range []struct {
		name   string
		split  Split
		resume bool
	}{
		{"just under", Split{Input: 1_000, CacheRead: MaxResumedContext - 1_001}, true},
		{"at", Split{Input: 1_000, CacheRead: MaxResumedContext - 1_000}, true},
		{"just over", Split{Input: 1_000, CacheRead: MaxResumedContext - 999}, false},
		{"writes and output don't count", Split{CacheRead: MaxResumedContext, CacheWrite: 50_000, Output: 50_000}, true},
	} {
		turn := resumable()
		turn.Split = c.split
		task := &Task{Turns: []*Turn{turn}}
		if session, fresh := task.SessionToResume("engineer", "claude", "omni"); (session != "") != c.resume {
			t.Errorf("%s: got %q, fresh because %q; want resume %v", c.name, session, fresh, c.resume)
		}
	}
}

func TestOnlyTheAgentsOwnLatestTurnCounts(t *testing.T) {
	older := resumable()
	older.EngineSession = "s-0"
	latest := resumable()
	latest.ID, latest.EngineSession = "turn-3", "s-1"
	reviewer := &Turn{ID: "turn-2", Agent: "reviewer", Engine: "claude", Worker: "omni", EngineSession: "r-1", Ended: true}
	running := &Turn{ID: "turn-4", Agent: "reviewer", Engine: "codex", Worker: "workstation"} // still open, elsewhere
	task := &Task{Turns: []*Turn{older, reviewer, latest, running}}
	if session, fresh := task.SessionToResume("engineer", "claude", "omni"); session != "s-1" {
		t.Errorf("the engineer resumes its latest session, past the reviewer's turns: got %q, fresh because %q", session, fresh)
	}

	latest.EngineSession = ""
	if session, fresh := task.SessionToResume("engineer", "claude", "omni"); session != "" || fresh != "no session id" {
		t.Errorf("an older session isn't picked when the latest has none: got %q, fresh because %q", session, fresh)
	}
}

// Logs from before story 2 have no fromSession or freshBecause; test logs
// may have no engine either. All three apply empty, and an unknown engine
// starts fresh.
func TestTurnStartedReadsItsOptionalFields(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	worker := events.Actor{Kind: events.Worker, ID: "omni"}
	event := func(actor events.Actor, kind events.Type, correlation string, data map[string]any) events.Event {
		return events.Event{At: time.Now(), Proposal: events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Correlation: correlation, Data: data}}
	}
	started := func(data map[string]any) *Task {
		return FromEvents([]events.Event{
			event(you, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead"}),
			event(lead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"}),
			event(worker, events.TurnStarted, "turn-1", data),
			event(worker, events.SessionStarted, "turn-1", map[string]any{"engineSession": "s-1"}),
			event(worker, events.TurnEnded, "turn-1", map[string]any{"report": "done"}),
		})
	}

	old := started(map[string]any{"agent": "engineer", "hash": "h", "worker": "omni"})
	if turn := old.Turn("turn-1"); turn.Engine != "" || turn.FromSession != "" || turn.FreshBecause != "" {
		t.Errorf("an old turn.started applies empty values: %+v", turn)
	}
	if session, fresh := old.SessionToResume("engineer", "claude", "omni"); session != "" || fresh != "engine unknown" {
		t.Errorf("an old turn with no engine starts fresh: got %q, fresh because %q", session, fresh)
	}

	resumed := started(map[string]any{"agent": "engineer", "hash": "h", "worker": "omni", "engine": "claude", "fromSession": "s-0"})
	if turn := resumed.Turn("turn-1"); turn.Engine != "claude" || turn.FromSession != "s-0" || turn.FreshBecause != "" {
		t.Errorf("a resumed turn keeps its engine and the session it resumed: %+v", turn)
	}
	fresh := started(map[string]any{"agent": "engineer", "hash": "h", "worker": "omni", "engine": "claude", "freshBecause": "first turn"})
	if turn := fresh.Turn("turn-1"); turn.FromSession != "" || turn.FreshBecause != "first turn" {
		t.Errorf("a fresh turn keeps why: %+v", turn)
	}
}
