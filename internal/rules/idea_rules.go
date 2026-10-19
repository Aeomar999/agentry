package rules

import (
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// checkIdea is check 3 for an idea's events: an idea is suggested once, from
// a task that exists, and you accept or dismiss it once, while it is open.
// Only you may do either; the catalogue checks that.
func checkIdea(p events.Proposal, history store.History) *events.Rejection {
	stream, err := history.Stream(p.Stream)
	if err != nil {
		return refuse("could not read the idea: " + err.Error())
	}
	idea := ideas.FromEvents(stream)
	reason := ""
	switch {
	case p.Type == events.IdeaSuggested && idea != nil:
		reason = "idea " + p.Stream + " already exists"
	case p.Type == events.IdeaSuggested:
		reason = aGoodSuggestion(p, history)
	case idea == nil:
		reason = "idea " + p.Stream + " doesn't exist"
	case idea.State != ideas.Open:
		reason = "idea " + p.Stream + " was " + string(idea.State) + " already"
	case p.Type == events.IdeaAccepted:
		reason = madeFromIt(idea, p, history)
	}
	if reason != "" {
		return refuse(reason)
	}
	return nil
}

// aGoodSuggestion has a title, a size, a kind of task, and comes from a task
// that exists; an agent suggests only from the task it is working on.
func aGoodSuggestion(p events.Proposal, history store.History) string {
	switch {
	case strings.TrimSpace(p.Text("title")) == "":
		return "an idea needs a title"
	case !slices.Contains(ideas.Sizes, p.Text("size")):
		return "an idea's size is S, M or L, not " + p.Text("size")
	}
	if kind := tasks.Kind(p.Text("kind")); kind != tasks.Story && kind != tasks.Epic {
		return `an idea becomes a story ("") or an epic ("epic"), not ` + string(kind)
	}
	stream, err := history.Stream(p.Text("task"))
	if err != nil {
		return "could not read the task: " + err.Error()
	}
	from := tasks.FromEvents(stream)
	switch {
	case from == nil:
		return "task " + p.Text("task") + " doesn't exist"
	case p.Actor.Kind == events.Agent && from.CurrentAgent() != p.Actor.ID:
		return p.Actor.ID + " is not working on " + from.ID
	}
	return ""
}

// madeFromIt: an idea is accepted only once its task is made, so an accepted
// idea always points at a task.
func madeFromIt(idea *ideas.Idea, p events.Proposal, history store.History) string {
	stream, err := history.Stream(p.Text("task"))
	if err != nil {
		return "could not read the task: " + err.Error()
	}
	switch task := tasks.FromEvents(stream); {
	case task == nil:
		return "task " + p.Text("task") + " doesn't exist"
	case task.Idea != idea.ID:
		return "task " + task.ID + " wasn't made from " + idea.ID
	}
	return ""
}
