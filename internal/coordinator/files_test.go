package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// Shared files on the real loop: what an agent leaves in .agentry/files is
// kept, a file you share reaches the next agent's folder, and the
// coordinator fetches from a machine: from its worktrees at once, from
// anywhere else once you say so.
func TestSharedFilesFollowTheWork(t *testing.T) {
	fakeClaudeOnPath(t)
	root := t.TempDir()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	repo := gitRepo(t)
	outsideDir := t.TempDir()
	const id = "task-1"
	run(t, newLoop(s, state, root), state)
	blobs := sharedfiles.Disk{Dir: filepath.Join(root, ".files")}

	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Crunch the numbers", "repo": repo, "coordinator": "tech-lead"}})
	task := waitForTurn(t, s, id, "turn-1", "tech-lead")
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Make the report."}`)
	task = waitForTurn(t, s, id, "turn-2", "engineer")
	if brief := readBrief(t, task, "turn-2"); !strings.Contains(brief, "## Shared files") || !strings.Contains(brief, ".agentry/files") {
		t.Errorf("the engineer should hear about the shared folder:\n%s", brief)
	}

	// The engineer leaves a file in the folder: it is kept.
	folder := sharedfiles.Dir(task.Worktree.Path)
	os.MkdirAll(filepath.Join(folder, "out"), 0o755)
	os.WriteFile(filepath.Join(folder, "out", "report.csv"), []byte("total,42\n"), 0o644)
	endTurn(t, s, id, "turn-2", "engineer", "The report is in out/report.csv.")
	task = waitForTurn(t, s, id, "turn-3", "tech-lead")
	if f := task.Files["out/report.csv"]; f.By != "engineer" || f.Size != 9 || !blobs.Has(f.Hash) {
		t.Errorf("the engineer's file should be kept: %+v", f)
	}
	if brief := readBrief(t, task, "turn-3"); !strings.Contains(brief, "- out/report.csv (9 bytes, from engineer)") {
		t.Errorf("the tech lead should see it:\n%s", brief)
	}

	// You share a file: the next agent finds it in its folder.
	hash, size, _ := blobs.Put(strings.NewReader("the spec"))
	if _, rejection, _ := s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.FileAdded,
		Data: map[string]any{"name": "spec.md", "hash": hash, "size": size, "by": "you"}}); rejection != nil {
		t.Fatal(rejection)
	}
	endTurn(t, s, id, "turn-3", "tech-lead", `{"action": "assign", "agent": "tester", "instructions": "Check it against the spec."}`)
	task = waitForTurn(t, s, id, "turn-4", "tester")
	if got, _ := os.ReadFile(filepath.Join(folder, "spec.md")); string(got) != "the spec" {
		t.Errorf("your file should be in the tester's folder: %q", got)
	}

	// The tester removes the report; it is no longer shared.
	os.Remove(filepath.Join(folder, "out", "report.csv"))
	endTurn(t, s, id, "turn-4", "tester", "Removed the stale report.")
	task = waitForTurn(t, s, id, "turn-5", "tech-lead")
	if _, kept := task.Files["out/report.csv"]; kept || task.Files["spec.md"].Hash == "" {
		t.Errorf("files after the tester: %v", task.Files)
	}

	// From inside this machine's worktrees, a fetch comes at once.
	other := filepath.Join(root, "task-other")
	os.MkdirAll(other, 0o755)
	os.WriteFile(filepath.Join(other, "data.bin"), []byte("bytes"), 0o644)
	endTurn(t, s, id, "turn-5", "tech-lead", `{"action": "fetch", "machine": "local", "path": "`+filepath.ToSlash(filepath.Join(other, "data.bin"))+`"}`)
	task = waitForTurn(t, s, id, "turn-6", "tech-lead")
	if f := task.Files["data.bin"]; f.From != "local:"+filepath.ToSlash(filepath.Join(other, "data.bin")) {
		t.Errorf("the fetched file: %+v", f)
	}
	if brief := readBrief(t, task, "turn-6"); !strings.Contains(brief, "Copied local:") {
		t.Errorf("the tech lead should hear it came:\n%s", brief)
	}

	// From anywhere else, you are asked.
	outside := filepath.Join(outsideDir, "private.txt")
	os.WriteFile(outside, []byte("mine"), 0o644)
	endTurn(t, s, id, "turn-6", "tech-lead", `{"action": "fetch", "machine": "local", "path": "`+filepath.ToSlash(outside)+`", "as": "notes.txt"}`)
	task = waitUntil(t, s, id, "the question", func(task *tasks.Task) bool { return task.Asked.Fetch != nil })
	if !strings.Contains(task.Question, "outside that machine's worktrees") || task.Asked.Options[0] != CopyFile || task.Asked.Fetch.As != "notes.txt" {
		t.Errorf("the question: %q %+v", task.Question, task.Asked)
	}
	if _, ok := task.Files["notes.txt"]; ok {
		t.Error("it was copied before you said so")
	}
}

