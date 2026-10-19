package mcp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/client"
)

// What waits for you, from any task, as one line each (open ideas share
// one), and the news Claude tells you of between your messages: what newly
// waits for you, tasks that finished, what a coordinator told you (its reply
// to you, an epic split into stories that wait for you to start them), and
// machines that went offline with work on them. The notifications and
// `agentry wait` are the same.

// Waiting is one thing waiting for you, or a task that finished.
type Waiting struct {
	Key  string // the same while it lasts: a new one has a new key
	Task string
	Line string // for a person: "task-4: deployer asks to run git push"
	Kind string // "needs you", "ideas" (open ones), "done" (a task finished), "said" (a coordinator told you) or "offline" (a machine)
}

// WaitingFor is everything waiting for you now, oldest first.
func WaitingFor(agentry client.Client) ([]Waiting, error) {
	var questions []question
	if err := agentry.Call("GET", "/api/questions", nil, &questions); err != nil {
		return nil, err
	}
	var all []Waiting
	for _, q := range questions {
		all = append(all, Waiting{Key: q.Task + "/" + q.Kind + "/" + q.Action + "/" + q.Text, Task: q.Task, Line: describe(q), Kind: "needs you"})
	}
	if ideas, ok := openIdeas(agentry, ""); ok { // ideas the server can't list are left out, not an error
		all = append(all, ideas)
	}
	return all, nil
}

// News is everything a waiter compares with what it saw before: what waits
// for you, every task that has finished, each task's coordinator's latest
// word to you, and the machines offline.
func News(agentry client.Client) ([]Waiting, error) {
	all, err := WaitingFor(agentry)
	if err != nil {
		return nil, err
	}
	var tasks []task
	if err := agentry.Call("GET", "/api/tasks", nil, &tasks); err != nil {
		return nil, err
	}
	for _, t := range tasks {
		if t.Status == "done" {
			all = append(all, Waiting{Key: "done/" + t.ID, Task: t.ID, Line: t.ID + " is done: " + t.Title, Kind: "done"})
		}
		if t.Told != nil {
			all = append(all, Waiting{Key: "said/" + t.ID + "/" + t.Told.At, Task: t.ID, Kind: "said",
				Line: fmt.Sprintf("%s (%s): %s says: %s", t.ID, t.Title, t.Told.From, oneLine(t.Told.Text, 400))})
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
					Line: fmt.Sprintf("the machine %s went offline; %s waits until it is back", w.Name, strings.Join(w.Tasks, ", "))})
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
	var t detail
	if w.Kind == "done" && agentry.Call("GET", "/api/tasks/"+w.Task, nil, &t) == nil && t.Summary != "" {
		w.Line += ". " + oneLine(t.Summary, 300)
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
