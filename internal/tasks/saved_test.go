package tasks

import (
	"encoding/json"
	"reflect"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/deeptest"
)

// Every field, exported or not, is filled by reflection, and the saved form
// goes through JSON as a snapshot will write it: a field added to Task that
// Save and Restore don't carry comes back empty, and the test fails.
func TestASavedTaskComesBackWhole(t *testing.T) {
	var task Task
	deeptest.Fill(t, reflect.ValueOf(&task).Elem())
	written, err := json.Marshal(task.Save())
	if err != nil {
		t.Fatal(err)
	}
	var read Saved
	if err := json.Unmarshal(written, &read); err != nil {
		t.Fatal(err)
	}
	if restored := Restore(read); !reflect.DeepEqual(&task, restored) {
		t.Errorf("the task read back differs:\n%+v\n%+v", task, *restored)
	}
}

func TestASavedTaskSharesNothing(t *testing.T) {
	var task Task
	deeptest.Fill(t, reflect.ValueOf(&task).Elem())
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
// apply leaves them, comes back the same.
func TestAReplayedTaskComesBackWhole(t *testing.T) {
	log := everyEvent()
	for i := range log {
		task := FromEvents(log[:i+1])
		written, err := json.Marshal(task.Save())
		if err != nil {
			t.Fatal(err)
		}
		var read Saved
		if err := json.Unmarshal(written, &read); err != nil {
			t.Fatal(err)
		}
		if restored := Restore(read); !reflect.DeepEqual(task, restored) {
			t.Errorf("after event %d, %s, the task read back differs:\n%+v\n%+v", i+1, log[i].Type, task, restored)
		}
	}
}
