// Package waitlist words the core's part of what waits for you: each
// approval, question and stop GET /api/questions lists, as one entry. The
// server lists them in GET /api/waiting; agentry mcp words them the same.
// It names no extension.
package waitlist

import (
	"fmt"
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// CloseEpic is the answer that approves closing an epic (coordinator.CloseEpic).
const CloseEpic = "Close the epic"

// CopyFile is the answer that lets a file be copied from outside a
// machine's worktrees (coordinator.CopyFile).
const CopyFile = "Copy it"

// Question is what an entry is worded from, as GET /api/questions lists it.
type Question struct {
	Kind, Task, TaskTitle, TaskKind, Agent, Text, Action string
	Options                                              []string
}

// Entry is q as one thing waiting for you. Its key lasts while q waits.
func Entry(q Question) extensions.Waiting {
	return extensions.Waiting{Key: q.Task + "/" + q.Kind + "/" + q.Action + "/" + q.Text, Task: q.Task, Line: Line(q), Kind: "needs you"}
}

// Line is q for a person, with the tool that deals with it.
func Line(q Question) string {
	id := tasks.ShownID(q.Task, tasks.Kind(q.TaskKind))
	switch q.Kind {
	case "approval":
		return fmt.Sprintf("%s: %s (review_approvals)", id, tasks.OneLine(tasks.WaitingLine(q.Agent, q.Text), 140))
	case "waiting":
		return fmt.Sprintf("%s: %s is stuck on a prompt in its terminal: %s", id, q.Agent, tasks.OneLine(q.Text, 120))
	}
	if slices.Contains(q.Options, CloseEpic) {
		return fmt.Sprintf("%s: the epic %s is ready to close (close_epic)", id, q.TaskTitle)
	}
	if slices.Contains(q.Options, CopyFile) {
		return fmt.Sprintf("%s: %s (answer_question with %q asks the user)", id, tasks.OneLine(q.Text, 200), CopyFile)
	}
	asker := q.Agent
	if asker == tasks.Loop {
		asker = "Agentry" // its own stops: a budget, a limit of turns
	}
	return fmt.Sprintf("%s: %s asks: %s", id, asker, tasks.OneLine(q.Text, 160))
}
