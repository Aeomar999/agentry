package rules

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

func erasedData(change func(map[string]any)) map[string]any {
	data := map[string]any{"why": "Asked to be forgotten.", "through": 9, "count": 3, "created": 6, "archives": []string{blob}}
	if change != nil {
		change(data)
	}
	return data
}

// A task the log has is erased, open or archived, with a marker that says
// which rows go; one the log hasn't isn't. Only the store writes the
// marker, so the rules' own decisions are read from Check. (Store.Erase
// takes it on from there, and erase_test.go in store checks that an
// erased task takes nothing after.)
func TestATaskIsErasedOpenOrArchived(t *testing.T) {
	s, state := openSeededState(t)
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: created.data}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	erasing := func(stream string, data map[string]any) *events.Rejection {
		return Check(events.Proposal{Stream: stream, Actor: you, Type: events.TaskErased, Data: data}, state, withIdeas)
	}
	if rejection := erasing("task-9", erasedData(nil)); rejection == nil || rejection.Reason != "task task-9 doesn't exist" {
		t.Errorf("erasing a task that doesn't exist: %v", rejection)
	}
	for _, c := range []struct {
		data map[string]any
		want string
	}{
		{erasedData(func(d map[string]any) { d["created"] = 0 }), `an erasure covers the task's rows from its task.created to its "through"`},
		{erasedData(func(d map[string]any) { d["through"] = 5 }), `an erasure covers the task's rows from its task.created to its "through"`},
		{erasedData(func(d map[string]any) { d["count"] = 0 }), "an erasure deletes the task's rows, its task.created at least"},
		{erasedData(func(d map[string]any) { d["archives"] = []string{"abc"} }), "not an archive's hash: abc"},
	} {
		if rejection := erasing("task-1", c.data); rejection == nil || rejection.Reason != c.want {
			t.Errorf("erasing with %v: %v, want %q", c.data, rejection, c.want)
		}
	}
	if rejection := erasing("task-1", erasedData(nil)); rejection != nil {
		t.Errorf("erasing an open task: %v", rejection)
	}
	if rejection := erasing("task-1", erasedData(func(d map[string]any) { d["archives"] = []string{} })); rejection != nil {
		t.Errorf("erasing a task never archived: %v", rejection)
	}

	_, archived := openArchived(t)
	if rejection := Check(events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskErased, Data: erasedData(nil)}, archived, withIdeas); rejection != nil {
		t.Errorf("erasing an archived task: %v", rejection)
	}
}

// Only the store writes task.erased, and only for you: no actor gets it in
// through Propose, and the catalogue refuses any but you as its writer.
func TestOnlyTheStoreWritesAnErasure(t *testing.T) {
	s, state := openArchived(t)
	for _, actor := range []events.Actor{you, coordinator, engineer, worker, policy, archiver} {
		_, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: actor, Type: events.TaskErased, Data: erasedData(nil)})
		if err != nil || rejection == nil || rejection.Check != "who" {
			t.Errorf("task.erased by %s: %v, %v, want refused by the who check", actor, rejection, err)
		}
	}
	if rejection := withIdeas.CheckWriter(events.Proposal{Stream: "task-1", Actor: archiver, Type: events.TaskErased}); rejection == nil {
		t.Error("the archiver may write task.erased")
	}
	if _, err := state.Task("task-1"); err == nil {
		t.Error("task-1 should still be archived")
	}
}
