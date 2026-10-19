package ideas_test

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
)

// board is the state Ideas' manifest reads: a roster and the ideas. The
// rest of the Reader is left nil, so reading it fails the test.
type board struct {
	features.Reader
	roster agents.Roster
	ideas  []*ideas.Idea
}

func (b board) Roster() agents.Roster { return b.roster }

func (b board) Owned(stream string) (any, error) {
	for _, idea := range b.ideas {
		if idea.ID == stream {
			return idea, nil
		}
	}
	return nil, nil
}

func (b board) OwnedIDs(prefix string) []string {
	var ids []string
	for _, idea := range b.ideas {
		ids = append(ids, idea.ID)
	}
	return ids
}

// Ideas is on by the template a team was made from, not its name.
func TestIdeasIsOnByTheTeamsTemplate(t *testing.T) {
	cases := []struct {
		name    string
		lead    agents.Agent
		enabled bool
		why     string
	}{
		{"a second Visionaries team", agents.Agent{Name: "lead", Team: "Visionaries 2", Coordinates: true, Template: "visionaries"},
			true, "the Visionaries 2 team suggests ideas"},
		{"a team you called Visionaries", agents.Agent{Name: "lead", Team: "Visionaries", Coordinates: true},
			false, "add a team from the Visionaries template to get ideas"},
		{"a Visionaries agent that doesn't coordinate", agents.Agent{Name: "lead", Team: "Visionaries", Template: "visionaries"},
			false, "add a team from the Visionaries template to get ideas"},
	}
	for _, c := range cases {
		state := board{roster: agents.Roster{{Name: "tech-lead", Coordinates: true}, c.lead}}
		entry := ideas.Feature().Manifest(state)
		if entry.Enabled != c.enabled || entry.Visible != c.enabled || entry.Why != c.why {
			t.Errorf("%s: %+v", c.name, entry)
		}
	}
}

func TestIdeasIsKeptForOneIdea(t *testing.T) {
	state := board{ideas: []*ideas.Idea{{ID: "idea-1", State: ideas.Dismissed}}}
	entry := ideas.Feature().Manifest(state)
	if entry.Enabled || !entry.Visible || entry.Badge != 0 || entry.Why != "kept while 1 idea remains; no team from the Visionaries template now" {
		t.Errorf("one dismissed idea: %+v", entry)
	}
}
