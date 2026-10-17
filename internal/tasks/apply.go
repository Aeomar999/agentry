package tasks

import (
	"cmp"
	"fmt"
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/workers"
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
		if e.Text("epic") != "" {
			t.Rank = float64(e.Position) // a story: after its epic's others, so a split keeps its planned order
		}
		t.Coordinator = e.Text("coordinator")
		t.TeamChosen, t.Parent, t.Depth = e.Flag("teamChosen"), e.Text("parent"), e.Number("depth")
		t.Kind, t.KindChosen, t.Epic, t.After = Kind(e.Text("kind")), e.Flag("kindChosen"), e.Text("epic"), e.List("after")
		t.Parked = e.Text("parked") // an epic's story starts in your Backlog
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
		t.FinishAsIs = t.FinishAsIs || e.Flag("asIs")

	case events.TaskBudgeted:
		t.Budget = e.Number("budget")

	case events.TaskHeld:
		t.Parked = e.Text("place")

	case events.TaskReopened:
		// Back to work in a fresh worktree (the loop makes it again), with a
		// new count of turns and issues, from what you said.
		t.IsDone, t.Summary, t.Worktree, t.FinishAsIs = false, "", nil, false
		t.Question, t.Asked, t.TurnsSinceAnswer, t.IssuesInARow = "", Asked{}, 0, 0
		t.step(e, You, t.Coordinator, e.Text("text"))

	case events.RepoChanged:
		// Its worktree is made again, of the new repo, before the next turn:
		// at once between turns, or when the turn in progress ends.
		repo, when := e.Text("repo"), " (it moves when the turn in progress ends)"
		t.MovingTo = repo
		if t.OpenTurn() == nil {
			t.moveRepo()
			when = ""
		}
		t.step(e, You, t.Coordinator, "I moved this task to the repo at "+repo+when+". From now on its work goes there: "+
			"the task has a new worktree of that repo, on its own branch, and every agent works in it.")

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

	case events.TaskTransferred:
		// Theirs from now on; the coordinator before passes it with why.
		was := t.Coordinator
		t.Coordinator, t.TeamChosen = e.Text("coordinator"), true
		t.step(e, was, t.Coordinator, "This task is your team's work: "+e.Text("reason"))

	case events.TaskRequested:
		// Not a handoff in this task: the other team works on its own task,
		// and this one waits for it.
		t.WaitingOn = e.Text("child")
		t.Trail = append(t.Trail, Step{At: e.At, From: t.Coordinator, To: e.Text("coordinator"),
			Text: "Asked the " + e.Text("team") + " team, as " + t.WaitingOn + ": " + e.Text("request")})
		t.PendingDecision, t.Handoff = "", nil

	case events.TaskSplit:
		t.Kind = Epic
		t.Stories = append(t.Stories, e.List("stories")...)
		t.reply(e)
		t.step(e, t.Coordinator, You, "Split into stories: "+e.Text("titles"))
		t.Told = Step{At: e.At, From: t.Coordinator, To: You,
			Text: "Split into stories: " + e.Text("titles") + ". They wait in your Backlog until you start them."}
		t.hearStoryNews(e)
	case events.EpicWaits:
		t.reply(e)
		t.step(e, t.Coordinator, You, cmp.Or(e.Text("note"), "Waiting for its stories."))
		t.hearStoryNews(e)
	case events.StoryDone:
		t.StoriesDone = append(t.StoriesDone, e.Text("story"))
		text := e.Text("story") + " is done: " + e.Text("summary")
		if t.idle() {
			t.step(e, e.Text("coordinator"), t.Coordinator, text)
		} else {
			// The coordinator is deciding: it hears this once it has.
			t.Trail = append(t.Trail, Step{At: e.At, From: e.Text("coordinator"), To: t.Coordinator, Text: text})
			t.storyNews = t.Handoff == nil
		}
	case events.EpicJoined:
		t.Epic = e.Text("epic")
	case events.EpicLeft:
		t.Epic, t.After = "", nil // what it built on were stories of that epic
	case events.StoryAdded:
		// Noted for the coordinator, which reads it at its next turn; the
		// work in hand goes on.
		story := e.Text("story")
		t.Stories = append(t.Stories, story)
		text := "I added " + story + " to this epic, as a story: " + e.Text("title")
		if e.Flag("done") {
			t.StoriesDone = append(t.StoriesDone, story)
			text += ". It is done already: " + e.Text("summary")
		}
		t.Trail = append(t.Trail, Step{At: e.At, From: You, To: t.Coordinator, Text: text})
	case events.StoryRemoved:
		story := e.Text("story")
		t.Stories = slices.DeleteFunc(t.Stories, func(id string) bool { return id == story })
		t.StoriesDone = slices.DeleteFunc(t.StoriesDone, func(id string) bool { return id == story })
		t.Trail = append(t.Trail, Step{At: e.At, From: You, To: t.Coordinator, Text: "I took " + story + " out of this epic: it is no longer one of its stories."})
	case events.RequestAnswered:
		t.WaitingOn = ""
		t.step(e, e.Text("coordinator"), t.Coordinator, e.Text("child")+" is done: "+e.Text("summary"))

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
		t.Worktree = &Worktree{Repo: e.Text("repo"), Worker: cmp.Or(e.Text("worker"), workers.Local), Path: e.Text("path"),
			Branch: e.Text("branch"), Base: e.Text("base"), Port: e.Number("port")}

	case events.WorktreeMoved:
		t.Worktree.Worker, t.Worktree.Path, t.Worktree.Port = e.Text("worker"), e.Text("path"), e.Number("port")

	case events.TaskAssigned:
		t.reply(e)
		t.step(e, e.Actor.ID, e.Text("agent"), e.Text("instructions"))
		t.Handoff.Machine = e.Text("machine")
		t.Trail[len(t.Trail)-1].Machine = t.Handoff.Machine

	case events.TurnStarted:
		t.Turns = append(t.Turns, &Turn{ID: e.Correlation, Agent: e.Text("agent"), Worker: cmp.Or(e.Text("worker"), workers.Local), Hash: e.Text("hash"),
			PlanFrom: e.Text("planFrom"), StartedAt: e.At, LastEventAt: e.At})
		t.Handoff = nil
		t.TurnsSinceAnswer++
		if e.Text("agent") == t.Coordinator {
			t.storyNews = false // its brief has every story's news so far
		}

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

	case events.PlanUpdated:
		t.Turn(e.Correlation).Plan = e.Text("plan")

	case events.AgentWaiting:
		t.Turn(e.Correlation).Waiting = e.Text("question")

	case events.TurnEnded:
		turn := t.Turn(e.Correlation)
		turn.Ended, turn.Report, turn.Tokens = true, e.Text("report"), e.Number("tokens")
		t.moveRepo()
		t.TokensUsed += turn.Tokens
		t.IssuesInARow = 0
		if turn.Agent == t.Coordinator {
			// An empty reply is still one to decide on: it has no decision
			// in it, so it goes back to the coordinator as an issue.
			t.PendingDecision = cmp.Or(turn.Report, "(the coordinator's turn ended with no reply)")
		} else {
			t.step(e, turn.Agent, t.Coordinator, turn.Report)
		}

	case events.IssueRaised:
		if turn := t.OpenTurn(); turn != nil {
			turn.Ended, turn.Issue = true, e.Text("reason")
			t.moveRepo()
		}
		t.IssuesInARow++
		t.step(e, Agentry, t.Coordinator, e.Text("reason"))

	case events.TurnStopped:
		// Not an issue: nothing went wrong, so it doesn't count towards
		// asking you. What you said is just above it in the trail.
		turn := t.Turn(e.Correlation)
		turn.Ended, turn.Stopped = true, true
		t.moveRepo()
		t.step(e, Agentry, t.Coordinator, stoppedText(turn, t.Coordinator))

	case events.TaskNeedsYou:
		t.Question = e.Text("question")
		t.Asked = Asked{By: e.Actor.ID, At: e.At, Options: e.List("options"), Page: e.Text("page")}
		t.Closing = e.Text("closing")
		t.reply(e)
		t.step(e, e.Actor.ID, You, t.Question)
	case events.TaskAnswered:
		t.Question, t.Asked, t.TurnsSinceAnswer, t.IssuesInARow, t.Closing = "", Asked{}, 0, 0, ""
		t.FinishAsIs = t.FinishAsIs || e.Flag("asIs")
		t.step(e, You, t.Coordinator, e.Text("answer"))

	case events.TaskDone:
		t.IsDone, t.Summary = true, e.Text("summary")
		t.Question, t.Asked, t.Closing = "", Asked{}, "" // an epic you closed: that was your answer
		t.reply(e)
		t.step(e, e.Actor.ID, You, t.Summary)
	}
}

