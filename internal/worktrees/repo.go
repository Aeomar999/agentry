package worktrees

import (
	"fmt"
	"os"
	"path/filepath"
)

// NewRepo makes the git repository a task's work will be, at path, for a task
// whose work is a new project rather than a change to an existing one. It
// gets a first, empty commit, so the task's branch has somewhere to start.
// The path must be absolute, and new or an empty folder.
func NewRepo(path, title string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("a new repo needs an absolute path, not %q", path)
	}
	if entries, err := os.ReadDir(path); err == nil && len(entries) > 0 {
		return fmt.Errorf("%s already exists and isn't empty: name a new folder, or use it as an existing repo", path)
	}
	if err := os.MkdirAll(path, 0o755); err != nil {
		return err
	}
	if _, err := git(path, "init", "-q", "-b", "main"); err != nil {
		return err
	}
	if _, err := git(path, "commit", "-q", "--allow-empty", "-m", "Start: "+title); err != nil {
		return fmt.Errorf("the first commit of the new repo: %w (is git's user.name and user.email set?)", err)
	}
	return nil
}

// IsRepo says why path isn't a git repository a task can work in, or "".
func IsRepo(path string) string {
	if !filepath.IsAbs(path) {
		return fmt.Sprintf("the repo needs an absolute path, not %q", path)
	}
	if _, err := git(path, "rev-parse", "--verify", "HEAD"); err != nil {
		return path + " isn't a git repository with a commit; to start one there, make it a new repo"
	}
	return ""
}
