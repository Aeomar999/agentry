package mcp

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/platform/client"
)

// What waits for you, from any task, as one line each (an extension's lines as
// it says), and the news Claude tells you of between your messages: what
// newly waits for you, tasks that finished, what a coordinator told you (its
// reply to you, an epic split into stories that wait for you to start
// them), and machines that went offline with work on them. The
// notifications and `agentry wait` are the same.

// Waiting is one thing waiting for you, or a task that finished: Kind is
// "needs you", "done" (a task finished), "said" (a coordinator told you),
// "offline" (a machine), or an extension's own.
type Waiting = extensions.Waiting

// task is the part of a task on the board the news reads; it mirrors
// internal/domain/tasks/tasksapi/tools.go's task.
type task struct {
	ID, Title, Status, Kind string
	Told                    *struct{ At, From, Text string }
}

// WaitingFor is what GET /api/waiting lists for a session in repo ("" when
// not known): the core's entries oldest first, then each extension's.
func WaitingFor(agentry client.Client, repo string) ([]Waiting, error) {
	var all []Waiting
	if err := agentry.Call("GET", "/api/waiting?repo="+url.QueryEscape(repo), nil, &all); err != nil {
		return nil, err
	}
	return all, nil
}

// News is everything a waiter compares with what it saw before: what waits
// for you, every task that has finished, each task's coordinator's latest
// word to you, and the machines offline.
func News(agentry client.Client) ([]Waiting, error) {
	all, err := WaitingFor(agentry, "")
	if err != nil {
		return nil, err
	}
	var board []task
	if err := agentry.Call("GET", "/api/tasks", nil, &board); err != nil {
		return nil, err
	}
	for _, t := range board {
		if t.Status == "done" {
			all = append(all, Waiting{Key: "done/" + t.ID, Task: t.ID, Line: t.shown() + " is done: " + t.Title, Kind: "done"})
		}
		if t.Told != nil {
			all = append(all, Waiting{Key: "said/" + t.ID + "/" + t.Told.At, Task: t.ID, Kind: "said",
				Line: fmt.Sprintf("%s (%s): %s says: %s", t.shown(), t.Title, t.Told.From, tasks.OneLine(t.Told.Text, 400))})
		}
	}
	var fleet struct {
		Workers []struct {
			Name, Since string
			Online      bool
			Tasks       []string
		}
	}
	if agentry.Call("GET", "/api/workers", nil, &fleet) == nil {
		for _, w := range fleet.Workers {
			if !w.Online && len(w.Tasks) > 0 { // offline with nothing on it is no news
				all = append(all, Waiting{Key: "offline/" + w.Name + "/" + w.Since, Task: w.Tasks[0], Kind: "offline",
					Line: fmt.Sprintf("the machine %s went offline; %s waits until it is back", w.Name, strings.Join(idsAs(w.Tasks, shownIDs(board)), ", "))})
			}
		}
	}
	return all, nil
}

// WaitForNew returns once there is news that isn't in seen, with all that
// is new; a finished task comes with its summary. It follows the log, so
// it hears at once.
func WaitForNew(ctx context.Context, agentry client.Client, seen map[string]bool) ([]Waiting, error) {
	agentry.Timeout = 40 * time.Second // the server answers a wait within 25 seconds
	var position int64
	for {
		all, err := News(agentry)
		if err != nil {
			return nil, err
		}
		var fresh []Waiting
		for _, w := range all {
			if !seen[w.Key] {
				fresh = append(fresh, withSummary(agentry, w))
			}
		}
		if len(fresh) > 0 {
			return fresh, nil
		}
		var changes struct{ Position int64 }
		if err := agentry.Call("GET", fmt.Sprintf("/api/changes?after=%d", position), nil, &changes); err != nil {
			return nil, err
		}
		position = changes.Position
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
	}
}

// withSummary adds a finished task's summary to its line.
func withSummary(agentry client.Client, w Waiting) Waiting {
	var t struct{ Summary string }
	if w.Kind == "done" && agentry.Call("GET", "/api/tasks/"+w.Task, nil, &t) == nil && t.Summary != "" {
		w.Line += ". " + tasks.OneLine(t.Summary, 300)
	}
	return w
}

// Seen is the keys of what waits now, to wait for what comes after.
func Seen(waiting []Waiting) map[string]bool {
	seen := map[string]bool{}
	for _, w := range waiting {
		seen[w.Key] = true
	}
	return seen
}

// waitingNote is what waits for you anywhere, for the end of every reply.
func (s *Server) waitingNote() string {
	waiting, err := WaitingFor(s.agentry, s.repo)
	if err != nil {
		return ""
	}
	var notes []string
	for _, w := range waiting {
		notes = append(notes, w.Line)
	}
	if len(notes) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\n\nWaiting for the user (%d):", len(notes))
	for _, note := range notes[:min(5, len(notes))] {
		b.WriteString("\n- " + note)
	}
	return b.String()
}

// shown is the task's id as you read it: epic-N for an epic. It mirrors
// internal/domain/tasks/tasksapi/tools.go's.
func (t task) shown() string { return tasks.ShownID(t.ID, tasks.Kind(t.Kind)) }

// shownIDs gives each task's id as you read it, by its kind on the board
// all; an id not on it stays as it is. It mirrors
// internal/domain/tasks/tasksapi/tools.go's.
func shownIDs(all []task) func(id string) string {
	kinds := map[string]tasks.Kind{}
	for _, t := range all {
		kinds[t.ID] = tasks.Kind(t.Kind)
	}
	return func(id string) string { return tasks.ShownID(id, kinds[id]) }
}

// idsAs are the ids, each as id gives it. It mirrors
// internal/domain/tasks/tasksapi/tools.go's.
func idsAs(ids []string, id func(string) string) []string {
	var out []string
	for _, each := range ids {
		out = append(out, id(each))
	}
	return out
}
