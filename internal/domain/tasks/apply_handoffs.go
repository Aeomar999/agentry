package tasks

import (
	"cmp"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

func (t *Task) applyHandoffs(e events.Event) {
	switch e.Type {
	case events.TaskAssigned:
		// Its coordinator's handoff takes over the column you moved it to,
		// except a builder taking a task you put in Review, which a team
		// with no checker can do: it stays there. Moved to Shipping, the
		// handoff to an agent that ships work is the one your move asked for.
		ships := t.Column == "shipping" && e.Flag("ships")
		if t.Column != "review" || e.Flag("checks") {
			t.Column, t.SentBack = "", false
		}
		t.reply(e)
		t.step(e, e.Actor.ID, e.Text("agent"), e.Text("instructions"))
		t.Handoff.Machine, t.Handoff.Ships = e.Text("machine"), ships
		t.Trail[len(t.Trail)-1].Machine, t.Trail[len(t.Trail)-1].Ships = t.Handoff.Machine, ships

	case events.TaskNeedsYou:
		t.Question = e.Text("question")
		t.Asked = Asked{By: e.Actor.ID, At: e.At, Options: e.List("options"), Page: e.Text("page"), Limit: e.Text("limit")}
		if e.Text("fetchPath") != "" {
			t.Asked.Fetch = &Fetch{Machine: e.Text("fetchMachine"), Path: e.Text("fetchPath"), As: e.Text("fetchAs")}
			t.Fetching = nil // it waits for you now
		}
		t.Closing = cmp.Or(e.Text("closing"), t.Closing)
		t.reply(e)
		t.step(e, e.Actor.ID, You, t.Question)
	case events.TaskAnswered:
		// Closing stays: an epic you leave open can still be closed later.
		t.Question, t.Asked, t.TurnsSinceAnswer, t.IssuesInARow = "", Asked{}, 0, 0
		t.step(e, You, t.Coordinator, e.Text("answer"))
		t.moveColumn(e)

	case events.TaskDone:
		t.IsDone, t.DoneAt, t.Summary = true, e.At, e.Text("summary")
		t.Question, t.Asked, t.Closing = "", Asked{}, "" // an epic you closed: that was your answer
		t.Column, t.SentBack = "", false
		// You moved it to Done on the board: it is done, whatever was in hand.
		if e.Flag("moved") {
			t.DoneByYou, t.Parked, t.WaitingOn = true, "", ""
			t.wrapUp(e, "done", "I moved this task to Done: it is finished as it is.")
			return
		}
		t.reply(e)
		t.step(e, e.Actor.ID, You, t.Summary)
	}
}
