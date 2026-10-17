package worktrees

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The work goes to a worker and back with nothing lost: what was committed,
// and what was left uncommitted on each machine.
func TestWorkMovesThroughTheHubAndBack(t *testing.T) {
	hub := newRepo(t)
	here, err := Create(hub, t.TempDir(), "task-1", 7901)
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(here.Path, "main.go"), "package main\n")
	mustGit(t, here.Path, "commit", "-qam", "fix: remove the TODO")
	write(t, filepath.Join(here.Path, "notes.md"), "half done\n") // left uncommitted

	// It leaves this machine.
	if left, err := here.CommitLeftovers("Agentry: work left uncommitted"); err != nil || !left {
		t.Fatalf("committing what was left: %v %v", left, err)
	}
	if err := here.Discard(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(here.Path); !os.IsNotExist(err) {
		t.Error("the worktree it left is still here")
	}

	// It arrives on the worker, from the hub's branch.
	mirror := Mirror{Dir: filepath.Join(t.TempDir(), RepoKey(hub)), Hub: hub, Header: "X-Test: 1", Origin: "https://example.com/app.git"}
	there, err := mirror.Worktree(t.TempDir(), "task-1", here.Base, 7902)
	if err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(filepath.Join(there.Path, "notes.md")); string(content) != "half done\n" {
		t.Errorf("the uncommitted work didn't travel: %q", content)
	}
	if origin := mustGit(t, there.Path, "remote", "get-url", "origin"); origin != "https://example.com/app.git" {
		t.Errorf("agents on the worker push to %q, want the repo's own remote", origin)
	}
	if status := mustGit(t, there.Path, "status", "--porcelain"); status != "" {
		t.Errorf("a worktree that just arrived should be clean, got %q", status)
	}

	// It goes back: the worker commits what it left and bundles the branch.
	write(t, filepath.Join(there.Path, "notes.md"), "done\n")
	there.CommitLeftovers("Agentry: work left uncommitted")
	bundle := filepath.Join(t.TempDir(), "task-1.bundle")
	if made, err := there.Bundle(bundle); err != nil || !made {
		t.Fatalf("bundle: %v %v", made, err)
	}
	if err := ReceiveBundle(hub, there.Branch, bundle); err != nil {
		t.Fatal(err)
	}
	if err := there.Discard(); err != nil {
		t.Fatal(err)
	}
	if err := DeleteBranch(mirror.Dir, there.Branch); err != nil {
		t.Fatal(err)
	}

	back, err := Create(hub, t.TempDir(), "task-1", 7901)
	if err != nil {
		t.Fatal(err)
	}
	if content, _ := os.ReadFile(filepath.Join(back.Path, "notes.md")); string(content) != "done\n" {
		t.Errorf("the worker's work didn't come back: %q", content)
	}
	if log := mustGit(t, back.Path, "log", "--format=%s"); !strings.Contains(log, "fix: remove the TODO") {
		t.Errorf("history lost:\n%s", log)
	}
	if back.Base != here.Base {
		t.Errorf("base moved from %s to %s", here.Base, back.Base)
	}
}

func TestABranchWithNothingNewBundlesNothing(t *testing.T) {
	w, _ := Create(newRepo(t), t.TempDir(), "task-1", 7901)
	if made, err := w.Bundle(filepath.Join(t.TempDir(), "b")); made || err != nil {
		t.Errorf("got %v %v, want nothing to send", made, err)
	}
	if left, err := w.CommitLeftovers("x"); left || err != nil {
		t.Errorf("a clean worktree had leftovers: %v %v", left, err)
	}
}
