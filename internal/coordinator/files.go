package coordinator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// A task's shared files follow its work: after each turn the folder is read
// back (collect), and before each turn the machine's folder is made to
// hold the task's files, and a story's epic's (deliver). The coordinator
// can also bring a file from any machine (fetch): from inside that
// machine's worktrees freely, from anywhere else once you approve.
//
// Each of these can take a while, so they run beside the loop, one at a
// time per task, and say what they did as events, which wake the loop again.

// CopyFile is the answer that approves copying a file from outside a
// machine's worktrees.
const CopyFile = "Copy it"

// fileWork is the tasks with files being moved now.
type fileWork struct {
	mu   sync.Mutex
	busy map[string]bool
}

// beside runs work for the task, unless some is running for it already, or
// the task has changed since the loop read it: the event that changed it
// wakes the loop again, to decide from what the task is now. Work returns
// its last event, which is proposed before the task is free again, so the
// pass that event wakes never finds it busy.
func (l *Loop) beside(task *tasks.Task, work func(ctx context.Context) events.Proposal) {
	l.files.mu.Lock()
	defer l.files.mu.Unlock()
	if l.files.busy == nil {
		l.files.busy = map[string]bool{}
	}
	if now, _ := l.state.Task(task.ID); l.files.busy[task.ID] || now != task {
		return
	}
	l.files.busy[task.ID] = true
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Hour)
		defer cancel()
		last := work(ctx)
		l.files.mu.Lock()
		defer l.files.mu.Unlock()
		l.tell(last)
		delete(l.files.busy, task.ID)
	}()
}

// told is an event for the task, as work beside the loop ends with.
func told(task *tasks.Task, actor events.Actor, kind events.Type, data map[string]any) events.Proposal {
	return events.Proposal{Stream: task.ID, Actor: actor, Type: kind, Data: data}
}

// readyForTurn says whether the task's files are where its next turn, on
// target, needs them; if not, it sets that going.
func (l *Loop) readyForTurn(task *tasks.Task, target string) bool {
	if last := task.LastEndedTurn(); last != nil && !task.FilesCollected(last.ID) {
		l.beside(task, func(ctx context.Context) events.Proposal { return l.collect(ctx, task, last) })
		return false
	}
	files, version := l.filesFor(task)
	if version == "0.0" || task.Delivered == target+"@"+version {
		return true // nothing to share, or shared already
	}
	l.beside(task, func(ctx context.Context) events.Proposal { return l.deliver(ctx, task, target, files, version) })
	return false
}

// collect reads the turn's folder back: what is new or changed is sent to
// the server and recorded; a file the turn removed is no longer shared.
func (l *Loop) collect(ctx context.Context, task *tasks.Task, turn *tasks.Turn) events.Proposal {
	done := map[string]any{"turn": turn.ID}
	collected := told(task, agentry, events.FilesCollected, done)
	if task.Worktree == nil {
		return collected
	}
	m := l.fleet.Of(task.Worktree.Worker)
	found, err := m.SharedManifest(ctx, task.Worktree.Path)
	if err != nil {
		done["error"] = err.Error()
		slog.Warn("coordinator loop: reading a turn's shared files", "task", task.ID, "err", err)
		return collected
	}
	var changed []string
	seen := map[string]bool{}
	for _, f := range found {
		seen[f.Name] = true
		if task.Files[f.Name].Hash != f.Hash {
			changed = append(changed, f.Name)
		}
	}
	if len(changed) > 0 {
		sent, err := m.SharedUpload(ctx, task.Worktree.Path, changed)
		if err != nil {
			done["error"] = err.Error()
			slog.Warn("coordinator loop: sending a turn's shared files", "task", task.ID, "err", err)
			return collected
		}
		for _, f := range sent {
			l.propose(task.ID, agentry, events.FileAdded, map[string]any{"name": f.Name, "hash": f.Hash, "size": f.Size,
				"by": turn.Agent, "turn": turn.ID})
		}
	}
	for name, f := range task.Files {
		// Gone from the folder: the turn removed it, if it was there when
		// the turn started. One you shared since hasn't reached it yet.
		if !seen[name] && !strings.HasPrefix(name, sharedfiles.EpicFolder+"/") && f.At.Before(turn.StartedAt) {
			l.propose(task.ID, agentry, events.FileRemoved, map[string]any{"name": name, "by": turn.Agent, "turn": turn.ID})
		}
	}
	return collected
}

