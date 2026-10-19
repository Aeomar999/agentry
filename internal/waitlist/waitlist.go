// Package waitlist words the core's part of what waits for you: each
// approval, question and stop GET /api/questions lists, as one entry. The
// server lists them in GET /api/waiting; agentry mcp words them the same.
// It names no feature.
package waitlist

import (
	"fmt"
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// CloseEpic is the answer that approves closing an epic (coordinator.CloseEpic).
const CloseEpic = "Close the epic"

// CopyFile is the answer that lets a file be copied from outside a
// machine's worktrees (coordinator.CopyFile).
const CopyFile = "Copy it"

// Question is what an entry is worded from, as GET /api/questions lists it.
type Question struct {
	Kind, Task, TaskTitle, Agent, Text, Action string
	Options                                    []string
}

// Entry is q as one thing waiting for you. Its key lasts while q waits.
func Entry(q Question) features.Waiting {
	return features.Waiting{Key: q.Task + "/" + q.Kind + "/" + q.Action + "/" + q.Text, Task: q.Task, Line: Line(q), Kind: "needs you"}
}

// Line is q for a person, with the tool that deals with it.
func Line(q Question) string {
	switch q.Kind {
	case "approval":
		return fmt.Sprintf("%s: %s (review_approvals)", q.Task, tasks.OneLine(tasks.WaitingLine(q.Agent, q.Text), 140))
	case "waiting":
		return fmt.Sprintf("%s: %s is stuck on a prompt in its terminal: %s", q.Task, q.Agent, tasks.OneLine(q.Text, 120))
	}
	if slices.Contains(q.Options, CloseEpic) {
		return fmt.Sprintf("%s: the epic %s is ready to close (close_epic)", q.Task, q.TaskTitle)
	}
	if slices.Contains(q.Options, CopyFile) {
		return fmt.Sprintf("%s: %s (answer_question with %q asks the user)", q.Task, tasks.OneLine(q.Text, 200), CopyFile)
	}
	asker := q.Agent
	if asker == tasks.Loop {
		asker = "Agentry" // its own stops: a budget, a limit of turns
	}
	return fmt.Sprintf("%s: %s asks: %s", q.Task, asker, tasks.OneLine(q.Text, 160))
}
