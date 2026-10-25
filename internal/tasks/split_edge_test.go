package tasks

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// oldTurnsLog is a task as a log from before the split has it: no
// session.started, one turn with a total, one whose count was unknown.
func oldTurnsLog() []events.Event {
	you := events.Actor{Kind: events.Human, ID: "you"}
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	var log []events.Event
	add := func(actor events.Actor, kind events.Type, correlation string, data map[string]any) {
		log = append(log, events.Event{Position: int64(len(log) + 1), At: time.Date(2026, 10, 1, 9, 0, len(log), 0, time.UTC),
			Proposal: events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Correlation: correlation, Data: data}})
	}
	add(you, events.TaskCreated, "", map[string]any{"title": "t", "coordinator": "tech-lead"})
	add(lead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"})
	add(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "engineer", "hash": "h"})
	add(worker, events.TurnEnded, "turn-1", map[string]any{"agent": "engineer", "report": "Done.", "tokens": float64(2500)})
	add(lead, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "again"})
	add(worker, events.TurnStarted, "turn-2", map[string]any{"agent": "engineer", "hash": "h2"})
	add(worker, events.TurnEnded, "turn-2", map[string]any{"agent": "engineer", "report": "Done.", "tokens": float64(0), "tokensUnknown": "transcript not saved"})
	return log
}

// An old log replays to the same totals: every token is unsplit and no
// turn has a session.
func TestAnOldLogIsAllUnsplit(t *testing.T) {
	task := FromEvents(oldTurnsLog())
	if task.TokensUsed != 2500 || task.TokensSplit != (Split{}) || task.Unsplit() != 2500 {
		t.Errorf("used %d, split %+v, unsplit %d; want 2500, none, 2500", task.TokensUsed, task.TokensSplit, task.Unsplit())
	}
	for _, turn := range task.Turns {
		if turn.Split != (Split{}) || turn.EngineSession != "" {
			t.Errorf("%s: split %+v, session %q", turn.ID, turn.Split, turn.EngineSession)
		}
	}
	if turn := task.Turn("turn-2"); turn.TokensUnknown != "transcript not saved" {
		t.Errorf("turn-2 unknown %q", turn.TokensUnknown)
	}
}

// A session.started for a turn the task doesn't have changes nothing.
func TestASessionForNoTurnIsIgnored(t *testing.T) {
	log := oldTurnsLog()
	before := len(FromEvents(log).Turns)
	log = append(log, events.Event{Position: int64(len(log) + 1), At: time.Date(2026, 10, 1, 10, 0, 0, 0, time.UTC),
		Proposal: events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Worker, ID: "local"}, Type: events.SessionStarted,
			Correlation: "turn-9", Data: map[string]any{"engineSession": "x"}}})
	task := FromEvents(log)
	if len(task.Turns) != before {
		t.Errorf("turns %d, want %d", len(task.Turns), before)
	}
	for _, turn := range task.Turns {
		if turn.EngineSession != "" {
			t.Errorf("%s got session %q", turn.ID, turn.EngineSession)
		}
	}
}

// A snapshot keeps the session and split, so a restart shows the same.
func TestASavedTaskKeepsItsSessionAndSplit(t *testing.T) {
	task := FromEvents(turnsLog())
	restored := Restore(task.Save())
	if restored.TokensSplit != task.TokensSplit || restored.Unsplit() != task.Unsplit() {
		t.Errorf("restored split %+v/%d, want %+v/%d", restored.TokensSplit, restored.Unsplit(), task.TokensSplit, task.Unsplit())
	}
	if turn := restored.Turn("turn-1"); turn.EngineSession != "4f1c" || turn.Split != task.Turn("turn-1").Split {
		t.Errorf("restored turn-1: session %q split %+v", turn.EngineSession, turn.Split)
	}
}
