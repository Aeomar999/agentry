package tasks

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks/taskstest"
	"gitlab.com/dev.chempa/agentry/internal/testkit/deeptest"
)

// Action looks actions up by ID, so every copy of a task needs its own way
// to them: before every action.requested of the log, the task is copied
// each way state copies it, and the copy takes the event. Every action,
// the new one too, is then found in the copy's own Actions.
func TestAnActionIsFoundInEveryCopy(t *testing.T) {
	copies := map[string]func(*Task) *Task{
		"Clone":   (*Task).Clone,
		"Restore": func(task *Task) *Task { return Restore(task.Save()) },
	}
	for _, encoding := range deeptest.Encodings {
		copies["Restore through "+encoding.Name] = func(task *Task) *Task { return through(t, encoding, task) }
	}
	log := taskstest.EveryEvent()
	for i, e := range log {
		if e.Type != events.ActionRequested {
			continue
		}
		for name, copied := range copies {
			original := FromEvents(log[:i])
			task := copied(original)
			task.Apply(e)
			if len(task.Actions) == len(original.Actions) {
				t.Fatalf("%s, then %s: no action added", name, e.Correlation)
			}
			for j, action := range task.Actions {
				if found := task.Action(action.ID); found != action {
					t.Errorf("%s, then %s: Action(%q) is %p, want the copy's %p", name, e.Correlation, action.ID, found, action)
				}
				if j < len(original.Actions) && original.Action(action.ID) != original.Actions[j] {
					t.Errorf("%s, then %s: the original lost %s", name, e.Correlation, action.ID)
				}
			}
			if original.Action(e.Correlation) != nil {
				t.Errorf("%s, then %s: the original has the copy's new action", name, e.Correlation)
			}
		}
	}
}

// A log with two actions of one ID, which no rule refuses on an old log,
// finds the first, as a scan of Actions did, and so does every copy.
func TestADuplicateActionIDFindsTheFirst(t *testing.T) {
	log := taskstest.EveryEvent()
	first := indexOf(t, log, events.ActionRequested, "tool-1")
	again := log[first]
	again.Data = map[string]any{"tool": "Bash", "command": "go vet"}
	log = append(log[:first+1:first+1], append([]events.Event{again}, log[first+1:]...)...)

	task := FromEvents(log)
	for name, task := range map[string]*Task{"fold": task, "Clone": task.Clone(), "Restore": Restore(task.Save())} {
		if action := task.Action("tool-1"); action == nil || action != scan(task, "tool-1") || action.Command != "go test" {
			t.Errorf("%s: Action(tool-1) is %+v, want the first, go test", name, action)
		}
	}
}

// scan is Action as it was before the index: the first action with the ID.
func scan(t *Task, id string) *Action {
	for _, action := range t.Actions {
		if action.ID == id {
			return action
		}
	}
	return nil
}

func indexOf(t *testing.T, log []events.Event, kind events.Type, correlation string) int {
	t.Helper()
	for i, e := range log {
		if e.Type == kind && e.Correlation == correlation {
			return i
		}
	}
	t.Fatalf("no %s %s in the log", kind, correlation)
	return -1
}
