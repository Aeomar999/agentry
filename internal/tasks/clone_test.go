package tasks

import (
	"reflect"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/tasks/taskstest"
	"gitlab.com/dev.chempa/agentry/internal/testkit/deeptest"
)

// Every field of a fully populated task is filled by reflection, so a
// field added to Task later is checked without anyone adding it here.
func TestACloneSharesNothing(t *testing.T) {
	task := filled(t)
	clone := task.Clone()
	if !reflect.DeepEqual(&task, clone) {
		t.Fatalf("the clone differs from the task:\n%+v\n%+v", task, *clone)
	}
	deeptest.Shared(t, "Task", reflect.ValueOf(task), reflect.ValueOf(*clone))
}

// Next must leave the task it is given as it was, and folding it over a
// log must give what FromEvents gives, after every event.
func TestNextLeavesTheTaskAsItWas(t *testing.T) {
	log := taskstest.EveryEvent()
	var task *Task
	for i, e := range log {
		next := Next(task, e)
		// DeepEqual can't see sharing beyond a slice's length; that is TestACloneSharesNothing's job.
		if before := FromEvents(log[:i]); !reflect.DeepEqual(task, before) {
			t.Errorf("event %d, %s, changed the task it was applied to", i+1, e.Type)
		}
		if want := FromEvents(log[:i+1]); !reflect.DeepEqual(next, want) {
			t.Errorf("after event %d, %s, Next differs from FromEvents:\n%+v\n%+v", i+1, e.Type, next, want)
		}
		task = next
	}
	if task == nil || !task.IsDone || !task.DoneByYou || task.WrapUp == nil || len(task.Turns) != 6 || len(task.Actions) != 4 {
		t.Errorf("the log didn't run through: %+v", task)
	}
}

// filled is a task with every field filled by reflection but actionAt,
// which is made from the filled Actions: it is derived from them, and an
// index filled at random would point at no action in particular.
func filled(t *testing.T) Task {
	var task Task
	deeptest.Fill(t, reflect.ValueOf(&task).Elem())
	task.Actions[1].ID = "y"
	task.actionAt = indexActions(task.Actions)
	return task
}

func TestNextBeforeTheTaskIsCreated(t *testing.T) {
	if task := Next(nil, taskstest.EveryEvent()[1]); task != nil {
		t.Errorf("a task out of an event before task.created: %+v", task)
	}
}
