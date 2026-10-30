package tasksapi

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks/waitlist"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/platform/schema"
)

// ---- what only the user decides -----------------------------------------------

// closeEpic asks you, in Claude Code, whether to close the epic: one that
// asks to be closed, or one you left open with every story done.
func (c Client) closeEpic(ctx context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Epic string }
	json.Unmarshal(arguments, &a)
	id, shown := tasks.StoredID(a.Epic), tasks.ShownEpic(tasks.StoredID(a.Epic))
	title, text, ok := c.closable(id)
	if !ok {
		return "", fmt.Errorf("%s isn't ready to close: not every story of it is done", a.Epic) // as you named it: it may be no epic
	}
	ask := extensions.AskOf(ctx)
	if ask == nil {
		return "Closing an epic is the user's to approve. Give them this link: " + c.Agentry.PageURL("/tasks/"+id), nil
	}
	answer, ok, err := ask(ctx, "Close "+shown+", "+title+"?\n\n"+text, schema.Object(schema.Props{
		"decision": schema.Enum("Close it, or keep it open to tell its coordinator what else it needs", "close", "keep open"),
	}, "decision"))
	if err != nil {
		return "", err
	}
	if !ok || answer["decision"] != "close" {
		return "The user kept " + shown + " open. Ask them what else it needs, and send it with answer_question.", nil
	}
	if err := c.Agentry.Call("POST", "/api/tasks/"+id+"/answer", map[string]string{"answer": waitlist.CloseEpic}, nil); err != nil {
		return "", err
	}
	return "The user closed " + shown + ".", nil
}

// closable gives the epic's title and what to show you before closing it,
// if it can close: it asks to, or every story is done and you left it open.
func (c Client) closable(id string) (title, text string, ok bool) {
	if q, asking := c.questionOf(id); asking && slices.Contains(q.Options, waitlist.CloseEpic) {
		return q.TaskTitle, q.Text, true
	}
	var epic Task
	if err := c.Agentry.Call("GET", "/api/tasks/"+id, nil, &epic); err != nil || epic.Closing == "" {
		return "", "", false
	}
	return epic.Title, "Every story of it is done. Its summary:\n\n" + epic.Closing, true
}

// reviewApprovals asks you, in Claude Code, about each waiting action.
func (c Client) reviewApprovals(ctx context.Context, arguments json.RawMessage) (string, error) {
	var a struct{ Task string }
	json.Unmarshal(arguments, &a)
	a.Task = tasks.StoredID(a.Task)
	_, waiting, err := c.board()
	if err != nil {
		return "", err
	}
	var approvals []question
	for _, q := range waiting {
		if q.Kind == "approval" && (a.Task == "" || q.Task == a.Task) {
			approvals = append(approvals, q)
		}
	}
	if len(approvals) == 0 {
		return "No action is waiting for approval.", nil
	}
	ask := extensions.AskOf(ctx)
	if ask == nil {
		return fmt.Sprintf("%d actions wait for the user's approval, which is theirs to give. Give them this link: %s", len(approvals),
			c.Agentry.PageURL("/tasks?status=needs-you")), nil
	}
	var done []string
	for _, q := range approvals {
		answer, ok, err := ask(ctx, approvalText(q), schema.Object(schema.Props{
			"decision": schema.Enum("Let it run, or refuse it", "approve", "deny"),
			"reason":   schema.Text("Optional: why, for the agent"),
		}, "decision"))
		if err != nil {
			return "", err
		}
		if !ok {
			done = append(done, q.shown()+" "+q.Action+": left for later")
			break
		}
		verb, past := "approve", "approved"
		if answer["decision"] == "deny" {
			verb, past = "deny", "denied"
		}
		reason, _ := answer["reason"].(string)
		if err := c.Agentry.Call("POST", "/api/tasks/"+q.Task+"/actions/"+q.Action+"/"+verb, map[string]string{"reason": reason}, nil); err != nil {
			done = append(done, q.shown()+" "+q.Action+": "+err.Error())
			continue
		}
		done = append(done, q.shown()+" "+q.Action+": "+past+" by the user")
	}
	return strings.Join(done, "\n"), nil
}

// heldAtLayout shows when an action was held in local time, with its zone,
// since the coordinator's hand-back message gives the time in UTC.
const heldAtLayout = "Mon 2 Jan 15:04 MST"

// approvalText asks about one action. A held one was refused long ago, so
// approving it now means something else, and the branch may have moved.
func approvalText(q question) string {
	text := fmt.Sprintf("%s, on %s (%s), asks to run:\n\n%s: %s\n\nWhy it asks: %s", q.Agent, q.shown(), q.TaskTitle, q.Tool, q.Text, q.Reason)
	if q.Held {
		text += fmt.Sprintf("\n\nNo answer came in time: it was held at %s, and %s was told no and carried on. "+
			"If you approve, the coordinator hears of it and %s may run this exact command once, in its next turn. Check it still fits the branch.",
			q.HeldAt.Local().Format(heldAtLayout), q.Agent, q.Agent)
	}
	return text
}

func copies(q question, answer string) bool {
	return slices.Contains(q.Options, waitlist.CopyFile) && strings.EqualFold(strings.TrimSpace(answer), waitlist.CopyFile)
}

// approveCopy asks you, in Claude Code, whether to let the file be copied.
func (c Client) approveCopy(ctx context.Context, q question) (string, error) {
	ask := extensions.AskOf(ctx)
	if ask == nil {
		return "Copying a file from outside a machine's worktrees is the user's to allow. Give them this link: " + c.Agentry.PageURL("/tasks/"+q.Task), nil
	}
	answer, ok, err := ask(ctx, q.Text, schema.Object(schema.Props{"decision": schema.Enum("Copy it into the task's shared files, or not", "copy", "don't copy")}, "decision"))
	if err != nil {
		return "", err
	}
	if !ok || answer["decision"] != "copy" {
		return "The user didn't allow it. Send their reason, if they give one, with answer_question.", nil
	}
	if err := c.Agentry.Call("POST", "/api/tasks/"+q.Task+"/answer", map[string]string{"answer": waitlist.CopyFile}, nil); err != nil {
		return "", err
	}
	return "The user allowed it: the file is in " + q.shown() + "'s shared files.", nil
}

func closes(answer string) bool {
	return strings.EqualFold(strings.TrimSpace(answer), waitlist.CloseEpic)
}
