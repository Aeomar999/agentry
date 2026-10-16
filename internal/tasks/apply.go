package tasks

import (
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// FromEvents builds a task by applying its accepted events in order.
// It returns nil if the events don't include task.created.
func FromEvents(stream []events.Event) *Task {
	var t *Task
	for _, e := range stream {
		if e.Type == events.TaskCreated {
			t = &Task{ID: e.Stream, CreatedAt: e.At}
		}
		if t != nil {
			t.apply(e)
		}
	}
	return t
}

func (t *Task) apply(e events.Event) {
	t.LastActiveAt = e.At
	if turn := t.Turn(e.Correlation); turn != nil {
		turn.LastEventAt = e.At
	}
	if action := t.Action(e.Correlation); action != nil {
		if turn := t.Turn(action.Turn); turn != nil {
			turn.LastEventAt = e.At
		}
	}

	switch e.Type {
	case events.TaskCreated:
		t.Title, t.Description, t.Repo = e.Text("title"), e.Text("description"), e.Text("repo")
		t.Labels = e.List("labels")
		t.Budget = e.Number("budget")
		t.Rank = -float64(e.Position) // newest first until you reorder; positions never repeat
		t.Coordinator = e.Text("coordinator")
		t.step(e, You, t.Coordinator, firstNonEmpty(t.Description, t.Title))

	case events.TaskDirected:
		// Only noted in the trail: the work in hand goes on, and the
		// coordinator reads it at its next turn.
		t.Trail = append(t.Trail, Step{At: e.At, From: You, To: t.Coordinator, Text: e.Text("text")})

	case events.TaskBudgeted:
		t.Budget = e.Number("budget")

	case events.TaskRanked:
		t.Rank = e.Real("rank")

	case events.TaskLabeled:
		t.Labels = e.List("labels")

	case events.TaskRenamed:
		if t.FirstTitle == "" {
			t.FirstTitle = t.Title
		}
		t.Title = e.Text("title")

	case events.WorktreeReady:
		t.Worktree = &Worktree{Repo: e.Text("repo"), Path: e.Text("path"), Branch: e.Text("branch"), Base: e.Text("base"), Port: e.Number("port")}

	case events.TaskAssigned:
		t.step(e, e.Actor.ID, e.Text("agent"), e.Text("instructions"))

	case events.TurnStarted:
		t.Turns = append(t.Turns, &Turn{ID: e.Correlation, Agent: e.Text("agent"), Hash: e.Text("hash"),
			StartedAt: e.At, LastEventAt: e.At})
		t.Handoff = nil
		t.TurnsSinceAnswer++

	case events.PromptAcknowledged:
		turn := t.Turn(e.Correlation)
		turn.Acknowledged, turn.Waiting = true, ""

	case events.ActionRequested:
		turn := t.OpenTurn()
		turn.Waiting = ""
		t.Actions = append(t.Actions, &Action{ID: e.Correlation, Turn: turn.ID, Agent: e.Actor.ID,
			Tool: e.Text("tool"), Command: e.Text("command"), RequestedAt: e.At})
	case events.ActionAsked:
		action := t.Action(e.Correlation)
		action.Asked, action.Reason = true, e.Text("reason") // the rule that asked, until you decide
	case events.ActionApproved:
		t.decide(e, Approved)
	case events.ActionDenied:
		t.decide(e, Denied)
	case events.ActionResult:
		t.Action(e.Correlation).HasResult = true

	case events.AgentWaiting:
		t.Turn(e.Correlation).Waiting = e.Text("question")

	case events.TurnEnded:
		turn := t.Turn(e.Correlation)
		turn.Ended, turn.Report, turn.Tokens = true, e.Text("report"), e.Number("tokens")
		t.TokensUsed += turn.Tokens
		t.IssuesInARow = 0
		if turn.Agent == t.Coordinator {
			t.PendingDecision = turn.Report
		} else {
			t.step(e, turn.Agent, t.Coordinator, turn.Report)
		}

	case events.IssueRaised:
		if turn := t.OpenTurn(); turn != nil {
			turn.Ended, turn.Issue = true, e.Text("reason")
		}
		t.IssuesInARow++
		t.step(e, Agentry, t.Coordinator, e.Text("reason"))

	case events.TaskNeedsYou:
		t.Question = e.Text("question")
		t.Asked = Asked{By: e.Actor.ID, At: e.At, Options: e.List("options"), Page: e.Text("page")}
		t.step(e, e.Actor.ID, You, t.Question)
	case events.TaskAnswered:
		t.Question, t.Asked, t.TurnsSinceAnswer, t.IssuesInARow = "", Asked{}, 0, 0
		t.step(e, You, t.Coordinator, e.Text("answer"))

	case events.TaskDone:
		t.IsDone, t.Summary = true, e.Text("summary")
		t.step(e, e.Actor.ID, You, t.Summary)
	}
}

func (t *Task) decide(e events.Event, decision Decision) {
	action := t.Action(e.Correlation)
	action.Decision, action.Reason = decision, e.Text("reason")
}

// step adds a line to the trail. A step to an agent is also the task's
// pending handoff, and any step settles a pending decision.
func (t *Task) step(e events.Event, from, to, text string) {
	step := Step{At: e.At, From: from, To: to, Text: text}
	t.Trail = append(t.Trail, step)
	t.PendingDecision, t.Handoff = "", nil
	if to != You {
		t.Handoff = &step
	}
}

func turnID(number int) string { return fmt.Sprintf("turn-%d", number) }

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
