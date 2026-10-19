package tasks

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// DoneAt is when the task was last done, whoever finished it, and nothing
// while it is open again.
func TestDoneAtIsWhenTheTaskWasLastDone(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	start := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	var log []events.Event
	add := func(actor events.Actor, kind events.Type, data map[string]any) time.Time {
		at := start.Add(time.Duration(len(log)) * time.Minute)
		log = append(log, events.Event{Position: int64(len(log) + 1), At: at,
			Proposal: events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Data: data}})
		return at
	}
	add(you, events.TaskCreated, map[string]any{"title": "Ship it", "coordinator": "tech-lead"})
	if task := FromEvents(log); !task.DoneAt.IsZero() {
		t.Errorf("a new task: done at %v", task.DoneAt)
	}
	done := add(lead, events.TaskDone, map[string]any{"summary": "Shipped."})
	if task := FromEvents(log); !task.DoneAt.Equal(done) {
		t.Errorf("done by its coordinator: done at %v, want %v", task.DoneAt, done)
	}
	add(you, events.TaskReopened, map[string]any{"text": "Once more."})
	if task := FromEvents(log); !task.DoneAt.IsZero() {
		t.Errorf("reopened: done at %v", task.DoneAt)
	}
	moved := add(you, events.TaskDone, map[string]any{"summary": "Moved.", "moved": true})
	if task := FromEvents(log); !task.DoneAt.Equal(moved) {
		t.Errorf("moved to Done by you: done at %v, want %v", task.DoneAt, moved)
	}
}

// The rules refuse these now, but a log written before them can hold them,
// and replaying it must not fail: a second task.done moves DoneAt on, and a
// task.reopened of an open task leaves it zero.
func TestDoneAtTakesWhatAnOldLogHolds(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	at := func(minute int) time.Time { return time.Date(2026, 10, 19, 9, minute, 0, 0, time.UTC) }
	event := func(minute int, actor events.Actor, kind events.Type, data map[string]any) events.Event {
		return events.Event{Position: int64(minute + 1), At: at(minute), Proposal: events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Data: data}}
	}
	created := event(0, you, events.TaskCreated, map[string]any{"title": "Old", "coordinator": "tech-lead"})

	twice := FromEvents([]events.Event{created,
		event(1, lead, events.TaskDone, map[string]any{"summary": "Done."}),
		event(2, lead, events.TaskDone, map[string]any{"summary": "Done again."})})
	if !twice.DoneAt.Equal(at(2)) {
		t.Errorf("done twice: done at %v, want the second", twice.DoneAt)
	}
	reopenedOpen := FromEvents([]events.Event{created, event(1, you, events.TaskReopened, map[string]any{"text": "Again?"})})
	if !reopenedOpen.DoneAt.IsZero() || reopenedOpen.IsDone {
		t.Errorf("an open task reopened: done %v at %v", reopenedOpen.IsDone, reopenedOpen.DoneAt)
	}
}

// UnarchivedAt is when the task was last unarchived. It leaves DoneAt as
// it was, and a label after it moves neither.
func TestUnarchivedAtIsWhenTheTaskWasLastUnarchived(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	archiver := events.Actor{Kind: events.System, ID: "archiver"}
	start := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	var log []events.Event
	add := func(actor events.Actor, kind events.Type, data map[string]any) time.Time {
		at := start.Add(time.Duration(len(log)) * time.Minute)
		log = append(log, events.Event{Position: int64(len(log) + 1), At: at,
			Proposal: events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Data: data}})
		return at
	}
	add(you, events.TaskCreated, map[string]any{"title": "Ship it", "coordinator": "tech-lead"})
	done := add(you, events.TaskDone, map[string]any{"summary": "Moved.", "moved": true})
	if task := FromEvents(log); !task.UnarchivedAt.IsZero() {
		t.Errorf("never archived: unarchived at %v", task.UnarchivedAt)
	}
	add(archiver, events.TaskArchived, map[string]any{})
	unarchived := add(archiver, events.TaskUnarchived, map[string]any{"marker": 3})
	add(you, events.TaskLabeled, map[string]any{"labels": []any{"web"}})
	task := FromEvents(log)
	if !task.UnarchivedAt.Equal(unarchived) || !task.DoneAt.Equal(done) {
		t.Errorf("unarchived at %v and done at %v, want %v and %v", task.UnarchivedAt, task.DoneAt, unarchived, done)
	}
}
