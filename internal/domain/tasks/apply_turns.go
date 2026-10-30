package tasks

import (
	"cmp"
	"fmt"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
)

func (t *Task) applyTurns(e events.Event) {
	switch e.Type {
	case events.TurnStarted:
		t.Turns = append(t.Turns, &Turn{ID: e.Correlation, Agent: e.Text("agent"), Worker: cmp.Or(e.Text("worker"), workers.Local), Hash: e.Text("hash"),
			Engine: e.Text("engine"), FromSession: e.Text("fromSession"), SessionFrom: e.Text("sessionFrom"), Forked: e.Flag("forked"), FreshBecause: e.Text("freshBecause"), PlanFrom: e.Text("planFrom"), Seen: seenBy(e, len(t.Trail)), StartedAt: e.At, LastEventAt: e.At, Shipped: t.Handoff != nil && t.Handoff.Ships && t.Handoff.To == e.Text("agent")})
		if t.Handoff != nil {
			t.Turns[len(t.Turns)-1].Handoff = *t.Handoff
		}
		t.Handoff = nil
		t.TurnsSinceAnswer++
		coordinates, unheard := e.Text("agent") == t.Coordinator, false
		if coordinates {
			unheard = t.hearApprovals(seenBy(e, len(t.Trail)))
		}
		t.startPasses(e.Correlation, e.Text("agent"))
		if coordinates {
			// Its brief has every story's news so far, and your approvals,
			// but those that came after it was made.
			t.storyNews, t.approvalNews = false, unheard
			if t.WrapUp != nil {
				t.WrapUp.Turn = e.Correlation
			}
		}

	case events.PromptAcknowledged:
		turn := t.Turn(e.Correlation)
		turn.Acknowledged, turn.Waiting = true, ""
	case events.SessionStarted:
		t.Turn(e.Correlation).EngineSession = e.Text("engineSession")

	case events.PlanUpdated:
		t.Turn(e.Correlation).Plan = e.Text("plan")

	case events.AgentWaiting:
		t.Turn(e.Correlation).Waiting = e.Text("question")

	case events.TurnEnded:
		turn := t.Turn(e.Correlation)
		turn.Ended, turn.Report, turn.Tokens, turn.TokensUnknown = true, e.Text("report"), e.Number("tokens"), e.Text("tokensUnknown")
		t.moveRepo()
		turn.Split = Split{e.Number("inputTokens"), e.Number("cacheWriteTokens"), e.Number("cacheReadTokens"), e.Number("outputTokens")}
		turn.Context = e.Number("contextTokens")
		t.TokensUsed += turn.Tokens
		t.TokensSplit = t.TokensSplit.add(turn.Split)
		t.IssuesInARow = 0
		if turn.Agent == t.Coordinator {
			// An empty reply is still one to decide on: it has no decision
			// in it, so it goes back to the coordinator as an issue.
			t.PendingDecision = cmp.Or(turn.Report, "(the coordinator's turn ended with no reply)")
		} else {
			t.step(e, turn.Agent, t.Coordinator, turn.Report)
		}

	case events.TurnLimited:
		// Not an issue: nothing is wrong with the work, so it doesn't count
		// towards asking you, and the turn stays open to go on after the
		// reset. An old log's odd one (an ended turn, a time it can't read)
		// changes what it can and nothing else.
		if turn := t.Turn(e.Correlation); !turn.Ended {
			again := turn.Limited != "" // the engine said so twice: the new reset time stands, noted once
			turn.Limited = cmp.Or(e.Text("details"), e.Text("error"), "usage limit")
			turn.ResetsAt, _ = time.Parse(time.RFC3339, e.Text("resetsAt"))
			if again {
				break
			}
			// Only noted in the trail: the turn goes on after the reset.
			t.Trail = append(t.Trail, Step{At: e.At, From: Agentry, To: t.Coordinator,
				Text: turn.Agent + "'s " + turn.ID + " hit a " + LimitText(turn.ResetsAt, e.At)})
		}

	case events.TurnResumed:
		// An old log's odd one (an ended turn, one not limited, another
		// agent's) changes nothing.
		if turn := t.Turn(e.Correlation); !turn.Ended && turn.Limited != "" && e.Text("agent") == turn.Agent {
			turn.Limited, turn.ResetsAt = "", time.Time{}
			turn.Resumes, turn.ResumedAt = turn.Resumes+1, e.At
			if e.Text("reason") == "reset" {
				turn.RetriedAt = e.At
			}
			t.Trail = append(t.Trail, Step{At: e.At, From: Agentry, To: t.Coordinator,
				Text: turn.Agent + "'s " + turn.ID + " resumed after its usage limit"})
		}

	case events.IssueRaised:
		t.Fetching = nil // a fetch that failed is told as an issue
		turn := t.OpenTurn()
		if turn != nil {
			turn.Ended, turn.Issue = true, e.Text("reason")
			t.moveRepo()
		}
		t.IssuesInARow, t.LastIssue = t.IssuesInARow+1, e.Text("reason")
		if e.Flag("retryFresh") && turn != nil && turn.Handoff.To == turn.Agent {
			// Its resumed session didn't start, so nothing was done: the
			// same agent gets the same step again, and the coordinator is
			// only told. It still counts as an issue, so retries are bounded.
			t.Trail = append(t.Trail, Step{At: e.At, From: Agentry, To: t.Coordinator, Text: e.Text("reason")})
			again := turn.Handoff // its own At on purpose: the same step as in the trail, not a new one
			t.PendingDecision, t.Handoff = "", &again
			break
		}
		t.step(e, Agentry, t.Coordinator, e.Text("reason"))

	case events.TurnStopped:
		// Not an issue: nothing went wrong, so it doesn't count towards
		// asking you. What you said is just above it in the trail.
		turn := t.Turn(e.Correlation)
		t.stopTurn()
		t.step(e, Agentry, t.Coordinator, stoppedText(turn, t.Coordinator))
	}
}

// stopTurn ends the turn in progress, if there is one, as you stopped it.
func (t *Task) stopTurn() bool {
	turn := t.OpenTurn()
	if turn == nil {
		return false
	}
	turn.Ended, turn.Stopped = true, true
	t.moveRepo()
	return true
}

// seenBy is how many steps of the trail the turn's brief was made from. A
// turn started before turn.started said so saw all of it.
func seenBy(e events.Event, trail int) int {
	if _, given := e.Data["seen"]; given {
		return e.Number("seen")
	}
	return trail
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

// LimitText says when a usage limit hit at at resets, in the server's local
// time: "usage limit, resets ~03:00", with the date when that isn't the day
// it was hit, or "usage limit, reset time unknown".
func LimitText(resetsAt, at time.Time) string {
	if resetsAt.IsZero() {
		return "usage limit, reset time unknown"
	}
	resets, hit := resetsAt.Local(), at.Local()
	layout := "15:04"
	if resets.Format(time.DateOnly) != hit.Format(time.DateOnly) {
		layout = "Jan 2 15:04"
	}
	return "usage limit, resets ~" + resets.Format(layout)
}

func turnID(number int) string { return fmt.Sprintf("turn-%d", number) }
