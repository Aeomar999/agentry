package tasks

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// turnsLog is a task with two turns on a laptop: the first from a newer
// worker, which says the engine's session and how its tokens split, the
// second as an older log has it, with only the total.
func turnsLog() []events.Event {
	you := events.Actor{Kind: events.Human, ID: "you"}
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	worker := events.Actor{Kind: events.Worker, ID: "laptop"}
	var log []events.Event
	add := func(actor events.Actor, kind events.Type, correlation string, data map[string]any) {
		log = append(log, events.Event{Position: int64(len(log) + 1), At: time.Date(2026, 10, 24, 9, 0, len(log), 0, time.UTC),
			Proposal: events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Correlation: correlation, Data: data}})
	}
	add(you, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead"})
	add(lead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"})
	add(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "engineer", "hash": "h", "worker": "laptop"})
	add(worker, events.SessionStarted, "turn-1", map[string]any{"engineSession": "4f1c"})
	add(worker, events.TurnEnded, "turn-1", map[string]any{"agent": "engineer", "report": "Done.", "tokens": float64(53174),
		"inputTokens": float64(4), "cacheWriteTokens": float64(12800), "cacheReadTokens": float64(40000), "outputTokens": float64(370), "contextTokens": float64(26801)})
	add(lead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "again"})
	add(worker, events.TurnStarted, "turn-2", map[string]any{"agent": "engineer", "hash": "h2", "worker": "laptop"})
	add(worker, events.TurnEnded, "turn-2", map[string]any{"agent": "engineer", "report": "Done again.", "tokens": float64(1000)})
	return log
}

// A turn keeps the engine's session beside the machine it ran on, so a
// later turn can find both, and the four parts of what it used.
func TestATurnKeepsItsEngineSessionAndSplit(t *testing.T) {
	task := FromEvents(turnsLog())
	turn := task.Turn("turn-1")
	if turn.EngineSession != "4f1c" || turn.Worker != "laptop" {
		t.Errorf("session %q on %q, want 4f1c on laptop", turn.EngineSession, turn.Worker)
	}
	if want := (Split{Input: 4, CacheWrite: 12800, CacheRead: 40000, Output: 370}); turn.Split != want || turn.Tokens != 53174 {
		t.Errorf("tokens %d split %+v, want 53174 split %+v", turn.Tokens, turn.Split, want)
	}
	if turn.Context != 26801 {
		t.Errorf("context %d, want its last call's 26801", turn.Context)
	}
}

// A turn ended with only the total, as from an older worker or in an old
// log, has no split and no session: none is made up. Its tokens are in the
// task's total, which the budget counts, and in none of the four.
func TestATurnWithOnlyATotalHasNoSplit(t *testing.T) {
	task := FromEvents(turnsLog())
	if turn := task.Turn("turn-2"); turn.Tokens != 1000 || turn.Split != (Split{}) || turn.EngineSession != "" || turn.Context != 0 {
		t.Errorf("turn-2: tokens %d, split %+v, session %q, context %d", turn.Tokens, turn.Split, turn.EngineSession, turn.Context)
	}
	if task.TokensUsed != 54174 {
		t.Errorf("used %d, want both turns' 54174", task.TokensUsed)
	}
	if want := (Split{Input: 4, CacheWrite: 12800, CacheRead: 40000, Output: 370}); task.TokensSplit != want || task.Unsplit() != 1000 {
		t.Errorf("split %+v and %d unsplit, want %+v and 1000", task.TokensSplit, task.Unsplit(), want)
	}
}

// Next, which the server applies event by event, agrees with FromEvents.
func TestNextKeepsTheSplitLikeFromEvents(t *testing.T) {
	var task *Task
	for _, e := range turnsLog() {
		task = Next(task, e)
	}
	want := FromEvents(turnsLog())
	if task.TokensSplit != want.TokensSplit || task.Turn("turn-1").EngineSession != "4f1c" {
		t.Errorf("Next: split %+v, session %q", task.TokensSplit, task.Turn("turn-1").EngineSession)
	}
}
