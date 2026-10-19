package current

import (
	"reflect"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
)

var (
	chief    = agents.Agent{Name: "chief", Prompt: "Hand the work on.", Engine: "claude", Coordinates: true, Team: "Papers", Color: "teal"}
	courier  = agents.Agent{Name: "courier", Prompt: "Take the papers.", Engine: "claude", Profile: "safe", Team: "Papers", Column: agents.Ships}
	redrafts = agents.Agent{Name: "courier", Prompt: "Take the papers, signed.", Engine: "codex", Team: "Papers", Workers: []string{"laptop"}}
)

func saved(id, description string, members ...agents.Agent) events.Proposal {
	return proposal(events.TemplatesStream, you, events.TemplateSaved,
		map[string]any{"id": id, "team": "Papers", "description": description, "agents": members})
}

func removed(id string) events.Proposal {
	return proposal(events.TemplatesStream, you, events.TemplateRemoved, map[string]any{"id": id})
}

// Each step's log, loaded at every split and applied after, keeps the
// templates the step says, in the order they were first saved.
func TestUserTemplatesKeepTheirSaveOrder(t *testing.T) {
	papers := agents.UserTemplate{ID: "papers", Team: "Papers", Description: "First", Agents: []agents.Agent{chief, courier}}
	again := agents.UserTemplate{ID: "papers", Team: "Papers", Description: "Again", Agents: []agents.Agent{chief, redrafts}}
	orders := agents.UserTemplate{ID: "orders", Team: "Papers", Agents: []agents.Agent{chief}}
	steps := []struct {
		name string
		p    events.Proposal
		want agents.UserTemplates
	}{
		{"save", saved("papers", "First", chief, courier), agents.UserTemplates{papers}},
		{"save another", saved("orders", "", chief), agents.UserTemplates{papers, orders}},
		{"save the first again", saved("papers", "Again", chief, redrafts), agents.UserTemplates{again, orders}},
		{"remove the first", removed("papers"), agents.UserTemplates{orders}},
		{"save the first once more", saved("papers", "First", chief, courier), agents.UserTemplates{orders, papers}},
		{"remove the other", removed("orders"), agents.UserTemplates{papers}},
		{"remove the last", removed("papers"), agents.UserTemplates{}},
	}
	var proposals []events.Proposal
	for _, step := range steps {
		proposals = append(proposals, step.p)
		log := logOf(proposals...)
		for split := range len(log) + 1 {
			s := loaded(log[:split])
			for _, e := range log[split:] {
				s.Apply(e)
			}
			if got := s.Templates(); !reflect.DeepEqual(got, step.want) {
				t.Fatalf("%s, loading %d of %d events:\n%+v\nwant %+v", step.name, split, len(log), got, step.want)
			}
		}
	}
}

func TestALogWithoutTemplatesHasNone(t *testing.T) {
	if got := loaded(everyStream()).Templates(); got != nil {
		t.Errorf("templates from a log with none: %+v", got)
	}
}

// Changing what Templates gave, down to an agent's workers, changes
// nothing the state holds, and a later event changes nothing given before.
func TestTemplatesHandedOutShareNothing(t *testing.T) {
	log := logOf(saved("papers", "First", chief, redrafts))
	s := loaded(log)
	given := s.Templates()
	given[0].Description, given[0].Agents[1].Workers[0] = "changed", "changed"
	if want := agents.UserTemplatesFromEvents(log); !reflect.DeepEqual(s.Templates(), want) {
		t.Errorf("changing what was handed out changed the state:\n%+v", s.Templates())
	}

	before := s.Templates()
	s.Apply(events.Event{Position: 2, Proposal: saved("papers", "Again", chief)})
	if want := agents.UserTemplatesFromEvents(log); !reflect.DeepEqual(before, want) {
		t.Errorf("saving again changed what was handed out:\n%+v", before)
	}
}
