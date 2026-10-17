// Package workspace reads a task's worktree for its Workspace page, the way an
// editor's git view does: every file with its git status against the last
// commit, the branch's commits since the task started with what each
// changed, and any file next to its last committed version (or, in a
// commit, next to the version before it). It only reads, never outside the
// worktree.
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
	"time"
)

// Change is a file's git status, as an editor shows it: "M" modified, "A"
// added to the index, "D" deleted, "U" untracked (new, not added), or ""
// for a file as it was committed. In a commit, what the commit did to it.
type Change string

const (
	Modified  Change = "M"
	Added     Change = "A"
	Deleted   Change = "D"
	Untracked Change = "U"
	Unchanged Change = ""
)

type File struct {
	Path   string `json:"path"` // slash-separated, from the worktree's root
	Change Change `json:"change"`
}

// Commit is one commit on the task's branch since it started.
type Commit struct {
	Hash    string    `json:"hash"`
	Author  string    `json:"author"`
	At      time.Time `json:"at"`
	Subject string    `json:"subject"`
	Files   []File    `json:"files"` // what it changed
}

// Workspace is one task's worktree and the commit it started from. With
// Commit set, Read shows a file in that commit instead of now.
type Workspace struct {
	Dir    string
	Base   string
	Commit string `json:",omitempty"`
}

// ErrOutside is a path that would leave the worktree.
var ErrOutside = errors.New("that path is outside the workspace")

// Files lists every file in the worktree, tracked and new (what .gitignore
// leaves out stays out), plus the deleted ones, each with its git status.
func (w Workspace) Files() ([]File, error) {
	if _, err := os.Stat(w.Dir); err != nil {
		return nil, fmt.Errorf("the workspace is gone: %w", err)
	}
	status, err := w.git("status", "--porcelain=v1", "-z", "--untracked-files=all", "--no-renames")
	if err != nil {
		return nil, err
	}
	changes := map[string]Change{}
	for _, entry := range strings.Split(status, "\x00") {
		if len(entry) < 4 {
			continue
		}
		changes[entry[3:]] = statusOf(entry[0], entry[1])
	}
	tracked, err := w.git("ls-files")
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var files []File
	for _, path := range lines(tracked) {
		seen[path] = true
		files = append(files, File{Path: path, Change: changes[path]})
	}
	for path, change := range changes {
		if !seen[path] {
			files = append(files, File{Path: path, Change: change})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

// statusOf reads one line of git status: what the index (x) and the working
// tree (y) say, as one letter.
func statusOf(x, y byte) Change {
	switch {
	case x == '?':
		return Untracked
	case x == 'D' || y == 'D':
		return Deleted
	case x == 'A':
		return Added
	default:
		return Modified
	}
}

// Commits are the branch's commits since the task started, newest first,
// each with the files it changed.
func (w Workspace) Commits() ([]Commit, error) {
	log, err := w.git("log", "--format=%x1e%H%x1f%an%x1f%cI%x1f%s", "--name-status", "--no-renames", w.Base+"..HEAD")
	if err != nil {
		return nil, err
	}
	commits := []Commit{}
	for _, record := range strings.Split(log, "\x1e")[1:] {
		head, rest, _ := strings.Cut(record, "\n")
		fields := strings.SplitN(head, "\x1f", 4)
		if len(fields) < 4 {
			continue
		}
		at, _ := time.Parse(time.RFC3339, fields[2])
		commit := Commit{Hash: fields[0], Author: fields[1], At: at, Subject: fields[3], Files: []File{}}
		for _, line := range lines(rest) {
			status, path, _ := strings.Cut(line, "\t")
			if path == "" {
				continue
			}
			change := Modified
			switch status[:1] {
			case "A":
				change = Added
			case "D":
				change = Deleted
			}
			commit.Files = append(commit.Files, File{Path: path, Change: change})
		}
		commits = append(commits, commit)
	}
	return commits, nil
}

// Contents is a file now and as last committed (or, in a commit, in it and
// before it). A file that is too big or isn't text is said to be so instead
// of sent.
type Contents struct {
	Path   string `json:"path"`
	Now    string `json:"now"`
	Before string `json:"before"` // last committed, or before the commit; "" for a file new since
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
	var now []byte
	before, from := "", "HEAD"
	if w.Commit != "" {
		if !isHash(w.Commit) {
			return c, fmt.Errorf("%q is not a commit", w.Commit)
		}
		at, _ := w.git("show", w.Commit+":"+filepath.ToSlash(path)) // missing: the commit deleted it
		now, from = []byte(at), w.Commit+"^"
	} else if now, err = os.ReadFile(full); err != nil && !os.IsNotExist(err) { // a deleted file has no "now"
		return c, err
	}
	before, _ = w.git("show", from+":"+filepath.ToSlash(path)) // missing then: new since
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

// isHash is a full or short commit hash, and nothing git would read as an option.
func isHash(text string) bool {
	if len(text) < 7 || len(text) > 40 {
		return false
	}
	for _, r := range text {
		if !strings.ContainsRune("0123456789abcdef", r) {
			return false
		}
	}
	return true
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