func (t *Task) decide(e events.Event, decision Decision) {
	action := t.Action(e.Correlation)
	action.Decision, action.Reason = decision, e.Text("reason")
}

// step adds a line to the trail. A step to an agent is also the task's
// pending handoff, and any step settles a pending decision.
// hearStoryNews hands the epic back to its coordinator when a story
// finished while it was deciding.
func (t *Task) hearStoryNews(e events.Event) {
	if t.storyNews {
		t.storyNews = false
		t.step(e, Agentry, t.Coordinator, "A story finished while you were deciding: see just above. Decide again with it in mind.")
	}
}

// idle is a task with nothing in hand: no turn, no decision to apply and
// no one about to start.
func (t *Task) idle() bool {
	return t.OpenTurn() == nil && t.PendingDecision == "" && t.Handoff == nil
}

func (t *Task) step(e events.Event, from, to, text string) {
	step := Step{At: e.At, From: from, To: to, Text: text}
	t.Trail = append(t.Trail, step)
	t.PendingDecision, t.Handoff = "", nil
	if to != You {
		t.Handoff = &step
	}
}

// reply adds the coordinator's answer to what you told it, which any
// decision may carry, to the trail before the decision itself.
func (t *Task) reply(e events.Event) {
	if text := e.Text("toYou"); text != "" {
		t.Told = Step{At: e.At, From: e.Actor.ID, To: You, Text: text}
		t.Trail = append(t.Trail, t.Told)
	}
}

// stoppedText tells the coordinator a turn was stopped for it to hear you.
func stoppedText(turn *Turn, coordinator string) string {
	if turn.Agent == coordinator {
		return "The user stopped your " + turn.ID + " to tell you what is just above. Decide again with it in mind."
	}
	return "The user stopped " + turn.Agent + "'s " + turn.ID + " to tell you what is just above. " + turn.Agent +
		" wrote no report, but its work so far is in the worktree, and its unfinished plan carries over to its next turn. " +
		"Unless what they said changes the direction, hand the task back to " + turn.Agent + " to carry on, with its " +
		"instructions brought up to date for what they said."
}

// moveRepo makes the move you asked for: the next turn works in a new
// worktree, of the new repo.
func (t *Task) moveRepo() {
	if t.MovingTo != "" {
		t.Repo, t.Worktree, t.MovingTo = t.MovingTo, nil, ""
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
