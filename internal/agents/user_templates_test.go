package agents

import (
	"encoding/json"
	"strings"
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

func TestASavedTeamIsAddedBackTheSame(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	log := seeded()
	for _, a := range papers().Agents {
		a.Team, a.Workers, a.Model, a.Profile = "Papers", []string{"laptop"}, "opus", "developer"
		log = append(log, events.Event{Proposal: a.Proposal(you, false)})
	}
	log = append(log, events.Event{Proposal: Agent{Name: "reader"}.Learned(you, "Skim the abstract first.")})
	made := FromEvents(log)

	saving, err := SaveTemplate(made, "Papers", "papers", "Reads papers.", you)
	if err != nil {
		t.Fatal(err)
	}
	saved, ok := UserTemplatesFromEvents([]events.Event{logged(t, saving)}).WithID("papers")
	if !ok || saved.Team != "Papers" || saved.Description != "Reads papers." {
		t.Fatalf("the saved template: %+v", saved)
	}

	for _, a := range made.Assignees("Papers") {
		log = append(log, events.Event{Proposal: a.Removal(you)})
	}
	lead, _ := made.Coordinator("Papers")
	log = added(saved.Template(), append(log, events.Event{Proposal: lead.Removal(you)}))

	again := FromEvents(log)
	for _, had := range made {
		if had.Team != "Papers" {
			continue
		}
		want := had
		want.Workers, want.Knowledge, want.Template, want.untouched = []string{}, "", "papers", false
		got, _ := again.Get(had.Name)
		if !equalAgents(got, want) {
			t.Errorf("%s added back:\n got %+v\nwant %+v", had.Name, got, want)
		}
	}
}

func equalAgents(a, b Agent) bool {
	encodedA, _ := json.Marshal(a)
	encodedB, _ := json.Marshal(b)
	return string(encodedA) == string(encodedB)
}

func TestSavingATeamThatIsntThere(t *testing.T) {
	if _, err := SaveTemplate(FromEvents(seeded()), "Papers", "papers", "", events.Actor{Kind: events.Human, ID: "you"}); err == nil {
		t.Error("saved a team with no agents")
	}
}

func TestTheDefaultTeamIsSavedByItsName(t *testing.T) {
	saving, err := SaveTemplate(FromEvents(seeded()), DefaultTeam, "mine", "", events.Actor{Kind: events.Human, ID: "you"})
	if members, _ := TemplateAgents(saving.Data); err != nil || len(members) != len(defaults) {
		t.Errorf("the %s team saved %d agents (%v)", DefaultTeam, len(members), err)
	}
}

func TestAUserTemplateWithTheLongestNamesIsAddedTwice(t *testing.T) {
	long := papers()
	long.Agents[0].Name = "lead-" + strings.Repeat("a", 23) // 28 characters, the most a template's rules allow
	again := FromEvents(added(long.Template(), added(long.Template(), seeded())))
	numbered, ok := again.Get(long.Agents[0].Name + "-2")
	if !ok || len(numbered.Name) > 31 || numbered.Team != "Papers 2" {
		t.Errorf("the second lead: %+v (%d characters)", numbered, len(numbered.Name))
	}
	if made := len(again.Assignees("Papers 2")) + 1; made != len(long.Agents) {
		t.Errorf("the second Papers team has %d agents, want %d", made, len(long.Agents))
	}
}
