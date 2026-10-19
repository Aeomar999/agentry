package rules

import (
	"cmp"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// checkIdea is check 3 for an idea's events: an idea is suggested once, from
// a task that exists, and you accept or dismiss it once, while it is open.
// Only you may do either, except that Agentry accepts an idea every
// Visionary agreed on when you turned that on; the catalogue checks that
// no agent does.
func checkIdea(p events.Proposal, state State) *events.Rejection {
	idea, err := state.Idea(p.Stream)
	if err != nil {
		return refuse("could not read the idea: " + err.Error())
	}
	reason := ""
	switch {
	case p.Type == events.IdeaSuggested && idea != nil:
		reason = "idea " + p.Stream + " already exists"
	case p.Type == events.IdeaSuggested:
		reason = aGoodSuggestion(p, state)
	case idea == nil:
		reason = "idea " + p.Stream + " doesn't exist"
	case idea.State != ideas.Open:
		reason = "idea " + p.Stream + " was " + string(idea.State) + " already"
	case p.Type == events.IdeaAccepted:
		reason = cmp.Or(madeFromIt(idea, p, state), agentryMayAccept(idea, p, state))
	}
	if reason != "" {
		return refuse(reason)
	}
	return nil
}

// aGoodSuggestion has a title, a size, a kind of task, and comes from a task
// that exists; an agent suggests only from the task it is working on.
func aGoodSuggestion(p events.Proposal, state State) string {
	switch {
	case strings.TrimSpace(p.Text("title")) == "":
		return "an idea needs a title"
	case !slices.Contains(ideas.Sizes, p.Text("size")):
		return "an idea's size is S, M or L, not " + p.Text("size")
	}
	if kind := tasks.Kind(p.Text("kind")); kind != tasks.Story && kind != tasks.Epic {
		return `an idea becomes a story ("") or an epic ("epic"), not ` + string(kind)
	}
	from, err := state.Task(p.Text("task"))
	if err != nil {
		return "could not read the task: " + err.Error()
	}
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
func madeFromIt(idea *ideas.Idea, p events.Proposal, state State) string {
	task, err := state.Task(p.Text("task"))
	if err != nil {
		return "could not read the task: " + err.Error()
	}
	switch {
	case task == nil:
		return "task " + p.Text("task") + " doesn't exist"
	case task.Idea != idea.ID:
		return "task " + task.ID + " wasn't made from " + idea.ID
	}
	return ""
}

// agentryMayAccept: Agentry accepts an idea for you only while you have that
// turned on, and only one every Visionary agreed on, so the log can't hold
// any other, whoever proposes it.
func agentryMayAccept(idea *ideas.Idea, p events.Proposal, state State) string {
	switch {
	case p.Actor.Kind != events.System:
		return ""
	case !state.Settings().AutoAcceptIdeas:
		return "Agentry accepts an idea for you only when you turn that on"
	case !ideas.Agreed(idea, state.Roster(), ideas.VisionariesTeam):
		return "Agentry accepts only an idea every Visionary agreed on"
	}
	return ""
}

// forAnOpenIdea: besides a task about an epic you closed, Agentry makes one
// only from an idea that is still open, to accept it for you, so one it may
// accept.
func forAnOpenIdea(p events.Proposal, state State) string {
	const only = "Agentry makes a task only about an epic you closed, or from an idea every Visionary agreed on"
	if p.Text("idea") == "" {
		return only
	}
	idea, err := state.Idea(p.Text("idea"))
	switch {
	case err != nil:
		return "could not read the idea: " + err.Error()
	case idea == nil:
		return "idea " + p.Text("idea") + " doesn't exist"
	case idea.State != ideas.Open:
		return "idea " + idea.ID + " was " + string(idea.State) + " already"
	}
	return agentryMayAccept(idea, p, state)
}

// startingAnAcceptedIdea: Agentry moves a task on the board only to start
// one it made from an idea it accepted for you, while you have starting
// them turned on: once, out of the Backlog into In progress, before it has
// run. A task you parked again, or one you made, stays where you put it.
func startingAnAcceptedIdea(task *tasks.Task, p events.Proposal, state State) string {
	const only = "Agentry only starts, once, a task it made from an idea it accepted"
	if now := state.Settings(); !now.AutoAcceptIdeas || !now.AutoStartIdeas {
		return "Agentry starts a task from an idea it accepted only when you turn that on"
	}
	if p.Text("place") != "" || p.Text("column") != "in-progress" || p.Flag("sentBack") ||
		task.Parked == "" || len(task.Turns) > 0 || task.AutoStarted || !task.ByAgentry || task.Idea == "" {
		return only
	}
	idea, err := state.Idea(task.Idea)
	switch {
	case err != nil:
		return "could not read the idea: " + err.Error()
	case idea == nil || !idea.AutoAccepted || idea.Became != task.ID:
		return only
	}
	return ""
}
