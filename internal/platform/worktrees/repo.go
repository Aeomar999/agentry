package worktrees

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
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

// IsAddress says whether repo is an address to copy it from, rather than a
// folder on this machine: any git address, such as
// git@gitlab.com:team/app.git, https://github.com/team/app or
// ssh://host/srv/app.git, or a link to an archive of it, such as
// https://example.com/app-1.2.tar.gz.
func IsAddress(repo string) bool {
	return !filepath.IsAbs(repo) && (strings.Contains(repo, "://") || scpAddress.MatchString(repo))
}

// scpAddress is git's short form for SSH: user@host:path.
var scpAddress = regexp.MustCompile(`^[\w.-]+@[\w.-]+:[^/\\]`)

// CloneTimeout is how long a clone, an update or a download may take.
const CloneTimeout = 10 * time.Minute

// Clone gives the folder in dir where the repo at address is checked out:
// copied the first time it is named, and after that, for a git address,
// brought up to date with its remote, so a new task starts from what the
// remote has now. If the update can't be made (the remote can't be reached,
// or the copy here has work of its own), the copy here is used as it is.
// An archive is downloaded once and made a repo of its own.
func Clone(dir, address string) (string, error) {
	folder := filepath.Join(dir, cloneName(address))
	unlock := lockFolder(folder)
	defer unlock()
	kind := archiveKind(address)
	if _, err := os.Stat(filepath.Join(folder, ".git")); err == nil {
		if kind == "" {
			quietly(folder, "pull", "--ff-only", "-q")
		}
		return folder, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if kind != "" {
		if err := download(folder, address, kind); err != nil {
			os.RemoveAll(folder)
			return "", err
		}
		return folder, nil
	}
	if output, err := quietly(dir, "clone", "-q", "--", address, folder); err != nil {
		os.RemoveAll(folder)
		return "", fmt.Errorf("cloning %s: %s", address, strings.TrimSpace(output))
	}
	if _, err := git(folder, "rev-parse", "--verify", "HEAD"); err != nil {
		os.RemoveAll(folder)
		return "", fmt.Errorf("%s has no commits yet: push one first, or make the task a new repo", address)
	}
	return folder, nil
}

// cloning holds a lock for each clone's folder, so two tasks naming the same
// address at once don't clone into it together, and one's failure doesn't
// take away the other's clone.
var cloning sync.Map

func lockFolder(folder string) (unlock func()) {
	lock, _ := cloning.LoadOrStore(folder, &sync.Mutex{})
	lock.(*sync.Mutex).Lock()
	return lock.(*sync.Mutex).Unlock
}

// quietly runs git where it can't stop to ask for a password or to trust a
// host key, so a clone that needs either fails and says why, rather than
// waiting for an answer no one can give.
func quietly(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), CloneTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if os.Getenv("GIT_SSH_COMMAND") == "" {
		command.Env = append(command.Env, "GIT_SSH_COMMAND=ssh -o BatchMode=yes -o StrictHostKeyChecking=accept-new")
	}
	output, err := command.CombinedOutput()
	return string(output), err
}

// cloneName names an address's folder: the repo's name, and a hash of the
// address so two repos with the same name never share a folder.
func cloneName(address string) string {
	name := strings.TrimSuffix(strings.TrimRight(address, "/"), ".git")
	if kind := archiveKind(address); kind != "" {
		link, _ := url.Parse(address) // a link's query isn't part of the name
		name = link.Path[:len(link.Path)-len(kind)]
	}
	if _, after, ok := strings.Cut(name, ":"); ok && !strings.Contains(name, "://") {
		name = after // git@host:team/app
	}
	sum := sha256.Sum256([]byte(address))
	return path.Base(name) + "-" + hex.EncodeToString(sum[:4])
}
