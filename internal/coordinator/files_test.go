package coordinator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// Shared files on the real loop: what an agent leaves in .agentry/files is
// kept, a file you share reaches the next agent's folder, and the
// coordinator fetches from a machine: from its worktrees at once, from
// anywhere else once you say so.
func TestSharedFilesFollowTheWork(t *testing.T) {
	fakeClaudeOnPath(t)
	root := t.TempDir()
	s, _ := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	const id = "task-files-1"
	defer sessions.Stop(sessions.SocketFor(id))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go newLoop(s, root).Run(ctx)
	blobs := sharedfiles.Disk{Dir: filepath.Join(root, ".files")}

	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Crunch the numbers", "repo": gitRepo(t), "coordinator": "tech-lead"}})
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
	outside := filepath.Join(t.TempDir(), "private.txt")
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
