package ideas

import (
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// Extension is Ideas as an extension. It is enabled while some team's coordinator
// suggests ideas (Suggests), and visible while it is enabled or any idea, of
// any state, is left: your open ideas still need an answer and the rest are
// their history. Its badge is the number of open ideas. Those coordinators'
// done decisions carry the ideas Agentry keeps (coordinator.go). Of the
// core's types it writes the task an accepted idea becomes (makeTask), and
// the task.held that starts one Agentry accepted (startIdea), and its Guards
// check both when Agentry proposes them (rules.go). In agentry mcp
// it brings its tools and their instructions (tools.go), and the open
// ideas wait for you there.
func Extension() extensions.Extension {
	return extensions.Extension{Name: Name, Version: 1, Streams: Streams(), Writes: []events.Type{events.TaskCreated, events.TaskHeld}, Guards: guards,
		Coordinator: coordinatorHook(), Routes: routes, Manifest: manifest, Tools: tools, Instructions: instructions, Waiting: openIdeas}
}

// Streams are the ideas' streams, for the catalogue to check and the state
// to build and keep: each holds the three idea types, and its state is its
// *Idea, nil until it is suggested. checkIdea checks what goes into one.
func Streams() *extensions.Streams {
	return &extensions.Streams{Prefix: Prefix, Types: map[events.Type]events.Spec{
		IdeaSuggested: {Fields: []string{"title", "problem", "proposal", "size", "repo", "task"}, Writers: []events.ActorKind{events.Agent, events.System}},
		IdeaAccepted:  {Fields: []string{"task"}, Writers: []events.ActorKind{events.Human, events.System}}, // "note" may be empty; Agentry's: an idea the Visionaries agreed on by your rule
		IdeaDismissed: {Fields: []string{"note"}, Writers: []events.ActorKind{events.Human}},
	}, FromEvents: fromEvents, Next: next, Check: checkIdea}
}

// fromEvents is FromEvents with no idea as a plain nil, which the state
// keeps nothing for, rather than a nil *Idea.
func fromEvents(accepted []events.Event) any {
	if idea := FromEvents(accepted); idea != nil {
		return idea
	}
	return nil
}

// next is Next on what the state held, the same way.
func next(prev any, e events.Event) any {
	held, _ := prev.(*Idea)
	if idea := Next(held, e); idea != nil {
		return idea
	}
	return nil
}

// manifest is Ideas' entry, reading the ideas from the state's streams.
func manifest(state extensions.Reader) extensions.Entry {
	list := All(state)
	entry := extensions.Entry{Badge: open(list)}
	if teams := Visionaries(state.Roster(), agents.BasesIn(state)); len(teams) > 0 {
		entry.Enabled, entry.Visible = true, true
		entry.Why = fmt.Sprintf("the %s team suggests ideas", teams[0])
		return entry
	}
	switch kept := len(list); kept {
	case 0:
		entry.Why = "add a team from the Visionaries template, or one of yours based on it, to get ideas"
	case 1:
		entry.Visible, entry.Why = true, "kept while 1 idea remains; no team from the Visionaries template or one based on it now"
	default:
		entry.Visible, entry.Why = true, fmt.Sprintf("kept while %d ideas remain; no team from the Visionaries template or one based on it now", kept)
	}
	return entry
}

// open counts the ideas in list still waiting for you to accept or dismiss.
func open(list []*Idea) int {
	count := 0
	for _, idea := range list {
		if idea.State == Open {
			count++
		}
	}
	return count
}
