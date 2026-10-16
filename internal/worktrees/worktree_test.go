package worktrees

import (
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// newRepo makes a repo on dev with one commit, an untracked .env and node_modules.
func newRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.com")
	mustGit(t, repo, "init", "-q", "-b", "dev")
	write(t, filepath.Join(repo, "main.go"), "package main // TODO remove\n")
	mustGit(t, repo, "add", ".")
	mustGit(t, repo, "commit", "-qm", "first")
	write(t, filepath.Join(repo, ".env"), "SECRET=1\n")
	write(t, filepath.Join(repo, "node_modules", "left-pad", "index.js"), "module.exports = 1\n")
	write(t, filepath.Join(repo, ".gitignore"), ".env\nnode_modules/\n")
	mustGit(t, repo, "add", ".gitignore")
	mustGit(t, repo, "commit", "-qm", "ignore")
	return repo
}

func TestCreateSetsTheWorktreeUp(t *testing.T) {
	repo := newRepo(t)
	w, err := Create(repo, t.TempDir(), "task-1", 7901)
	if err != nil {
		t.Fatal(err)
	}
	if w.Branch != "agentry/task-1" || w.Port != 7901 || len(w.Base) != 40 {
		t.Errorf("worktree: %+v", w)
	}
	if content, _ := os.ReadFile(filepath.Join(w.Path, ".env")); string(content) != "SECRET=1\n" {
		t.Errorf(".env not copied: %q", content)
	}
	original, _ := os.Stat(filepath.Join(repo, "node_modules", "left-pad", "index.js"))
	linked, err := os.Stat(filepath.Join(w.Path, "node_modules", "left-pad", "index.js"))
	if err != nil || !os.SameFile(original, linked) {
		t.Errorf("node_modules should be hard links: %v", err)
	}

	// Agentry's own files never show up as changes.
	write(t, filepath.Join(w.Path, ".agentry", "turn-1.md"), "brief")
	write(t, filepath.Join(w.Path, ".codex", "hooks.json"), "{}")
	if status := mustGit(t, w.Path, "status", "--porcelain"); status != "" {
		t.Errorf("git status should be clean, got %q", status)
	}
	// Creating a second task's worktree doesn't add the exclude lines twice.
	if _, err := Create(repo, t.TempDir(), "task-2", 7902); err != nil {
		t.Fatal(err)
	}
	exclude, _ := os.ReadFile(filepath.Join(repo, ".git", "info", "exclude"))
	if strings.Count(string(exclude), ".agentry/") != 1 {
		t.Errorf("exclude:\n%s", exclude)
	}
}

func TestRemoveOnlyOnceMerged(t *testing.T) {
	repo := newRepo(t)
	w, _ := Create(repo, t.TempDir(), "task-1", 7901)
	write(t, filepath.Join(w.Path, "main.go"), "package main\n")
	mustGit(t, w.Path, "commit", "-qam", "fix: remove the TODO")
	write(t, filepath.Join(w.Path, "setup.lock"), "left by setup") // proof 06: blocks a plain remove

	if err := w.Remove(); !errors.Is(err, ErrNotMerged) {
		t.Fatalf("an unmerged branch must be kept, got %v", err)
	}
	mustGit(t, repo, "merge", "-q", "--ff-only", w.Branch)
	if err := w.Remove(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(w.Path); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("the worktree is still there")
	}
	if branches := mustGit(t, repo, "branch", "--list", w.Branch); branches != "" {
		t.Errorf("the branch is still there: %q", branches)
	}
}

func TestCreateRefusesAFolderThatIsNotARepo(t *testing.T) {
	if _, err := Create(t.TempDir(), t.TempDir(), "task-1", 7901); err == nil || !strings.Contains(err.Error(), "not a git repository") {
		t.Errorf("got %v", err)
	}
}

func TestLeasePortSkipsTakenAndBusyPorts(t *testing.T) {
	busy, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", FirstPort+1))
	if err != nil {
		t.Skip("port in use already:", err)
	}
	defer busy.Close()
	port, err := LeasePort([]int{FirstPort})
	if err != nil || port != FirstPort+2 {
		t.Errorf("got %d, %v; want %d (first taken, second busy)", port, err, FirstPort+2)
	}
}

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	output, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func write(t *testing.T, path, content string) {
	t.Helper()
	os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// The deployer's way (deployer.md): merge origin/dev in, push HEAD:dev. After
// that the branch is in origin/dev, so the worktree can go.
func TestRemoveAfterTheDeployerPushesToOriginDev(t *testing.T) {
	repo := newRepo(t)
	origin := t.TempDir()
	mustGit(t, origin, "init", "-q", "--bare", "-b", "dev")
	mustGit(t, repo, "remote", "add", "origin", origin)
	mustGit(t, repo, "push", "-q", "origin", "dev")

	w, _ := Create(repo, t.TempDir(), "task-1", 7901)
	write(t, filepath.Join(w.Path, "main.go"), "package main\n")
	mustGit(t, w.Path, "commit", "-qam", "fix: remove the TODO")
	mustGit(t, w.Path, "fetch", "-q", "origin")
	mustGit(t, w.Path, "merge", "-q", "origin/dev")
	if err := w.Remove(); !errors.Is(err, ErrNotMerged) {
		t.Fatalf("before the push: %v", err)
	}
	mustGit(t, w.Path, "push", "-q", "origin", "HEAD:dev")
	if err := w.Remove(); err != nil {
		t.Errorf("after the push: %v", err)
	}
}
