package tasks

import (
	"cmp"
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

func (t *Task) applyEpics(e events.Event) {
	switch e.Type {
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
		t.Closing = "" // more to do first
		t.hearStoryNews(e)
		t.hearApprovalNews(e)
	case events.EpicWaits:
		waits := "Waiting for its stories."
		if t.Closing != "" {
			waits = "Every story is done. It stays open until you close it or add a story."
		}
		t.reply(e)
		t.step(e, t.Coordinator, You, cmp.Or(e.Text("note"), waits))
		t.hearStoryNews(e)
		t.hearApprovalNews(e)
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
		} else {
			t.Closing = "" // more to do first
		}
		t.Trail = append(t.Trail, Step{At: e.At, From: You, To: t.Coordinator, Text: text})
	case events.StoryRemoved:
		story := e.Text("story")
		t.Stories = slices.DeleteFunc(t.Stories, func(id string) bool { return id == story })
		t.StoriesDone = slices.DeleteFunc(t.StoriesDone, func(id string) bool { return id == story })
		if len(t.Stories) == 0 {
			t.Closing = "" // no stories left to have finished
		}
		t.Trail = append(t.Trail, Step{At: e.At, From: You, To: t.Coordinator, Text: "I took " + story + " out of this epic: it is no longer one of its stories."})
	case events.FollowUpsSkipped:
		t.Skipped = int64(e.Number("close"))
		t.Trail = append(t.Trail, Step{At: e.At, From: Agentry, To: You, Text: e.Text("reason")})
	case events.RequestAnswered:
		t.WaitingOn = ""
		t.step(e, e.Text("coordinator"), t.Coordinator, e.Text("child")+" is done: "+e.Text("summary"))
	}
}

// hearStoryNews hands the epic back to its coordinator when a story
// finished while it was deciding.
func (t *Task) hearStoryNews(e events.Event) {
	if t.storyNews {
		t.storyNews = false
		t.step(e, Agentry, t.Coordinator, "A story finished while you were deciding: see just above. Decide again with it in mind.")
	}
}
