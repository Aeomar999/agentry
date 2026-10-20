package agents

import (
	"encoding/json"
	"testing"
	"time"

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

	saving, err := SaveTemplate(made, "Papers", "Papers", "papers", "Reads papers.", you)
	if err != nil {
		t.Fatal(err)
	}
	saved, ok := UserTemplatesFromEvents([]events.Event{logged(t, saving)}).WithID("papers")
	if !ok || saved.Team != "Papers" || saved.Description != "Reads papers." || saved.From != "Papers" {
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
	if _, err := SaveTemplate(FromEvents(seeded()), "Papers", "Papers", "papers", "", events.Actor{Kind: events.Human, ID: "you"}); err == nil {
		t.Error("saved a team with no agents")
	}
}

func TestTheDefaultTeamIsSavedByItsName(t *testing.T) {
	saving, err := SaveTemplate(FromEvents(seeded()), DefaultTeam, DefaultTeam, "mine", "", events.Actor{Kind: events.Human, ID: "you"})
	if members, _ := TemplateAgents(saving.Data); err != nil || len(members) != len(defaults) {
		t.Errorf("the %s team saved %d agents (%v)", DefaultTeam, len(members), err)
	}
}

// A template is named apart from the team it was saved from: a team made
// from it gets the name, and the template keeps where it came from and,
// from the log, when it was saved.
func TestATemplateKeepsItsNameItsTeamAndWhenItWasSaved(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	log := added(papers().Template(), seeded())
	saving, err := SaveTemplate(FromEvents(log), "Papers", "Reading Room", "reading-room", "", you)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)
	event := logged(t, saving)
	event.At = at
	saved, _ := UserTemplatesFromEvents([]events.Event{event}).WithID("reading-room")
	if saved.Team != "Reading Room" || saved.From != "Papers" || !saved.SavedAt.Equal(at) {
		t.Errorf("the saved template: %+v", saved)
	}
	lead, _ := FromEvents(added(saved.Template(), log)).Coordinator("Reading Room")
	if lead.Name != "lead-2" {
		t.Errorf("a team made from it is called Reading Room: %+v", lead)
	}
}

// A template.saved from before templates were named has no "from": it
// replays with none, its team as it was, and when the log says.
func TestATemplateSavedWithoutFromReplays(t *testing.T) {
	at := time.Date(2025, 3, 1, 8, 0, 0, 0, time.UTC)
	old := events.Event{At: at, Proposal: events.Proposal{Stream: events.TemplatesStream, Type: events.TemplateSaved, Data: map[string]any{
		"id": "papers", "team": "Papers", "description": "Reads papers.", "agents": []any{
			map[string]any{"name": "lead", "prompt": "Hand out the reading.", "engine": "claude", "coordinates": true}},
	}}}
	saved, ok := UserTemplatesFromEvents([]events.Event{old}).WithID("papers")
	if !ok || saved.Team != "Papers" || saved.From != "" || !saved.SavedAt.Equal(at) || len(saved.Agents) != 1 {
		t.Errorf("an old template: %+v", saved)
	}
}

// A template with no source team saves no "from", so an imported file
// from before templates were named writes the event it always did.
func TestATemplateWithNoSourceTeamSavesNoFrom(t *testing.T) {
	if data := papers().Save(events.Actor{Kind: events.Human, ID: "you"}).Data; data["from"] != nil {
		t.Errorf("saved %+v", data)
	}
}

// A save from a team never replaces a template: its event says it is new,
// so the rules refuse a name or id of yours, and an import's does not.
func TestASavedTeamIsANewTemplate(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	saving, err := SaveTemplate(FromEvents(seeded()), DefaultTeam, DefaultTeam, "mine", "", you)
	if err != nil || saving.Data["new"] != true {
		t.Errorf("saved %+v (%v), want new", saving.Data, err)
	}
	if imported := papers().Save(you); imported.Data["new"] != nil {
		t.Errorf("imported %+v, want not new", imported.Data)
	}
}
