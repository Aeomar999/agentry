package rules

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
)

var redactor = events.Actor{Kind: events.System, ID: "redactor"}

func redactedData(change func(map[string]any)) map[string]any {
	data := map[string]any{"positions": []int64{6, 8}, "fields": [][]string{{"$.title"}, {"$.text"}}, "removed": 18, "who": "human:you", "why": ""}
	if change != nil {
		change(data)
	}
	return data
}

// A redaction goes onto any task the log has, open or done, and onto the
// streams that aren't tasks, saying which rows and fields it changed;
// not onto a task the log hasn't, an archived task or an erased one. Only
// the store writes it, so the rules' own decisions are read from Check.
// (Store.Redact checks its rows, and storetest's redact_test.go the rest.)
func TestARedactionNamesTheRowsAndFieldsItChanged(t *testing.T) {
	s, state := openSeededState(t)
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: created.data},
		{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: created.data},
		{Stream: "task-2", Actor: coordinator, Type: events.TaskDone, Data: map[string]any{"summary": "Done."}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	redacting := func(stream string, data map[string]any) *events.Rejection {
		return Check(events.Proposal{Stream: stream, Actor: redactor, Type: events.EventRedacted, Data: data}, state, withIdeas)
	}
	for _, c := range []struct {
		data map[string]any
		want string
	}{
		{redactedData(func(d map[string]any) { d["positions"] = []int64{} }), "a redaction names the rows it rewrote, by position"},
		{redactedData(func(d map[string]any) { d["positions"] = []any{"6"} }), "a redaction names the rows it rewrote, by position"},
		{redactedData(func(d map[string]any) { d["positions"] = []int64{0, 8} }), "a redaction names each row once, in the log's order"},
		{redactedData(func(d map[string]any) { d["positions"] = []int64{8, 6} }), "a redaction names each row once, in the log's order"},
		{redactedData(func(d map[string]any) { d["positions"] = []int64{6, 6} }), "a redaction names each row once, in the log's order"},
		{redactedData(func(d map[string]any) { d["fields"] = [][]string{{"$.title"}} }), "a redaction names, for each row, the fields it changed"},
		{redactedData(func(d map[string]any) { d["fields"] = [][]string{{"$.title"}, {}} }), "a redaction names, for each row, the fields it changed"},
		{redactedData(func(d map[string]any) { d["fields"] = []any{[]any{"$.title"}, []any{7.0}} }), "a redaction names, for each row, the fields it changed"},
		{redactedData(func(d map[string]any) { d["removed"] = 0 }), "a redaction says how many bytes it removed"},
		{redactedData(func(d map[string]any) { d["who"] = "" }), "a redaction says who asked for it"},
	} {
		if rejection := redacting("task-1", c.data); rejection == nil || rejection.Reason != c.want {
			t.Errorf("redacting with %v: %v, want %q", c.data, rejection, c.want)
		}
	}
	if rejection := redacting("task-9", redactedData(nil)); rejection == nil || rejection.Reason != "task task-9 doesn't exist" {
		t.Errorf("redacting a task that doesn't exist: %v", rejection)
	}
	for _, stream := range []string{"task-1", "task-2", events.AgentsStream} {
		if rejection := redacting(stream, redactedData(nil)); rejection != nil {
			t.Errorf("redacting %s: %v", stream, rejection)
		}
	}
	// As read back from the log, every list decoded as []any of float64s.
	read := redactedData(func(d map[string]any) {
		d["positions"], d["fields"] = []any{6.0, 8.0}, []any{[]any{"$.title"}, []any{"$.text"}}
	})
	if rejection := redacting("task-1", read); rejection != nil {
		t.Errorf("redacting with the data as the log reads it: %v", rejection)
	}

	_, archived := openArchived(t)
	if rejection := Check(events.Proposal{Stream: "task-1", Actor: redactor, Type: events.EventRedacted, Data: redactedData(nil)}, archived, withIdeas); rejection == nil || rejection.Reason != ArchivedRedactionRefusal("task-1") {
		t.Errorf("redacting an archived task: %v, want %q", rejection, ArchivedRedactionRefusal("task-1"))
	}
	if _, rejection, err := s.Erase("task-1", you, "Asked to be forgotten.", sharedfiles.Disk{Dir: t.TempDir()}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if rejection := redacting("task-1", redactedData(nil)); rejection == nil || rejection.Reason != ErasedRefusal("task-1") {
		t.Errorf("redacting an erased task: %v, want %q", rejection, ErasedRefusal("task-1"))
	}
}

// Only the store writes event.redacted: no actor gets it in through
// Propose, and the catalogue refuses any but Agentry as its writer.
func TestOnlyTheStoreWritesARedaction(t *testing.T) {
	s, _ := openSeededState(t)
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: created.data}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	for _, actor := range []events.Actor{you, coordinator, engineer, worker, policy, redactor} {
		_, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: actor, Type: events.EventRedacted, Data: redactedData(nil)})
		if err != nil || rejection == nil || rejection.Check != "who" {
			t.Errorf("event.redacted by %s: %v, %v, want refused by the who check", actor, rejection, err)
		}
	}
	for _, actor := range []events.Actor{you, coordinator, worker, policy} {
		if rejection := withIdeas.CheckWriter(events.Proposal{Stream: "task-1", Actor: actor, Type: events.EventRedacted}); rejection == nil {
			t.Errorf("%s may write event.redacted", actor)
		}
	}
}
