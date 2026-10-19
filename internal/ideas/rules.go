package ideas

import (
	"cmp"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// checkIdea is the ideas' Streams.Check, check 3 for an idea's events: an idea is suggested once, from
// a task that exists, and you accept or dismiss it once, while it is open.
// Only you may do either, except that Agentry accepts an idea the
// Visionaries agreed on, by your rule, when you turned that on; the
// catalogue checks that no agent does.
func checkIdea(p events.Proposal, state features.Reader) *events.Rejection {
	idea, err := Get(state, p.Stream)
	if err != nil {
		return refuseFor("could not read the idea: " + err.Error())
	}
	reason := ""
	switch {
	case p.Type == IdeaSuggested && idea != nil:
		reason = "idea " + p.Stream + " already exists"
	case p.Type == IdeaSuggested:
		reason = aGoodSuggestion(p, state)
	case idea == nil:
		reason = "idea " + p.Stream + " doesn't exist"
	case idea.State != Open:
		reason = "idea " + p.Stream + " was " + string(idea.State) + " already"
	case p.Type == IdeaAccepted:
		reason = cmp.Or(madeFromIt(idea, p, state), agentryMayAccept(idea, p, state))
	}
	return refuseFor(reason)
}

// aGoodSuggestion has a title, a size, a kind of task, and comes from a task
// that exists; an agent suggests only from the task it is working on. The
// idea it says it repeats, if any, exists.
func aGoodSuggestion(p events.Proposal, state features.Reader) string {
	switch {
	case strings.TrimSpace(p.Text("title")) == "":
		return "an idea needs a title"
	case !slices.Contains(Sizes, p.Text("size")):
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
	return repeatsAnIdea(p, state)
}

// repeatsAnIdea: an idea said to repeat another names one that exists.
func repeatsAnIdea(p events.Proposal, state features.Reader) string {
	repeats := p.Text("repeats")
	if repeats == "" {
		return ""
	}
	original, err := Get(state, repeats)
	switch {
	case err != nil:
		return "could not read the idea it repeats: " + err.Error()
	case original == nil:
		return "the idea it repeats, " + repeats + ", doesn't exist"
	}
	return ""
}

// madeFromIt: an idea is accepted only once its task is made, so an accepted
// idea always points at a task.
func madeFromIt(idea *Idea, p events.Proposal, state features.Reader) string {
	task, err := state.Task(p.Text("task"))
	if err != nil {
		return "could not read the task: " + err.Error()
	}
	switch {
	case task == nil:
		return "task " + p.Text("task") + " doesn't exist"
	case task.MadeFrom != idea.ID:
		return "task " + task.ID + " wasn't made from " + idea.ID
	}
	return ""
}

// agentryMayAccept: Agentry accepts an idea for you only while you have that
// turned on, and only one the Visionaries agreed on by your rule that
// repeats no other idea (NotAutoAccepted), so the log can't hold any
// other, whoever proposes it.
func agentryMayAccept(idea *Idea, p events.Proposal, state features.Reader) string {
	now := state.Settings()
	switch {
	case p.Actor.Kind != events.System:
		return ""
	case !now.AutoAcceptIdeas:
		return "Agentry accepts an idea for you only when you turn that on"
	}
	if why := NotAutoAccepted(idea, List(state, idea.Repo), state.Roster(), now.AutoAcceptRule); why != "" {
		return "Agentry accepts only an idea the Visionaries agreed on, by your rule, that repeats no other, and " + idea.ID + " isn't: " + why
	}
	return ""
}

// guards are the ideas' checks on the core's types, for what Agentry does
// with an idea it accepts for you. The core refuses Agentry either without
// them, so a build without ideas makes and starts no such task.
var guards = map[events.Type]features.Rule{events.TaskCreated: forAnOpenIdea, events.TaskHeld: startingAnAcceptedIdea}

// forAnOpenIdea: besides a task about an epic you closed, Agentry makes one
// only from an idea that is still open, to accept it for you, so one it may
// accept, and only in your Backlog: it starts one only by moving it, while
// you have that turned on (startingAnAcceptedIdea). It checks only such a
// task made from an idea; the core refuses one made from no stream.
func forAnOpenIdea(p events.Proposal, state features.Reader) *events.Rejection {
	from := tasks.MadeFromOf(p)
	if p.Actor.Kind != events.System || p.Text("about") != "" || !IsStream(from) {
		return nil
	}
	idea, err := Get(state, from)
	reason := ""
	switch {
	case err != nil:
		reason = "could not read the idea: " + err.Error()
	case idea == nil:
		reason = "idea " + from + " doesn't exist"
	case idea.State != Open:
		reason = "idea " + idea.ID + " was " + string(idea.State) + " already"
	case p.Text("parked") != "backlog":
		reason = "Agentry makes a task from an idea only in your Backlog"
	default:
		reason = agentryMayAccept(idea, p, state)
	}
	return refuseFor(reason)
}

// startingAnAcceptedIdea: Agentry moves a task on the board only to start
// one it made from an idea it accepted for you, while you have starting
// them turned on: once, out of the Backlog into In progress, before it has
// run. A task you parked again, or one you made, stays where you put it. It
// checks only Agentry's moves of a task made from an idea; the core refuses
// the rest.
func startingAnAcceptedIdea(p events.Proposal, state features.Reader) *events.Rejection {
	if p.Actor.Kind != events.System {
		return nil
	}
	task, err := state.Task(p.Stream)
	switch {
	case err != nil:
		return refuseFor("could not read the task: " + err.Error())
	case task == nil || !IsStream(task.MadeFrom):
		return nil
	}
	return refuseFor(startingIt(task, p, state))
}

// startingIt is why Agentry may not start task, made from an idea, or "".
func startingIt(task *tasks.Task, p events.Proposal, state features.Reader) string {
	const only = "Agentry only starts, once, a task it made from an idea it accepted"
	if now := state.Settings(); !now.AutoAcceptIdeas || !now.AutoStartIdeas {
		return "Agentry starts a task from an idea it accepted only when you turn that on"
	}
	if p.Text("place") != "" || p.Text("column") != "in-progress" || p.Flag("sentBack") ||
		task.Parked == "" || len(task.Turns) > 0 || task.AutoStarted || !task.ByAgentry {
		return only
	}
	idea, err := Get(state, task.MadeFrom)
	switch {
	case err != nil:
		return "could not read the idea: " + err.Error()
	case idea == nil || !idea.AutoAccepted || idea.Became != task.ID:
		return only
	}
	return ""
}

// refuseFor refuses for reason, as the core's rules do, or lets the
// proposal through when there is none.
func refuseFor(reason string) *events.Rejection {
	if reason == "" {
		return nil
	}
	return &events.Rejection{Check: "order", Reason: reason}
}
