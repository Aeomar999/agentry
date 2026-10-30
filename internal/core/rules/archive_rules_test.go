package rules

import (
	"errors"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

var archiver = events.Actor{Kind: events.System, ID: "archiver"}

const blob = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func archivedData(change func(map[string]any)) map[string]any {
	data := map[string]any{"through": 9, "count": 3, "archive": blob, "created": 6, "summary": map[string]any{"title": "Remove the TODOs"}}
	if change != nil {
		change(data)
	}
	return data
}

func unarchivedData(change func(map[string]any)) map[string]any {
	data := map[string]any{"marker": 10, "archive": blob}
	if change != nil {
		change(data)
	}
	return data
}

// openArchived is a store with task-1 made, done and archived by its
// marker, and the state that holds it as its summary.
func openArchived(t *testing.T) (*store.Store, *current.State) {
	t.Helper()
	s, state := openSeededState(t)
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: created.data},
		{Stream: "task-1", Actor: coordinator, Type: events.TaskDone, Data: map[string]any{"summary": "Gone."}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	archive(t, s, state, "task-1")
	return s, state
}

// archive archives a done task through the store, as the archiver does.
func archive(t *testing.T, s *store.Store, state *current.State, id string) {
	t.Helper()
	task, err := state.Task(id)
	if err != nil {
		t.Fatal(err)
	}
	summary := tasks.SummaryOf(task, state.Roster())
	if _, rejection, err := s.Archive(id, func() any { return summary }, sharedfiles.Disk{Dir: t.TempDir()}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
}

// An archived task takes its unarchiving and nothing else. Only the store
// writes the markers, so the rules' own refusals of them are read from
// Check. (Store.Unarchive takes it on from there.)
func TestAnArchivedTaskTakesOnlyItsUnarchiving(t *testing.T) {
	s, state := openArchived(t)
	if rejection := Check(events.Proposal{Stream: "task-1", Actor: archiver, Type: events.TaskArchived, Data: archivedData(nil)}, state, withIdeas); rejection == nil ||
		rejection.Reason != "task-1 is archived: an archived task takes no events" {
		t.Errorf("archiving an archived task: %v", rejection)
	}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskReopened, Data: map[string]any{"text": "More."}},
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: created.data},
	} {
		if _, rejection, err := s.Propose(p); err != nil || rejection == nil || rejection.Reason != "task-1 is archived: an archived task takes no events" {
			t.Errorf("%s on an archived task: %v, %v", p.Type, rejection, err)
		}
	}
	for _, c := range []struct {
		data map[string]any
		want string
	}{
		{unarchivedData(func(d map[string]any) { d["archive"] = "abc" }), "not an archive's hash"},
		{unarchivedData(func(d map[string]any) { d["marker"] = 0 }), "an unarchiving names the position of the marker it undoes"},
	} {
		p := events.Proposal{Stream: "task-1", Actor: archiver, Type: events.TaskUnarchived, Data: c.data}
		if rejection := Check(p, state, withIdeas); rejection == nil || rejection.Reason != c.want {
			t.Errorf("unarchiving with %v: %v, want %q", c.data, rejection, c.want)
		}
	}
	marker, err := s.Stream("task-1")
	if err != nil {
		t.Fatal(err)
	}
	unarchiving := unarchivedData(func(d map[string]any) { d["marker"], d["archive"] = marker[0].Position, marker[0].Text("archive") })
	if rejection := Check(events.Proposal{Stream: "task-1", Actor: archiver, Type: events.TaskUnarchived, Data: unarchiving}, state, withIdeas); rejection != nil {
		t.Errorf("unarchiving: %v", rejection)
	}
}

