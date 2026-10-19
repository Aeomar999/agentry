package ideas

import (
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/features"
)

// Feature is Ideas as a feature, its ideas read from source. It is enabled
// while some team's coordinator suggests ideas (Suggests), and visible while
// it is enabled or any idea, of any state, is left: your open ideas still
// need an answer and the rest are their history. Its badge is the number of
// open ideas.
func Feature(source Source) features.Feature {
	return features.Feature{Name: Name, Version: 1, Manifest: func(state features.Reader) features.Entry {
		return manifest(source.Ideas(), state)
	}}
}

func manifest(list []*Idea, state features.Reader) features.Entry {
	entry := features.Entry{Badge: open(list)}
	roster := state.Roster()
	for _, team := range roster.Teams() {
		if coordinator, found := roster.Coordinator(team); found && Suggests(coordinator) {
			entry.Enabled, entry.Visible = true, true
			entry.Why = fmt.Sprintf("the %s team suggests ideas", team)
			return entry
		}
	}
	switch kept := len(list); kept {
	case 0:
		entry.Why = "add the Visionaries team to get ideas"
	case 1:
		entry.Visible, entry.Why = true, "kept while 1 idea remains; the Visionaries team was removed"
	default:
		entry.Visible, entry.Why = true, fmt.Sprintf("kept while %d ideas remain; the Visionaries team was removed", kept)
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