// deliver makes the machine's folder hold the task's files.
func (l *Loop) deliver(ctx context.Context, task *tasks.Task, target string, files []sharedfiles.File, version string) events.Proposal {
	m := l.fleet.Of(target)
	if err := m.SharedDeliver(ctx, task.Worktree.Path, files); err != nil {
		return told(task, agentry, events.IssueRaised, map[string]any{"reason": "the task's shared files couldn't be put on " +
			target + ": " + sentence(err)})
	}
	return told(task, agentry, events.FilesDelivered, map[string]any{"worker": target, "version": version})
}

// filesFor are the task's files and, for a story, its epic's, under epic/;
// version changes whenever either does.
func (l *Loop) filesFor(task *tasks.Task) ([]sharedfiles.File, string) {
	var files []sharedfiles.File
	for _, f := range task.SharedFiles() {
		files = append(files, sharedfiles.File{Name: f.Name, Hash: f.Hash, Size: f.Size})
	}
	epicVersion := 0
	if task.Epic != "" {
		if epic, _, err := l.load(task.Epic); err == nil && epic != nil {
			epicVersion = epic.FilesVersion
			for _, f := range epic.SharedFiles() {
				files = append(files, sharedfiles.File{Name: sharedfiles.EpicFolder + "/" + f.Name, Hash: f.Hash, Size: f.Size})
			}
		}
	}
	return files, fmt.Sprintf("%d.%d", task.FilesVersion, epicVersion)
}

// fetch brings what the coordinator asked for: from inside the machine's
// worktrees at once; from anywhere else, it asks you first.
func (l *Loop) fetch(ctx context.Context, task *tasks.Task, f tasks.Fetch) events.Proposal {
	m, online := l.fleet.Get(f.Machine)
	if !online {
		return told(task, agentry, events.IssueRaised, map[string]any{"reason": "the fetch couldn't start: " + f.Machine + " is offline"})
	}
	files, err := m.SharedFetch(ctx, f.Path, f.As, false)
	switch {
	case errors.Is(err, sharedfiles.ErrOutside):
		return told(task, agentry, events.TaskNeedsYou, map[string]any{
			"question": fmt.Sprintf("%s wants to copy %s:%s into this task's shared files. It is outside that machine's worktrees, "+
				"so it is yours to allow. Copy it?", task.Coordinator, f.Machine, f.Path),
			"options": []string{CopyFile, "Don't copy it"}, "fetchMachine": f.Machine, "fetchPath": f.Path, "fetchAs": f.As})
	case err != nil:
		return told(task, agentry, events.IssueRaised, map[string]any{"reason": "the fetch of " + f.Machine + ":" + f.Path +
			" failed: " + sentence(err)})
	}
	return told(task, agentry, events.FilesFetched, Fetched(f, files, false))
}

// Fetched is the event's data for files fetched as f; answers when you
// approved it, so it answers your question too.
func Fetched(f tasks.Fetch, files []sharedfiles.File, answers bool) map[string]any {
	list := make([]any, 0, len(files))
	for _, file := range files {
		list = append(list, map[string]any{"name": file.Name, "hash": file.Hash, "size": file.Size})
	}
	return map[string]any{"files": list, "from": f.Machine + ":" + f.Path, "answers": answers}
}

// machineNamed is the machine a decision named, by its name or its host's.
func (l *Loop) machineNamed(wanted string) (string, bool) {
	all := l.machines()
	i := slices.IndexFunc(all, func(m machine) bool {
		return strings.EqualFold(m.name, strings.TrimSpace(wanted)) || strings.EqualFold(m.host, strings.TrimSpace(wanted))
	})
	if i < 0 {
		return "", false
	}
	return all[i].name, true
}
