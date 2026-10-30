package tasks

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// A task's shared files: what its agents, on any machine, and you hand
// each other that isn't in git (package sharedfiles). The log says which
// files the task has; what is in them is kept on the server, by hash.

// SharedFile is one file shared with a task.
type SharedFile struct {
	Name string
	Hash string
	Size int64
	By   string // the agent whose turn left it, or you
	At   time.Time
	From string // where it was fetched from, as machine:path, if it was
}

// Fetch is a file the coordinator asked for from a machine.
type Fetch struct {
	Machine string
	Path    string
	As      string // the name to keep it by; "" for its own
}

// SharedFiles are the task's files, by name.
func (t *Task) SharedFiles() []SharedFile {
	files := make([]SharedFile, 0, len(t.Files))
	for _, f := range t.Files {
		files = append(files, f)
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files
}

// FilesCollected says whether the turn's folder was read back.
func (t *Task) FilesCollected(turn string) bool { return t.collected[turn] }

// LastEndedTurn is the latest turn that has ended, or nil.
func (t *Task) LastEndedTurn() *Turn {
	for i := len(t.Turns) - 1; i >= 0; i-- {
		if t.Turns[i].Ended {
			return t.Turns[i]
		}
	}
	return nil
}

func (t *Task) applyFiles(e events.Event) {
	switch e.Type {
	case events.FileAdded:
		t.addFile(e, e.Text("name"), e.Text("hash"), int64(e.Number("size")), "")
	case events.FileRemoved:
		delete(t.Files, e.Text("name"))
		t.FilesVersion++
	case events.FilesCollected:
		if t.collected == nil {
			t.collected = map[string]bool{}
		}
		t.collected[e.Text("turn")] = true
	case events.FilesDelivered:
		t.Delivered = e.Text("worker") + "@" + e.Text("version")
	case events.FileFetching:
		t.Fetching = &Fetch{Machine: e.Text("machine"), Path: e.Text("path"), As: e.Text("as")}
		t.PendingDecision, t.Handoff = "", nil
		t.Trail = append(t.Trail, Step{At: e.At, From: e.Actor.ID, To: Agentry,
			Text: "Fetching " + e.Text("machine") + ":" + e.Text("path") + " into the task's shared files."})
	case events.FilesFetched:
		from := e.Text("from")
		var names []string
		files, _ := e.Data["files"].([]any)
		for _, item := range files {
			f, _ := item.(map[string]any)
			name, _ := f["name"].(string)
			hash, _ := f["hash"].(string)
			size, _ := f["size"].(float64)
			t.addFile(e, name, hash, int64(size), from)
			names = append(names, name)
		}
		t.Fetching = nil
		if e.Flag("answers") { // you approved it: that was your answer
			t.Question, t.Asked, t.TurnsSinceAnswer, t.IssuesInARow = "", Asked{}, 0, 0
		}
		t.step(e, Agentry, t.Coordinator, "Copied "+from+" into the task's shared files: "+joinNames(names)+".")
	}
}

// FileHashes are the blobs of shared files an event names: a file.added's,
// and each of a files.fetched's. Erasing a task deletes those no other
// task's event names (docs/design/log-growth.md section 5).
func FileHashes(e events.Event) []string {
	switch e.Type {
	case events.FileAdded:
		return []string{e.Text("hash")}
	case events.FilesFetched:
		var hashes []string
		files, _ := e.Data["files"].([]any)
		for _, item := range files {
			f, _ := item.(map[string]any)
			if hash, _ := f["hash"].(string); hash != "" {
				hashes = append(hashes, hash)
			}
		}
		return hashes
	}
	return nil
}

func (t *Task) addFile(e events.Event, name, hash string, size int64, from string) {
	if t.Files == nil {
		t.Files = map[string]SharedFile{}
	}
	t.Files[name] = SharedFile{Name: name, Hash: hash, Size: size, By: firstNonEmpty(e.Text("by"), e.Actor.ID), At: e.At, From: from}
	t.FilesVersion++
}

func joinNames(names []string) string {
	if len(names) > 6 {
		return joinNames(names[:6]) + " and " + strconv.Itoa(len(names)-6) + " more"
	}
	return strings.Join(names, ", ")
}
