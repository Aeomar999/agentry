package tasks

import (
	"fmt"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

func (t *Task) applyActions(e events.Event) {
	switch e.Type {
	case events.ActionRequested:
		turn := t.OpenTurn()
		turn.Waiting, turn.RetriedAt = "", time.Time{} // working again after a retry
		t.addAction(&Action{ID: e.Correlation, Turn: turn.ID, Agent: e.Actor.ID,
			Tool: e.Text("tool"), Command: e.Text("command"), RequestedAt: e.At})
	case events.ActionAsked:
		action := t.Action(e.Correlation)
		action.Asked, action.Reason = true, e.Text("reason") // the rule that asked, until you decide
	case events.ActionApproved:
		t.decide(e, Approved)
	case events.ActionDenied:
		t.decide(e, Denied)
	case events.ActionHeld:
		action := t.Action(e.Correlation)
		action.Held, action.HeldAt = true, e.At
	case events.ActionResult:
		t.Action(e.Correlation).HasResult = true
	}
}

func (t *Task) decide(e events.Event, decision Decision) {
	action := t.Action(e.Correlation)
	action.Decision, action.Reason = decision, e.Text("reason")
	if pass := e.Text("pass"); pass != "" {
		t.Action(pass).PassUsed = true
	}
	if action.Held && decision == Approved {
		t.handBack(e, action)
	}
}

// handBack tells the coordinator you approved a held action: its agent was
// told no long ago, so it runs it again only when the coordinator hands it
// back. Deciding, the coordinator hears it once it has decided.
func (t *Task) handBack(e events.Event, action *Action) {
	text := approvedLateText(t, action, e.At)
	action.HandBack = len(t.Trail)
	if t.idle() {
		t.step(e, Agentry, t.Coordinator, text)
		return
	}
	t.Trail = append(t.Trail, Step{At: e.At, From: Agentry, To: t.Coordinator, Text: text})
	t.approvalNews = t.approvalNews || t.Handoff == nil
}

// approvedLateText is the hand-back: who asked for what, when, and that the
// command is to be run again only once the branch is checked.
func approvedLateText(t *Task, action *Action, approvedAt time.Time) string {
	when := "after its turn ended"
	if turn := t.Turn(action.Turn); turn != nil && !turn.Ended {
		when = "while its turn went on without it"
	}
	again, turn := "Hand the task back to "+action.Agent+" to run it again", action.Agent+"'s next turn"
	if action.Agent == t.Coordinator {
		again, turn = "Run it again yourself in this turn", "this turn"
	}
	return fmt.Sprintf("The user approved a command %s asked for in %s at %s (%s), which Agentry held for them when no answer "+
		"came in time. They approved it at %s, %s, so %s was told no and hasn't run it. The full command:\n\n%s\n\n"+
		"%s, after checking the branch hasn't changed in a way that makes it wrong. Agentry lets this exact command "+
		"through once without asking, in %s only.",
		action.Agent, action.Turn, clock(action.RequestedAt), action.Tool, clock(approvedAt), when, action.Agent,
		action.Command, again, turn)
}

// hearApprovals marks the held actions you approved as heard when the
// starting coordinator turn's brief, made from the first seen steps of the
// trail, has their hand-back. It says whether one is still unheard: you
// approved it after the brief was made.
func (t *Task) hearApprovals(seen int) (unheard bool) {
	for _, action := range t.Actions {
		if action.Held && action.Decision == Approved && !action.Heard {
			action.Heard = action.HandBack < seen
			unheard = unheard || !action.Heard
		}
	}
	return unheard
}

// startPasses gives the agent's held actions you approved, which its
// coordinator has heard and whose pass has no turn yet, this turn to work
// in: the only one their pass works in. A turn the coordinator gave the
// agent before it heard is for other work, and doesn't use the pass up.
func (t *Task) startPasses(turn, agent string) {
	for _, action := range t.Actions {
		if action.Held && action.Decision == Approved && action.Heard && action.PassTurn == "" && action.Agent == agent {
			action.PassTurn = turn
		}
	}
}

func clock(at time.Time) string { return at.UTC().Format("2006-01-02 15:04 UTC") }

// hearApprovalNews hands the task back to its coordinator when you approved
// a held action after its brief was made.
func (t *Task) hearApprovalNews(e events.Event) {
	if t.approvalNews {
		t.approvalNews = false
		t.step(e, Agentry, t.Coordinator, "The user approved a held command after your brief was made: see above. Decide again with it in mind.")
	}
}
