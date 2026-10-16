// Package workspace reads a task's worktree for the page's peek: its files,
// what changed since the commit the task started from, and any file as it
// was then and as it is now. It only reads, and never outside the worktree.
package workspace

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// Change is what happened to a file since the base: "M" modified, "A"
// added, "D" deleted, or "" for unchanged.
type Change string

const (
	Modified  Change = "M"
	Added     Change = "A"
	Deleted   Change = "D"
	Unchanged Change = ""
)

type File struct {
	Path   string `json:"path"` // slash-separated, from the worktree's root
	Change Change `json:"change"`
}

// Workspace is one task's worktree and the commit it started from.
type Workspace struct {
	Dir  string
	Base string
}

// ErrOutside is a path that would leave the worktree.
var ErrOutside = errors.New("that path is outside the workspace")

// Files lists every file in the worktree, tracked and new (what .gitignore
// leaves out stays out), plus the deleted ones, each with its change.
func (w Workspace) Files() ([]File, error) {
	if _, err := os.Stat(w.Dir); err != nil {
		return nil, fmt.Errorf("the workspace is gone: %w", err)
	}
	changes := map[string]Change{}
	diff, err := w.git("diff", "--name-status", "--no-renames", w.Base)
	if err != nil {
		return nil, err
	}
	for _, line := range lines(diff) {
		status, path, _ := strings.Cut(line, "\t")
		switch status[:1] {
		case "A":
			changes[path] = Added
		case "D":
			changes[path] = Deleted
		default:
			changes[path] = Modified
		}
	}
	untracked, err := w.git("ls-files", "--others", "--exclude-standard")
	if err != nil {
		return nil, err
	}
	for _, path := range lines(untracked) {
		changes[path] = Added
	}

	tracked, err := w.git("ls-files")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var files []File
	for _, path := range append(lines(tracked), lines(untracked)...) {
		if !seen[path] {
			seen[path] = true
			files = append(files, File{Path: path, Change: changes[path]})
		}
	}
	for path, change := range changes {
		if change == Deleted && !seen[path] {
			files = append(files, File{Path: path, Change: Deleted})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// Contents is a file now and at the base. A file that is too big or isn't
// text is said to be so instead of sent.
type Contents struct {
	Path   string `json:"path"`
	Now    string `json:"now"`
	Before string `json:"before"` // at the base; "" for a file added since
	Binary bool   `json:"binary"`
	TooBig bool   `json:"tooBig"`
}

const mostBytes = 2 << 20

func (w Workspace) Read(path string) (Contents, error) {
	full, err := w.inside(path)
	if err != nil {
		return Contents{}, err
	}
	c := Contents{Path: path}
	now, err := os.ReadFile(full)
	if err != nil && !os.IsNotExist(err) { // a deleted file has no "now"
		return c, err
	}
	before, _ := w.git("show", w.Base+":"+filepath.ToSlash(path)) // missing at the base: added since
	switch {
	case len(now) > mostBytes || len(before) > mostBytes:
		c.TooBig = true
	case isBinary(now) || isBinary([]byte(before)):
		c.Binary = true
	default:
		c.Now, c.Before = string(now), before
	}
	return c, nil
}

// inside resolves path within the worktree, refusing anything that would
// leave it (.. or a link out) and git's own folder.
func (w Workspace) inside(path string) (string, error) {
	clean := filepath.Clean("/" + path)[1:]
	if clean == "" || clean == ".git" || strings.HasPrefix(clean, ".git/") {
		return "", ErrOutside
	}
	full := filepath.Join(w.Dir, clean)
	if real, err := filepath.EvalSymlinks(full); err == nil {
		root, _ := filepath.EvalSymlinks(w.Dir)
		if real != root && !strings.HasPrefix(real, root+string(filepath.Separator)) {
			return "", ErrOutside
		}
	}
	return full, nil
}

func (w Workspace) git(args ...string) (string, error) {
	out, err := exec.Command("git", append([]string{"-C", w.Dir}, args...)...).Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", args[0], err)
	}
	return string(out), nil
}

func lines(text string) []string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

func isBinary(data []byte) bool {
	return bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0
}
