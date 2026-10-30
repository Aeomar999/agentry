package tasks

import (
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

func (t *Task) applyBoard(e events.Event) {
	switch e.Type {
	case events.TaskHeld:
		// Your move stands at once: a turn in progress stops, and the
		// coordinator wraps up. Moved on again, it carries on from there.
		place := e.Text("place")
		switch {
		case place != "" && t.Parked == "":
			t.Parked, t.Column = place, ""
			t.wrapUp(e, place, "I moved this task to "+PlaceName(place)+": it waits there until I move it on.")
		case place != "":
			t.Parked = place
		default: // a wrap-up still going is over: you want more work
			wrapping := t.WrapUp != nil
			t.Parked, t.WrapUp = "", nil
			t.AutoStarted = t.AutoStarted || e.Actor.Kind == events.System
			t.moveColumn(e)
			if t.stopTurn() || wrapping || t.idle() {
				t.step(e, You, t.Coordinator, "I moved this task back to work. Carry on from where it was.")
			}
		}

	case events.TaskWrappedUp:
		t.WrapUp, t.PendingDecision, t.Handoff, t.wrappedUp = nil, "", nil, len(t.Turns)
		t.reply(e)
		text := e.Text("summary")
		if e.Actor.Kind == events.Agent && t.IsDone {
			t.Summary = text
		}
		if text == "" {
			text = "Wrapped up without a summary: " + e.Text("reason")
		}
		t.Told = Step{At: e.At, From: e.Actor.ID, To: You, Text: text}
		t.Trail = append(t.Trail, t.Told)

	case events.TaskReopened:
		// Back to work in a fresh worktree (the loop makes it again), with a
		// new count of turns and issues, from what you said. Reopened while
		// its coordinator wraps up, the wrap-up turn stops, and the worktree,
		// not removed yet, is worked in again.
		if t.WrapUp != nil {
			t.stopTurn()
		} else {
			t.Worktree = nil
		}
		t.IsDone, t.DoneAt, t.DoneByYou, t.Summary, t.WrapUp = false, time.Time{}, false, "", nil
		t.Question, t.Asked, t.TurnsSinceAnswer, t.IssuesInARow = "", Asked{}, 0, 0
		t.TurnsBeforeReopen = len(t.Turns)
		t.step(e, You, t.Coordinator, e.Text("text"))
		t.moveColumn(e)
	}
}

// wrapUp makes your move to Done or a parking place stand at once: the turn
// in progress stops, and the coordinator has one last turn to wrap up, if
// anyone has worked on the task since it last wrapped up.
func (t *Task) wrapUp(e events.Event, to, text string) {
	if note := e.Text("note"); note != "" {
		text += " " + note
	}
	nothingNew := len(t.Turns) == t.wrappedUp // an open turn counts: it is stopped and wrapped up
	switch {
	case t.WrapUp != nil:
		// Parked, and moved to Done while it wraps up: the same turn now
		// wraps up a done task. (Done and then parked is reopened first,
		// which ends the wrap-up, so parking it owes a fresh one.)
		t.WrapUp.To = to
		t.Trail = append(t.Trail, Step{At: e.At, From: You, To: t.Coordinator, Text: text})
	case nothingNew && to == "done": // nothing to wrap up
		t.Trail = append(t.Trail, Step{At: e.At, From: You, To: t.Coordinator, Text: text})
		t.PendingDecision, t.Handoff = "", nil
	case nothingNew: // parked before it started, or since it wrapped up: it starts as it would have
	default:
		t.stopTurn()
		t.WrapUp = &WrapUp{To: to}
		t.step(e, You, t.Coordinator, text)
	}
}

// moveColumn records your move of the card to In progress, Review or
// Shipping, which the first event the move writes carries: the column stands
// at once.
func (t *Task) moveColumn(e events.Event) {
	if column := e.Text("column"); column != "" {
		t.Column, t.SentBack = column, e.Flag("sentBack")
	}
}