// Only a done task is archived, with a marker that says where its rows
// went; a whole task has nothing to unarchive.
func TestOnlyADoneTaskIsArchived(t *testing.T) {
	s, state := openSeededState(t)
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: created.data}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	propose := func(kind events.Type, data map[string]any) *events.Rejection {
		return Check(events.Proposal{Stream: "task-1", Actor: archiver, Type: kind, Data: data}, state, withIdeas)
	}
	if rejection := propose(events.TaskArchived, archivedData(nil)); rejection == nil || rejection.Reason != "the task isn't done, so it can't be archived" {
		t.Errorf("archiving an open task: %v", rejection)
	}
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: coordinator, Type: events.TaskDone, Data: map[string]any{"summary": "Gone."}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	for _, c := range []struct {
		data map[string]any
		want string
	}{
		{archivedData(func(d map[string]any) { d["archive"] = "" }), "not an archive's hash"},
		{archivedData(func(d map[string]any) { d["created"] = 0 }), `an archive covers the task's rows from its task.created to its "through"`},
		{archivedData(func(d map[string]any) { d["through"] = 5 }), `an archive covers the task's rows from its task.created to its "through"`},
		{archivedData(func(d map[string]any) { d["count"] = 0 }), "an archive holds the task's rows, its task.created at least"},
	} {
		if rejection := propose(events.TaskArchived, c.data); rejection == nil || rejection.Reason != c.want {
			t.Errorf("archiving with %v: %v, want %q", c.data, rejection, c.want)
		}
	}
	if rejection := propose(events.TaskUnarchived, unarchivedData(nil)); rejection == nil || rejection.Reason != "task task-1 isn't archived" {
		t.Errorf("unarchiving a whole task: %v", rejection)
	}
	if rejection := propose(events.TaskArchived, archivedData(nil)); rejection != nil {
		t.Errorf("archiving the done task: %v", rejection)
	}
	archive(t, s, state, "task-1")
	if task, err := state.Task("task-1"); task != nil || !errors.Is(err, current.ErrArchived) {
		t.Errorf("an accepted marker archives the task: %v, %v", task, err)
	}
}

// A marker keeps a summary the state can read: anything else would break
// the task's stream for good, at the marker.
func TestAMarkerKeepsASummaryTheStateCanRead(t *testing.T) {
	s, state := openSeededState(t)
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: created.data},
		{Stream: "task-1", Actor: coordinator, Type: events.TaskDone, Data: map[string]any{"summary": "Gone."}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	archiving := func(summary any) *events.Rejection {
		return Check(events.Proposal{Stream: "task-1", Actor: archiver, Type: events.TaskArchived,
			Data: archivedData(func(d map[string]any) { d["summary"] = summary })}, state, withIdeas)
	}
	for _, summary := range []any{"Remove the TODOs", 7, nil, map[string]any{"title": 5}, map[string]any{"labels": "web"}, []any{}} {
		if rejection := archiving(summary); rejection == nil || !strings.HasPrefix(rejection.Reason, "an archive keeps the task's summary: ") {
			t.Errorf("a marker with the summary %#v: %v, want refused", summary, rejection)
		}
	}
	task, _ := state.Task("task-1")
	if rejection := archiving(tasks.SummaryOf(task, state.Roster())); rejection != nil {
		t.Errorf("a marker with the task's summary: %v", rejection)
	}
}

// Only the store writes the markers: no agent, no worker, not the page,
// not even the archiver's own actor through Propose.
func TestOnlyTheStoreWritesTheMarkers(t *testing.T) {
	s, _ := openArchived(t)
	for _, actor := range []events.Actor{you, coordinator, engineer, worker, policy, archiver} {
		for kind, data := range map[events.Type]map[string]any{events.TaskArchived: archivedData(nil), events.TaskUnarchived: unarchivedData(nil)} {
			_, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Data: data})
			if err != nil || rejection == nil || rejection.Check != "who" {
				t.Errorf("%s by %s: %v, %v, want refused by the who check", kind, actor, rejection, err)
			}
		}
	}
}

// A done task whose turn is still open, as the wrap-up turn after you move
// it to Done (which stops the turn in hand), isn't archived until the turn
// ends: by the rules, and so by the store, which checks the marker under
// its lock.
func TestATaskWithATurnRunningWaitsForItToEnd(t *testing.T) {
	s, state := openSeededState(t)
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: created.data},
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: worktree.data},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: turnStarted("turn-1", "tech-lead").data},
		{Stream: "task-1", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "Moved.", "moved": true}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-2", Data: turnStarted("turn-2", "tech-lead").data},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	const waits = "the task has a turn running, so it waits until the turn ends"
	if rejection := Check(events.Proposal{Stream: "task-1", Actor: archiver, Type: events.TaskArchived, Data: archivedData(nil)}, state, withIdeas); rejection == nil || rejection.Reason != waits {
		t.Errorf("the rules on archiving a task with a turn running: %v", rejection)
	}
	task, _ := state.Task("task-1")
	summary := tasks.SummaryOf(task, state.Roster())
	if _, rejection, err := s.Archive("task-1", func() any { return summary }, sharedfiles.Disk{Dir: t.TempDir()}); err != nil || rejection == nil || rejection.Reason != waits {
		t.Errorf("the store archiving a task with a turn running: %v, %v", rejection, err)
	}
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-2", Data: turnEnded("turn-2", "tech-lead", "Wrapped up.").data}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	archive(t, s, state, "task-1")
}
