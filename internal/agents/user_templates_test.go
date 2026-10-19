package agents

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// papers is a template you saved: its prompts are yours, not in defaults/.
func papers() UserTemplate {
	return UserTemplate{ID: "papers", Team: "Papers", Description: "Reads papers.", Agents: []Agent{
		{Name: "lead", Prompt: "Hand out the reading.", Engine: "claude", Coordinates: true, Color: "teal"},
		{Name: "reader", Prompt: "Read the papers.", Engine: "codex", Column: Reviews},
	}}
}

func added(t Template, log []events.Event) []events.Event {
	for _, p := range AddTeam(t, FromEvents(log), events.Actor{Kind: events.Human, ID: "you"}) {
		log = append(log, events.Event{Proposal: p})
	}
	return log
}

func TestATeamIsAddedFromAUserTemplateWithItsPrompts(t *testing.T) {
	roster := FromEvents(added(papers().Template(), seeded()))
	lead, _ := roster.Coordinator("Papers")
	reader, _ := roster.Get("reader")
	if lead.Name != "lead" || lead.Prompt != "Hand out the reading." || lead.Template != "papers" {
		t.Errorf("the coordinator: %+v", lead)
	}
	if reader.Prompt != "Read the papers." || reader.Team != "Papers" || reader.Template != "papers" || reader.Column != Reviews {
		t.Errorf("the reader: %+v", reader)
	}
}

func TestAUserTemplateAddedAgainGetsNumbers(t *testing.T) {
	log := added(papers().Template(), added(papers().Template(), seeded()))
	again := FromEvents(log)
	lead, _ := again.Coordinator("Papers 2")
	reader, _ := again.Get("reader-2")
	if lead.Name != "lead-2" || reader.Team != "Papers 2" || reader.Prompt != "Read the papers." {
		t.Errorf("a second Papers team: %v, %+v, %+v", again.Teams(), lead, reader)
	}
}
