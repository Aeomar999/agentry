package tasks

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/workers"
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
		{"moved here", func(*Turn) {}, "claude", workers.Local, "", "the work moved to local"},
		{"moved here, unnamed", func(*Turn) {}, "claude", "", "", "the work moved to local"},
		{"too big", func(turn *Turn) { turn.Context = 182_000 }, "claude", "omni", "", "context too big (182k tokens)"},
		{"unknown context", func(turn *Turn) { turn.Context = 0 }, "claude", "omni", "s-1", ""},
	} {
		turn := resumable()
		c.change(turn)
		task := &Task{Turns: []*Turn{turn}}
		if session, fresh := task.SessionToResume("engineer", c.engine, c.worker); session != c.session || fresh != c.want {
			t.Errorf("%s: got %q, fresh because %q; want %q, %q", c.name, session, fresh, c.session, c.want)
		}
	}
}

// The size is the last turn's context, the input of its last call. Its
// split sums every call of the turn, so even a big one doesn't count.
func TestAContextAtTheLimitStillResumes(t *testing.T) {
	for _, c := range []struct {
		name    string
		context int
		split   Split
		resume  bool
	}{
		{"just under", MaxResumedContext - 1, Split{}, true},
		{"at", MaxResumedContext, Split{}, true},
		{"just over", MaxResumedContext + 1, Split{}, false},
		{"a big split, a small context", 40_000, Split{Input: 300, CacheWrite: 60_000, CacheRead: 900_000, Output: 20_000}, true},
		{"a big split, an unknown context", 0, Split{Input: 300, CacheWrite: 60_000, CacheRead: 900_000, Output: 20_000}, true},
	} {
		turn := resumable()
		turn.Context, turn.Split = c.context, c.split
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

// A turn logged after the split was kept but before its context was has a
// big split and no contextTokens: its context is unknown, so it resumes.
func TestATurnFromBeforeTheContextWasKeptResumes(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	worker := events.Actor{Kind: events.Worker, ID: "omni"}
	event := func(actor events.Actor, kind events.Type, correlation string, data map[string]any) events.Event {
		return events.Event{At: time.Now(), Proposal: events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Correlation: correlation, Data: data}}
	}
	task := FromEvents([]events.Event{
		event(you, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead"}),
		event(lead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"}),
		event(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "engineer", "hash": "h", "worker": "omni", "engine": "claude", "freshBecause": "first turn"}),
		event(worker, events.SessionStarted, "turn-1", map[string]any{"engineSession": "s-1"}),
		event(worker, events.TurnEnded, "turn-1", map[string]any{"agent": "engineer", "report": "done", "tokens": float64(1_000_400),
			"inputTokens": float64(400), "cacheWriteTokens": float64(60_000), "cacheReadTokens": float64(920_000), "outputTokens": float64(20_000)}),
	})
	if turn := task.Turn("turn-1"); turn.Context != 0 || turn.Split.CacheRead != 920_000 {
		t.Errorf("context %d, split %+v; want 0 and the split", turn.Context, turn.Split)
	}
	if session, fresh := task.SessionToResume("engineer", "claude", "omni"); session != "s-1" {
		t.Errorf("got %q, fresh because %q; want s-1", session, fresh)
	}
}
