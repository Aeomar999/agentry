package current

import (
	"encoding/json"
	"reflect"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// The state built from a real store, with the times and JSON values the log
// reads back, comes back whole through a snapshot's saved forms: every task
// and every roster, written as JSON and restored, is what it was.
func TestTheStoredStateComesBackFromItsSavedForms(t *testing.T) {
	s := openStore(t, append(someProposals(),
		proposal("task-4", you, events.TaskCreated, map[string]any{"title": "Done", "repo": "/r", "coordinator": "tech-lead"}),
		proposal("task-4", you, events.TaskDone, map[string]any{"summary": "Moved.", "moved": true}))...)
	state := New(withIdeas, ideas.Streams())
	state.Loaded(0)
	fromTheLog(t, s, state)
	if done := state.tasks["task-4"]; done == nil || done.DoneAt.IsZero() {
		t.Fatal("the log built no done task")
	}
	for id, task := range state.tasks {
		var read tasks.Saved
		throughJSON(t, task.Save(), &read)
		if restored := tasks.Restore(read); !reflect.DeepEqual(task, restored) {
			t.Errorf("%s read back differs:\n%+v\n%+v", id, *task, *restored)
		}
	}
	var roster agents.Saved
	throughJSON(t, state.roster.Save(), &roster)
	if restored := agents.Restore(roster); !reflect.DeepEqual(state.roster, restored) {
		t.Errorf("the agents read back differ:\n%+v\n%+v", state.roster, restored)
	}
	var fleet workers.Saved
	throughJSON(t, state.workers.Save(), &fleet)
	if restored := workers.Restore(fleet); !reflect.DeepEqual(state.workers, restored) {
		t.Errorf("the workers read back differ:\n%+v\n%+v", state.workers, restored)
	}
	var linked connections.Saved
	throughJSON(t, state.connections.Save(), &linked)
	if restored := connections.Restore(linked); !reflect.DeepEqual(state.connections, restored) {
		t.Errorf("the connections read back differ:\n%+v\n%+v", state.connections, restored)
	}
	var templates agents.UserTemplates
	throughJSON(t, state.templates, &templates)
	if len(state.templates) == 0 || !reflect.DeepEqual(state.templates, templates) {
		t.Errorf("the templates read back differ:\n%+v\n%+v", state.templates, templates)
	}
}

func throughJSON(t *testing.T, saved, read any) {
	t.Helper()
	written, err := json.Marshal(saved)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(written, read); err != nil {
		t.Fatal(err)
	}
}