// Work beside the loop starts only from the task as it is now: a pass that
// read it before an event changed it leaves the work to the pass that
// event wakes. A stale pass would collect a turn twice, the second refused,
// and keep the task busy as the real next step came and was dropped.
func TestWorkBesideStartsOnlyFromTheTaskAsItIsNow(t *testing.T) {
	s, state, l := besideLoop(t)
	stale, _ := state.Task(besideTask)
	if _, rejection, _ := s.Propose(shared(besideTask, "a.txt")); rejection != nil {
		t.Fatal(rejection)
	}
	now, _ := state.Task(besideTask)

	started := make(chan string, 2)
	release := make(chan struct{})
	defer close(release)
	l.beside(stale, func(context.Context) events.Proposal {
		started <- "stale"
		<-release
		return shared(besideTask, "stale.txt")
	})
	l.beside(now, func(context.Context) events.Proposal {
		started <- "now"
		return shared(besideTask, "b.txt")
	})
	if got := <-started; got != "now" {
		t.Fatalf("the %s pass's work started", got)
	}
	waitUntil(t, s, besideTask, "b.txt", func(task *tasks.Task) bool { return task.Files["b.txt"].Hash != "" })
}

// Work's last event is in before the task is free again, so the pass that
// event wakes can start the next work: none waits on an event to come.
func TestThePassWorksLastEventWakesFindsTheTaskFree(t *testing.T) {
	s, state, l := besideLoop(t)
	task, _ := state.Task(besideTask)
	l.beside(task, func(context.Context) events.Proposal { return shared(besideTask, "a.txt") })
	waitUntil(t, s, besideTask, "a.txt", func(task *tasks.Task) bool { return task.Files["a.txt"].Hash != "" })

	task, _ = state.Task(besideTask)
	next := make(chan struct{})
	l.beside(task, func(context.Context) events.Proposal {
		close(next)
		return shared(besideTask, "b.txt")
	})
	select {
	case <-next:
	case <-time.After(10 * time.Second):
		t.Fatal("the next work was dropped: the task was still busy")
	}
	waitUntil(t, s, besideTask, "b.txt", func(task *tasks.Task) bool { return task.Files["b.txt"].Hash != "" })
}

const besideTask = "task-1"

// besideLoop is a loop that isn't running, and a task for its work beside.
func besideLoop(t *testing.T) (*store.Store, *current.State, *Loop) {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	if _, rejection, _ := s.Propose(events.Proposal{Stream: besideTask, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Crunch the numbers", "repo": t.TempDir(), "coordinator": "tech-lead"}}); rejection != nil {
		t.Fatal(rejection)
	}
	return s, state, newLoop(s, state, t.TempDir())
}

// shared is you sharing a file with the task.
func shared(task, name string) events.Proposal {
	return events.Proposal{Stream: task, Actor: you, Type: events.FileAdded,
		Data: map[string]any{"name": name, "hash": strings.Repeat("ab", 32), "size": 1, "by": "you"}}
}
