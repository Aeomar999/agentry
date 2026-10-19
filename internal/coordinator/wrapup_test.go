package coordinator

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// You move a task to Done while the engineer works: it is done at once, and
// the coordinator gets exactly one turn to wrap up, whose summary is kept.
// Nothing runs after it.
func TestYouFinishATaskAndTheCoordinatorWrapsUpOnce(t *testing.T) {
	s, id := taskWithTheEngineerWorking(t, "task-wrap-1")
	propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskDone,
		Data: map[string]any{"summary": "Moved.", "note": "Good enough for now.", "moved": true}})

	task := waitForTurn(t, s, id, "turn-3", "tech-lead")
	if !task.IsDone || !task.Turns[1].Stopped {
		t.Fatalf("done %v, the engineer's turn stopped %v", task.IsDone, task.Turns[1].Stopped)
	}
	brief := readBrief(t, task, "turn-3")
	for _, want := range []string{"I moved this task to Done: it is finished as it is. Good enough for now.", "## Wrap up", "it stays done", WrapUpFormat} {
		if !strings.Contains(brief, want) {
			t.Errorf("the wrap-up brief is missing %q", want)
		}
	}
	if strings.Contains(brief, `"action": "assign"`) {
		t.Error("the wrap-up brief offers new work")
	}

	endTurn(t, s, id, "turn-3", "tech-lead", `{"action": "done", "summary": "Half the TODOs are gone; the rest are in src/b.go."}`)
	task = waitUntil(t, s, id, "the wrap-up to end", func(task *tasks.Task) bool { return task.WrapUp == nil })
	if task.Summary != "Half the TODOs are gone; the rest are in src/b.go." {
		t.Errorf("summary %q", task.Summary)
	}
	nothingMoreRuns(t, s, id, 3)
}

// A wrap-up that decides anything but its summary starts nothing: the
// rules refuse it, and the task stays done.
func TestAWrapUpCantStartMoreWork(t *testing.T) {
	s, id := taskWithTheEngineerWorking(t, "task-wrap-2")
	propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "Moved.", "moved": true}})
	waitForTurn(t, s, id, "turn-3", "tech-lead")
	endTurn(t, s, id, "turn-3", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Finish src/b.go."}`)

	task := waitUntil(t, s, id, "the wrap-up to end", func(task *tasks.Task) bool { return task.WrapUp == nil })
	if !task.IsDone || task.Summary != "Moved." || !strings.Contains(task.Told.Text, `decided "assign"`) {
		t.Errorf("done %v, summary %q, told %q", task.IsDone, task.Summary, task.Told.Text)
	}
	_, rejection, _ := s.Propose(events.Proposal{Stream: id, Actor: coordinator, Type: events.TaskAssigned,
		Data: map[string]any{"agent": "engineer", "instructions": "Finish src/b.go."}})
	if rejection == nil || rejection.Reason != "the user moved this task to Done; it stays done" {
		t.Errorf("an assignment after your Done: %v", rejection)
	}
	nothingMoreRuns(t, s, id, 3)
}

// taskWithTheEngineerWorking runs the loop on a task until the engineer's
// turn (turn-2) is open.
func taskWithTheEngineerWorking(t *testing.T, id string) (*store.Store, string) {
	fakeClaudeOnPath(t)
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), rules.Check)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	t.Cleanup(func() { sessions.Stop(sessions.SocketFor(id)) })
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go newLoop(s, t.TempDir()).Run(ctx)

	propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Remove the TODOs", "repo": gitRepo(t), "coordinator": "tech-lead"}})
	waitForTurn(t, s, id, "turn-1", "tech-lead")
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Remove every TODO."}`)
	waitForTurn(t, s, id, "turn-2", "engineer")
	return s, id
}

// nothingMoreRuns wakes the loop for the task, and checks no turn starts.
func nothingMoreRuns(t *testing.T, s *store.Store, id string, turns int) {
	t.Helper()
	propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskLabeled, Data: map[string]any{"labels": []string{"wake"}}})
	time.Sleep(300 * time.Millisecond)
	if task := load(t, s, id); len(task.Turns) != turns || task.OpenTurn() != nil {
		t.Errorf("%d turns after the wrap-up, want %d", len(task.Turns), turns)
	}
}

func propose(t *testing.T, s *store.Store, p events.Proposal) {
	t.Helper()
	if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
		t.Fatal(p.Type, rejection, err)
	}
}
