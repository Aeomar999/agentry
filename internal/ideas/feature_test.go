package ideas_test

import (
	"errors"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
)

// board is the state Ideas' manifest reads: a roster, your templates,
// with their stream broken or not, and the ideas. The rest of the Reader is
// left nil, so reading it fails the test.
type board struct {
	features.Reader
	roster    agents.Roster
	templates agents.UserTemplates
	broken    error // the templates stream's
	ideas     []*ideas.Idea
}

func (b board) Roster() agents.Roster                     { return b.roster }
func (b board) Templates() agents.UserTemplates           { return b.templates }
func (b board) RemovedTemplates() agents.RemovedTemplates { return nil }
func (b board) Broken(string) error                       { return b.broken }

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
			false, "add a team from the Visionaries template, or one of yours based on it, to get ideas"},
		{"a Visionaries agent that doesn't coordinate", agents.Agent{Name: "lead", Team: "Visionaries", Template: "visionaries"},
			false, "add a team from the Visionaries template, or one of yours based on it, to get ideas"},
		{"a team from your template based on the Visionaries", agents.Agent{Name: "lead", Team: "Dreams", Coordinates: true, Template: "dreams"},
			true, "the Dreams team suggests ideas"},
	}
	for _, c := range cases {
		state := board{roster: agents.Roster{{Name: "tech-lead", Coordinates: true}, c.lead},
			templates: agents.UserTemplates{{ID: "dreams", Team: "Dreams", BasedOn: "visionaries"}}}
		entry := ideas.Feature().Manifest(state)
		if entry.Enabled != c.enabled || entry.Visible != c.enabled || entry.Why != c.why {
			t.Errorf("%s: %+v", c.name, entry)
		}
	}
}

// A broken templates stream leaves a built-in Visionaries team on, and only
// a team of your template off.
func TestIdeasIsOnForABuiltInTeamWhenYourTemplatesAreBroken(t *testing.T) {
	dreams := board{roster: agents.Roster{{Name: "lead", Team: "Dreams", Coordinates: true, Template: "dreams"}},
		templates: agents.UserTemplates{{ID: "dreams", Team: "Dreams", BasedOn: "visionaries"}}, broken: errors.New("a bad event")}
	if entry := ideas.Feature().Manifest(dreams); entry.Enabled {
		t.Errorf("your template unread: %+v", entry)
	}
	builtIn := dreams
	builtIn.roster = append(builtIn.roster, agents.Agent{Name: "visionary-lead", Team: "Visionaries", Coordinates: true, Template: "visionaries"})
	if entry := ideas.Feature().Manifest(builtIn); !entry.Enabled || entry.Why != "the Visionaries team suggests ideas" {
		t.Errorf("the built-in team: %+v", entry)
	}
}

func TestIdeasIsKeptForOneIdea(t *testing.T) {
	state := board{ideas: []*ideas.Idea{{ID: "idea-1", State: ideas.Dismissed}}}
	entry := ideas.Feature().Manifest(state)
	if entry.Enabled || !entry.Visible || entry.Badge != 0 || entry.Why != "kept while 1 idea remains; no team from the Visionaries template or one based on it now" {
		t.Errorf("one dismissed idea: %+v", entry)
	}
}
