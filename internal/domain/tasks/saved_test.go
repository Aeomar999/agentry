package tasks

import (
	"reflect"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/domain/tasks/taskstest"
	"gitlab.com/dev.chempa/agentry/internal/testkit/deeptest"
)

// Every field, exported or not, is filled by reflection, and the saved form
// goes through JSON and gob as a snapshot writes it: a field added to Task that
// Save and Restore don't carry comes back empty, and the test fails.
func TestASavedTaskComesBackWhole(t *testing.T) {
	task := filled(t)
	for _, encoding := range deeptest.Encodings {
		if restored := through(t, encoding, &task); !reflect.DeepEqual(&task, restored) {
			t.Errorf("the task read back from %s differs:\n%+v\n%+v", encoding.Name, task, *restored)
		}
	}
}

// through saves the task, writes and reads it as encoding does, and restores it.
func through(t *testing.T, encoding deeptest.Encoding, task *Task) *Task {
	t.Helper()
	var read Saved
	if err := encoding.RoundTrip(task.Save(), &read); err != nil {
		t.Fatal(err)
	}
	return Restore(read)
}

func TestASavedTaskSharesNothing(t *testing.T) {
	task := filled(t)
	saved := task.Save()
	deeptest.Shared(t, "Task", reflect.ValueOf(task), reflect.ValueOf(saved.Task))
	deeptest.Shared(t, "collected", reflect.ValueOf(task.collected), reflect.ValueOf(saved.Collected))
	if restored := Restore(saved); !reflect.DeepEqual(&task, restored) {
		t.Errorf("the restored task differs:\n%+v\n%+v", task, *restored)
	}
}

func TestARestoredTaskSharesNothing(t *testing.T) {
	var saved Saved
	deeptest.Fill(t, reflect.ValueOf(&saved).Elem())
	restored := Restore(saved)
	deeptest.Shared(t, "Task", reflect.ValueOf(saved.Task), reflect.ValueOf(*restored))
	deeptest.Shared(t, "Collected", reflect.ValueOf(saved.Collected), reflect.ValueOf(restored.collected))
}

// A task replayed from a log, with its empty lists and unset pointers as
// Apply leaves them, comes back the same: nil where it was nil, empty where
// it was empty.
func TestAReplayedTaskComesBackWhole(t *testing.T) {
	log := taskstest.EveryEvent()
	for i := range log {
		task := FromEvents(log[:i+1])
		for _, encoding := range deeptest.Encodings {
			if restored := through(t, encoding, task); !reflect.DeepEqual(task, restored) {
				t.Errorf("after event %d, %s, the task read back from %s differs:\n%+v\n%+v", i+1, log[i].Type, encoding.Name, task, restored)
			}
		}
	}
}

// A snapshot's task takes the events after it in place: restored at every
// cut of the log and applied the rest, it is the task FromEvents gives.
func TestARestoredTaskTakesTheRestOfItsLog(t *testing.T) {
	log := taskstest.EveryEvent()
	want := FromEvents(log)
	for cut := 1; cut < len(log); cut++ {
		task := Restore(FromEvents(log[:cut]).Save())
		for _, e := range log[cut:] {
			task.Apply(e)
		}
		if !reflect.DeepEqual(task, want) {
			t.Errorf("restored after event %d, then applied the rest, the task differs:\n%+v\n%+v", cut, task, want)
		}
	}
}
