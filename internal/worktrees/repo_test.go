package worktrees

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestANewRepoIsReadyForATask(t *testing.T) {
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	path := filepath.Join(t.TempDir(), "projects", "market-data")
	if reason := IsRepo(path); reason == "" {
		t.Error("a folder that isn't there is not a repo")
	}
	if err := NewRepo(path, "Design the market data system"); err != nil {
		t.Fatal(err)
	}
	if reason := IsRepo(path); reason != "" {
		t.Errorf("the new repo: %s", reason)
	}
	if subject, _ := git(path, "log", "-1", "--format=%s"); subject != "Start: Design the market data system" {
		t.Errorf("first commit: %q", subject)
	}
	w, err := Create(path, t.TempDir(), "task-9", 7909)
	if err != nil || w.Branch != "agentry/task-9" {
		t.Fatalf("a task's worktree from it: %+v %v", w, err)
	}

	// Never over work that is already there, and never a relative path.
	os.WriteFile(filepath.Join(path, "notes.md"), []byte("mine\n"), 0o644)
	if err := NewRepo(path, "again"); err == nil || !strings.Contains(err.Error(), "isn't empty") {
		t.Errorf("a folder with files in it: %v", err)
	}
	if err := NewRepo("projects/x", "x"); err == nil {
		t.Error("a relative path should be refused")
	}
}

func TestATaskThatMovesRepoGetsAWorktreeOfTheNewOne(t *testing.T) {
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	root := t.TempDir()
	first, second := filepath.Join(t.TempDir(), "first"), filepath.Join(t.TempDir(), "second")
	for _, repo := range []string{first, second} {
		if err := NewRepo(repo, filepath.Base(repo)); err != nil {
			t.Fatal(err)
		}
	}
	old, err := Create(first, root, "task-7", 7907)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(old.Path, "draft.md"), []byte("kept\n"), 0o644)

	moved, err := Create(second, root, "task-7", 7907)
	if err != nil {
		t.Fatal(err)
	}
	if !sameRepo(moved.Path, second) || moved.Path != old.Path {
		t.Errorf("the task's worktree should now be of the second repo, at the same place: %+v", moved)
	}
	if content, err := os.ReadFile(old.Path + ".before-move/draft.md"); err != nil || string(content) != "kept\n" {
		t.Errorf("the old worktree's work should be kept aside: %q %v", content, err)
	}
	if list, _ := git(first, "worktree", "list"); !strings.Contains(list, ".before-move") {
		t.Errorf("the first repo should know where its worktree went:\n%s", list)
	}
}
