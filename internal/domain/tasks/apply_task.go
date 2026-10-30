package tasks

import "gitlab.com/dev.chempa/agentry/internal/core/events"

func (t *Task) applyTask(e events.Event) {
	switch e.Type {
	case events.TaskUnarchived:
		t.UnarchivedAt = e.At
	case events.TaskCreated:
		t.Title, t.Description, t.Repo = e.Text("title"), e.Text("description"), e.Text("repo")
		t.Labels = e.List("labels")
		t.Budget, t.MaxTurns = e.Number("budget"), DefaultMaxTurns
		if _, given := e.Data["maxTurns"]; given { // a task made before keeps the old limit
			t.MaxTurns = e.Number("maxTurns")
		}
		t.Rank = -float64(e.Position) // newest first until you reorder; positions never repeat
		if e.Text("epic") != "" {
			t.Rank = float64(e.Position) // a story: after its epic's others, so a split keeps its planned order
		}
		t.Coordinator = e.Text("coordinator")
		t.TeamChosen, t.Parent, t.Depth = e.Flag("teamChosen"), e.Text("parent"), e.Number("depth")
		t.Kind, t.KindChosen, t.Epic, t.After = Kind(e.Text("kind")), e.Flag("kindChosen"), e.Text("epic"), e.List("after")
		t.Parked = e.Text("parked") // an epic's story, or an idea you accepted, starts in your Backlog
		t.MadeFrom, t.ByAgentry = MadeFromOf(e.Proposal), e.Actor.Kind == events.System
		t.About, t.AboutClose = e.Text("about"), int64(e.Number("close"))
		t.step(e, You, t.Coordinator, firstNonEmpty(t.Description, t.Title))

	case events.TaskDirected:
		// Only noted in the trail: the work in hand goes on, and the
		// coordinator reads it at its next turn.
		// An epic between stories has nothing in hand, so its coordinator
		// takes it up at once.
		if t.Kind == Epic && t.idle() {
			t.step(e, You, t.Coordinator, e.Text("text"))
		} else {
			t.Trail = append(t.Trail, Step{At: e.At, From: You, To: t.Coordinator, Text: e.Text("text")})
		}
		t.moveColumn(e)

	case events.TaskBudgeted:
		t.Budget = e.Number("budget")

	case events.TaskTurnsLimited:
		t.MaxTurns = e.Number("maxTurns")

	case events.AgentRenamedInTask:
		// What the task does next goes to the agent by its new name; what
		// has happened keeps the name it had then.
		was, to := e.Text("name"), e.Text("to")
		if t.Coordinator == was {
			t.Coordinator = to
		}
		if t.Handoff != nil && t.Handoff.To == was {
			t.Handoff.To = to
		}

	case events.CoordinatorMoved:
		// Only noted in the trail, as what is in hand goes on: a step for
		// the coordinator before is the new one's to take.
		was := t.Coordinator
		t.Coordinator = e.Text("to")
		if t.Handoff != nil && t.Handoff.To == was {
			t.Handoff.To = t.Coordinator
		}
		t.Trail = append(t.Trail, Step{At: e.At, From: You, To: t.Coordinator,
			Text: "I made you your team's coordinator in place of " + was + ", so this task is yours from now on."})

	case events.TaskRanked:
		t.Rank = e.Real("rank")

	case events.TaskLabeled:
		t.Labels = e.List("labels")

	case events.TaskRenamed:
		if t.FirstTitle == "" {
			t.FirstTitle = t.Title
		}
		t.Title = e.Text("title")
	}
}
