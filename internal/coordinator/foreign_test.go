package coordinator

import (
	"bytes"
	"log/slog"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// ideasLog writes a log as a build with ideas would: two tasks and two
// ideas, interleaved. It has no order check, as the log is only read back.
// The tasks are parked, so a loop taking the log leaves them be.
func ideasLog(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, err := store.Open(path, storetest.WithExtensions(), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	suggested := func(stream string) events.Proposal {
		return events.Proposal{Stream: stream, Actor: agentry, Type: ideas.IdeaSuggested,
			Data: map[string]any{"title": "Faster", "problem": "Slow", "proposal": "Cache", "size": "S", "repo": "/r", "task": "task-1"}}
	}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "One", "repo": "/r", "coordinator": "tech-lead", "parked": "backlog"}},
		suggested("idea-1"),
		{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Two", "repo": "/r", "coordinator": "tech-lead", "parked": "backlog"}},
		{Stream: "idea-1", Actor: you, Type: ideas.IdeaDismissed, Data: map[string]any{"note": "No"}},
		suggested("idea-2"),
		{Stream: "task-1", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "One, renamed"}},
	} {
		propose(t, s, p)
	}
	return path
}

// A build without ideas replays a log with idea streams to the same tasks,
// counts the idea streams, never hands them to the loop as tasks, and lets
// nothing more into them.
func TestABuildWithoutIdeasSkipsTheirStreams(t *testing.T) {
	path := ideasLog(t)
	with, withState := storetest.OpenWith(t, path, storetest.WithExtensions())
	wantTasks := idsOf(withState.Tasks())
	withLog, err := with.After(0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if got := touchedTasks(with.Catalogue(), withLog); !slices.Equal(got, []string{"task-1", "task-2"}) {
		t.Errorf("with ideas, touched %v", got)
	}

	s, state := storetest.Open(t, path)
	if got := idsOf(state.Tasks()); !slices.Equal(got, wantTasks) || len(got) != 2 {
		t.Errorf("tasks %v, with ideas %v", got, wantTasks)
	}
	for _, id := range wantTasks {
		task, _ := state.Task(id)
		withTask, _ := withState.Task(id)
		if !reflect.DeepEqual(task, withTask) {
			t.Errorf("%s differs from the build with ideas: %+v", id, task)
		}
	}
	want := []current.Unknown{{Prefix: ideas.Prefix, Streams: 2, Events: 3, Last: 5}}
	if got := state.Unknown(); !reflect.DeepEqual(got, want) {
		t.Errorf("unknown %+v, want %+v", got, want)
	}
	log, err := s.After(0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if got := touchedTasks(s.Catalogue(), log); !slices.Equal(got, []string{"task-1", "task-2"}) {
		t.Errorf("touched %v, want only the tasks", got)
	}

	for _, p := range []events.Proposal{
		{Stream: "idea-3", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Three", "repo": "/r", "coordinator": "tech-lead"}},
		{Stream: "idea-2", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Renamed"}},
	} {
		stored, rejection, err := s.Propose(p)
		if err != nil {
			t.Fatal(err)
		}
		if reason := events.ForeignReason(p.Stream); rejection == nil || rejection.Reason != reason || stored.Type != events.ProposalRejected {
			t.Errorf("%s into %s: %v, want refused: %s", p.Type, p.Stream, rejection, reason)
		}
	}
	for _, stream := range []string{"idea-2", "idea-3"} {
		if accepted, _ := s.Stream(stream); len(accepted) != map[string]int{"idea-2": 1, "idea-3": 0}[stream] {
			t.Errorf("%s holds %d events after the refusals", stream, len(accepted))
		}
	}
}

// The loop, taking that log on a build without ideas, never reads an idea
// stream as a task: it says nothing about reading one.
func TestALoopWithoutIdeasNeverReadsTheirStreamsAsTasks(t *testing.T) {
	s, state := storetest.Open(t, ideasLog(t))
	log, err := s.After(0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	said := logged(t)
	newLoop(s, state, t.TempDir()).take(log)
	if strings.Contains(said.String(), "reading the task") {
		t.Errorf("the loop read a stream as a task:\n%s", said)
	}
}

// logged collects what the code logs until the test ends.
func logged(t *testing.T) *bytes.Buffer {
	t.Helper()
	var said bytes.Buffer
	before := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&said, nil)))
	t.Cleanup(func() { slog.SetDefault(before) })
	return &said
}

func idsOf(list []*tasks.Task) []string {
	var ids []string
	for _, task := range list {
		ids = append(ids, task.ID)
	}
	return ids
}
