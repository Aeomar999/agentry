// Package worktrees gives each task its own git worktree, branch and port, so
// agents on different tasks never work on (or test) each other's code (proof 06).
package worktrees

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Worktree struct {
	Repo   string // the main checkout it was made from
	Path   string
	Branch string
	Base   string // the commit the branch started from
	Port   int
}

// untrackedFiles are copied from the main checkout when git doesn't track them:
// without them the project often can't run (proof 06).
var untrackedFiles = []string{".env", ".env.local", ".env.development", ".env.test"}

// agentryFiles are what Agentry writes into a worktree; git is told to ignore
// them so they never end up in a commit.
var agentryFiles = []string{".agentry/", ".codex/hooks.json"}

// ErrNotMerged means the branch has work that isn't in dev yet, so the
// worktree is kept.
var ErrNotMerged = errors.New("the task branch isn't merged into dev")

// Create makes the worktree for a task in root/<task>, on branch agentry/<task>,
// starting from the repo's current commit. A task reopened after it was done
// may still have its worktree or its branch, if its work wasn't merged: then
// that worktree is used, or made again on that branch, so no work is lost.
// A worktree there of another repo, left by a task that moved repos, is
// moved aside, not used.
func Create(repo, root, task string, port int) (Worktree, error) {
	top, err := git(repo, "rev-parse", "--show-toplevel")
	if err != nil {
		return Worktree{}, fmt.Errorf("%s is not a git repository: %w", repo, err)
	}
	base, err := git(top, "rev-parse", "HEAD")
	if err != nil {
		return Worktree{}, fmt.Errorf("%s has no commits yet: %w", top, err)
	}
	w := Worktree{Repo: top, Path: filepath.Join(root, task), Branch: "agentry/" + task, Base: base, Port: port}

	if err := os.MkdirAll(root, 0o755); err != nil {
		return Worktree{}, err
	}
	if isWorktree(w.Path) && !sameRepo(w.Path, top) {
		if err := moveAside(w.Path); err != nil {
			return Worktree{}, err
		}
	}
	switch {
	case isWorktree(w.Path):
		return w.since(top) // set up when it was made
	case branchExists(top, w.Branch):
		if _, err := git(top, "worktree", "add", "-q", w.Path, w.Branch); err != nil {
			return Worktree{}, err
		}
		if w, err = w.since(top); err != nil {
			return w, err
		}
	default:
		if _, err := git(top, "worktree", "add", "-q", "-b", w.Branch, w.Path, base); err != nil {
			return Worktree{}, err
		}
	}
	for _, setUp := range []func() error{w.copyUntrackedFiles, w.linkDependencies, w.ignoreAgentryFiles} {
		if err := setUp(); err != nil {
			return w, err
		}
	}
	return w, nil
}

func (w Worktree) copyUntrackedFiles() error {
	for _, name := range untrackedFiles {
		content, err := os.ReadFile(filepath.Join(w.Repo, name))
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return err
		}
		if _, tracked := git(w.Repo, "ls-files", "--error-unmatch", name); tracked == nil {
			continue // git already put it in the worktree
		}
		if err := os.WriteFile(filepath.Join(w.Path, name), content, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// linkDependencies gives the worktree the main checkout's node_modules as hard
// links: about 10 ms against 0.4 s for an install, and no extra disk (proof 06).
func (w Worktree) linkDependencies() error {
	source := filepath.Join(w.Repo, "node_modules")
	if _, err := os.Stat(source); errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if output, err := exec.Command("cp", "-al", source, filepath.Join(w.Path, "node_modules")).CombinedOutput(); err != nil {
		return fmt.Errorf("linking node_modules: %v: %s", err, output)
	}
	return nil
}

// ignoreAgentryFiles adds Agentry's files to the repo's info/exclude, which
// every worktree shares and which is never committed.
func (w Worktree) ignoreAgentryFiles() error {
	common, err := git(w.Path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	exclude := filepath.Join(common, "info", "exclude")
	existing, _ := os.ReadFile(exclude)
	var missing []string
	for _, pattern := range agentryFiles {
		if !strings.Contains(string(existing), "\n"+pattern+"\n") && !strings.HasPrefix(string(existing), pattern+"\n") {
			missing = append(missing, pattern)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(exclude), 0o755); err != nil {
		return err
	}
	file, err := os.OpenFile(exclude, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer file.Close()
	if len(existing) > 0 && !strings.HasSuffix(string(existing), "\n") {
		file.WriteString("\n")
	}
	_, err = file.WriteString("# Agentry\n" + strings.Join(missing, "\n") + "\n")
	return err
}

// since sets the base of a branch that already has work: where it left the
// repo's current commit, so a review sees the task's work and nothing else.
func (w Worktree) since(top string) (Worktree, error) {
	base, err := git(top, "merge-base", "HEAD", w.Branch)
	if err != nil {
		return w, err
	}
	w.Base = base
	return w, nil
}

func isWorktree(path string) bool {
	_, err := os.Stat(filepath.Join(path, ".git"))
	return err == nil
}

// sameRepo says whether the worktree at path belongs to the repo at top.
func sameRepo(path, top string) bool {
	here, err1 := git(path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	there, err2 := git(top, "rev-parse", "--path-format=absolute", "--git-common-dir")
	return err1 == nil && err2 == nil && filepath.Clean(here) == filepath.Clean(there)
}

// moveAside moves a worktree of another repo out of the way, with git, so
// that repo still knows where it is and nothing in it is lost.
func moveAside(path string) error {
	common, err := git(path, "rev-parse", "--path-format=absolute", "--git-common-dir")
	if err != nil {
		return err
	}
	aside := path + ".before-move"
	for n := 2; isWorktree(aside); n++ {
		aside = fmt.Sprintf("%s.before-move-%d", path, n)
	}
	if _, err := exec.Command("git", "--git-dir="+common, "worktree", "move", path, aside).CombinedOutput(); err != nil {
		return fmt.Errorf("moving the worktree of the task's old repo aside: %w", err)
	}
	return nil
}

func branchExists(repo, branch string) bool {
	_, err := git(repo, "rev-parse", "--verify", "--quiet", "refs/heads/"+branch)
	return err == nil
}

// Remove takes the worktree and its branch away, but only once the branch is
// merged into dev. Setup leaves untracked files, so the removal is forced
// after that check; then the branch goes, then git forgets the worktree (proof 06).
func (w Worktree) Remove() error {
	if !w.isMerged() {
		return ErrNotMerged
	}
	if _, err := git(w.Repo, "worktree", "remove", "--force", w.Path); err != nil {
		return err
	}
	if _, err := git(w.Repo, "branch", "-D", w.Branch); err != nil {
		return err
	}
	_, err := git(w.Repo, "worktree", "prune")
	return err
}

// isMerged: the branch is in origin/dev (where the deployer pushes), or in a
// local dev when there is no remote.
func (w Worktree) isMerged() bool {
	for _, dev := range []string{"origin/dev", "dev"} {
		if _, err := git(w.Repo, "rev-parse", "--verify", "--quiet", dev); err != nil {
			continue
		}
		_, err := git(w.Repo, "merge-base", "--is-ancestor", w.Branch, dev)
		return err == nil
	}
	return false
}

func git(dir string, args ...string) (string, error) {
	output, err := exec.Command("git", append([]string{"-C", dir}, args...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %v: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
