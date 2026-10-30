package worktrees

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// A task's work moves between machines through git, with the repo the task
// was made from as the hub: the machine the work leaves commits what was left
// uncommitted and gives the hub its branch; the machine it goes to makes its
// worktree from the hub's branch. Each worktree exists on one machine at a time.

// CommitLeftovers commits whatever is in the worktree and not yet committed,
// so nothing is lost when the work moves. It reports whether there was any.
func (w Worktree) CommitLeftovers(message string) (bool, error) {
	status, err := git(w.Path, "status", "--porcelain")
	if err != nil || status == "" {
		return false, err
	}
	if _, err := git(w.Path, "add", "--all"); err != nil {
		return false, err
	}
	commit := []string{"commit", "--quiet", "--no-verify", "--message", message}
	if email, _ := git(w.Path, "config", "user.email"); email == "" {
		commit = append([]string{"-c", "user.name=Agentry", "-c", "user.email=agentry@localhost"}, commit...)
	}
	_, err = git(w.Path, commit...)
	return err == nil, err
}

// Discard removes the worktree and leaves its branch, as when the work has
// moved to another machine and the branch went with it.
func (w Worktree) Discard() error {
	if _, err := os.Stat(w.Path); os.IsNotExist(err) {
		_, err := git(w.Repo, "worktree", "prune")
		return err
	}
	if _, err := git(w.Repo, "worktree", "remove", "--force", w.Path); err != nil {
		return err
	}
	_, err := git(w.Repo, "worktree", "prune")
	return err
}

// DeleteBranch deletes the task's branch from repo, as from the hub once the
// work it carried is merged, or from a worker once the work has left it.
func DeleteBranch(repo, branch string) error {
	if !branchExists(repo, branch) {
		return nil
	}
	_, err := git(repo, "branch", "-D", branch)
	return err
}

// Bundle writes the branch's commits since base to file, for the hub, which
// has base already. An empty bundle is refused by git, so a branch with no
// commits since base writes nothing and says so.
func (w Worktree) Bundle(file string) (bool, error) {
	tip, err := git(w.Repo, "rev-parse", "refs/heads/"+w.Branch)
	if err != nil {
		return false, err
	}
	if tip == w.Base {
		return false, nil
	}
	_, err = git(w.Repo, "bundle", "create", file, "refs/heads/"+w.Branch, "^"+w.Base)
	return err == nil, err
}

// ReceiveBundle sets the branch in repo, the hub, to the bundle's. The
// branch may be checked out somewhere (git refuses to fetch into it then),
// so it comes in under its own ref and the branch is moved to it.
func ReceiveBundle(repo, branch, file string) error {
	if _, err := git(repo, "bundle", "verify", "--quiet", file); err != nil {
		return fmt.Errorf("the work didn't arrive whole: %w", err)
	}
	incoming := "refs/agentry/incoming/" + strings.ReplaceAll(branch, "/", "-")
	if _, err := git(repo, "fetch", "--quiet", "--no-tags", file, "+refs/heads/"+branch+":"+incoming); err != nil {
		return err
	}
	defer git(repo, "update-ref", "-d", incoming)
	tip, err := git(repo, "rev-parse", incoming)
	if err != nil {
		return err
	}
	_, err = git(repo, "update-ref", "refs/heads/"+branch, tip)
	return err
}

// Mirror is a worker's copy of a hub's repo: bare, one per repo, with the
// worktrees of every task from that repo on this machine.
type Mirror struct {
	Dir    string
	Hub    string // the hub's URL for git
	Header string // what git sends the hub to be let in, as "Authorization: Bearer ..."
	CAFile string // the hub's CA, PEM, which git trusts alone for it; "" for the system's store
	Origin string // the repo's own remote, where agents push; "" for none
}

// Worktree makes the task's worktree in root/<task> from the hub's copy of
// its branch; base is where the task started, kept from the hub. A worktree
// left from an earlier visit is out of date, so it goes first: git won't
// fetch into a branch a worktree has checked out. One of another repo, left
// by a task that moved repos, is moved aside instead.
func (m Mirror) Worktree(root, task, base string, port int) (Worktree, error) {
	w := Worktree{Repo: m.Dir, Path: filepath.Join(root, task), Branch: "agentry/" + task, Base: base, Port: port}
	if err := m.open(); err != nil {
		return Worktree{}, err
	}
	if isWorktree(w.Path) && !sameRepo(w.Path, m.Dir) { // left by the task's old repo
		if err := moveAside(w.Path); err != nil {
			return Worktree{}, err
		}
	}
	if err := w.Discard(); err != nil {
		return Worktree{}, err
	}
	fetch := append(m.hubConfig(), "fetch", "--quiet", "--no-tags", m.Hub, "+refs/heads/"+w.Branch+":refs/heads/"+w.Branch)
	if _, err := git(m.Dir, fetch...); err != nil {
		return Worktree{}, err
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return Worktree{}, err
	}
	if _, err := git(m.Dir, "worktree", "add", "--quiet", w.Path, w.Branch); err != nil {
		return Worktree{}, err
	}
	return w, w.ignoreAgentryFiles()
}

// hubConfig is what git is given to reach the hub: the header that lets it
// in, and the CA it trusts alone, if the hub has one. Git for Windows' own
// TLS (schannel) ignores a CA file unless told to use it.
func (m Mirror) hubConfig() []string {
	config := []string{"-c", "http.extraHeader=" + m.Header}
	if m.CAFile != "" {
		config = append(config, "-c", "http.sslCAInfo="+m.CAFile, "-c", "http.schannelUseSSLCAInfo=true")
	}
	return config
}

// open makes the mirror if there is none yet, and points its origin at the
// repo's own remote.
func (m Mirror) open() error {
	if _, err := os.Stat(filepath.Join(m.Dir, "HEAD")); os.IsNotExist(err) {
		if err := os.MkdirAll(m.Dir, 0o755); err != nil {
			return err
		}
		if _, err := git(m.Dir, "init", "--quiet", "--bare"); err != nil {
			return err
		}
	}
	current, _ := git(m.Dir, "remote", "get-url", "origin")
	var err error
	switch {
	case m.Origin == "" || current == m.Origin:
	case current == "":
		_, err = git(m.Dir, "remote", "add", "origin", m.Origin)
	default:
		_, err = git(m.Dir, "remote", "set-url", "origin", m.Origin)
	}
	return err
}

// Origin is the repo's own remote, "" if it has none.
func Origin(repo string) (string, error) {
	return git(repo, "remote", "get-url", "origin")
}

// RepoKey names a repo for a worker's copy of it: its folder's name, and a
// hash of its path so two repos with the same name never share a copy.
func RepoKey(repo string) string {
	sum := sha256.Sum256([]byte(filepath.Clean(repo)))
	return filepath.Base(repo) + "-" + hex.EncodeToString(sum[:4])
}
