package agents

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

func seeded() []events.Event {
	var stream []events.Event
	for _, p := range SeedProposals(nil) {
		stream = append(stream, events.Event{Proposal: p})
	}
	return stream
}

func TestTheDefaultsAreFiveAgentsWithPrompts(t *testing.T) {
	roster := FromEvents(seeded())
	if len(roster) != 5 {
		t.Fatalf("want 5 default agents, got %d", len(roster))
	}
	for _, agent := range roster {
		if len(agent.Prompt) < 200 || agent.Description == "" || agent.Engine != "claude" {
			t.Errorf("%s: prompt %d chars, description %q, engine %q", agent.Name, len(agent.Prompt), agent.Description, agent.Engine)
		}
	}
	coordinator, ok := roster.Coordinator()
	if !ok || coordinator.Name != "coordinator" {
		t.Errorf("the coordinator: %+v", coordinator)
	}
	if assignees := roster.Assignees(); len(assignees) != 4 || assignees[0].Name != "engineer" {
		t.Errorf("assignees: %+v", assignees)
	}
}

func TestSeedOnlyWhenThereAreNoAgents(t *testing.T) {
	if again := SeedProposals(FromEvents(seeded())); again != nil {
		t.Errorf("seeding an existing roster should do nothing, got %d proposals", len(again))
	}
}

func TestAChangeReplacesTheAgent(t *testing.T) {
	stream := seeded()
	engineer, _ := FromEvents(stream).Get("engineer")
	engineer.Prompt, engineer.Engine = "Only write Go.", "codex"
	stream = append(stream, events.Event{Proposal: engineer.Proposal(events.Actor{Kind: events.Human, ID: "you"}, true)})

	roster := FromEvents(stream)
	if changed, _ := roster.Get("engineer"); changed.Prompt != "Only write Go." || changed.Engine != "codex" || len(roster) != 5 {
		t.Errorf("after the change: %+v (%d agents)", changed, len(roster))
	}
}

func TestEachDefaultAgentHasAProfile(t *testing.T) {
	want := map[string]string{"coordinator": "read-only", "engineer": "developer", "reviewer": "read-only", "tester": "developer", "deployer": "developer"}
	for _, agent := range FromEvents(seeded()) {
		if agent.Profile != want[agent.Name] {
			t.Errorf("%s: profile %q, want %q", agent.Name, agent.Profile, want[agent.Name])
		}
	}
}

func TestUpgradesFillOnlyEmptyProfiles(t *testing.T) {
	var stream []events.Event
	for _, agent := range FromEvents(seeded()) {
		agent.Profile = ""
		if agent.Name == "tester" {
			agent.Profile = "autonomous" // the user's choice stays
		}
		stream = append(stream, events.Event{Proposal: agent.Proposal(events.Actor{Kind: events.System, ID: "old"}, false)})
	}
	upgrades := Upgrades(FromEvents(stream))
	for _, p := range upgrades {
		stream = append(stream, events.Event{Proposal: p})
	}
	roster := FromEvents(stream)
	reviewer, _ := roster.Get("reviewer")
	tester, _ := roster.Get("tester")
	if len(upgrades) != 4 || reviewer.Profile != "read-only" || tester.Profile != "autonomous" {
		t.Errorf("%d upgrades, reviewer %q, tester %q", len(upgrades), reviewer.Profile, tester.Profile)
	}
	if again := Upgrades(roster); len(again) != 0 {
		t.Errorf("upgrading twice should do nothing, got %d", len(again))
	}
}

// A removed agent leaves the roster; workers survive the trip through the log.
func TestRemovedAgentsLeaveAndWorkersAreKept(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	designer := Agent{Name: "designer", Engine: "claude", Prompt: "Design.", Workers: []string{"mac-mini", "build-box"}}
	stream := append(seeded(), asEvent(designer.Proposal(you, false)))
	if got, _ := FromEvents(stream).Get("designer"); len(got.Workers) != 2 || got.Workers[1] != "build-box" {
		t.Errorf("workers: %+v", got.Workers)
	}
	stream = append(stream, asEvent(designer.Removal(you)))
	if _, ok := FromEvents(stream).Get("designer"); ok {
		t.Error("the removed agent is still in the roster")
	}
}

// asEvent is a proposal as the log hands it back: lists come back as []any.
func asEvent(p events.Proposal) events.Event {
	data := map[string]any{}
	for key, value := range p.Data {
		if list, ok := value.([]string); ok {
			items := make([]any, len(list))
			for i, item := range list {
				items[i] = item
			}
			value = items
		}
		data[key] = value
	}
	p.Data = data
	return events.Event{Proposal: p}
}

// An agent you changed is never upgraded, even where you left a field empty.
func TestUpgradesLeaveAgentsYouChanged(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	old := events.Actor{Kind: events.System, ID: "defaults"}
	stream := []events.Event{
		{Proposal: Agent{Name: "engineer", Engine: "claude", Prompt: "x"}.Proposal(old, false)},
		{Proposal: Agent{Name: "reviewer", Engine: "claude", Prompt: "x"}.Proposal(old, false)},
		{Proposal: Agent{Name: "reviewer", Engine: "claude", Prompt: "mine"}.Proposal(you, true)},
	}
	upgrades := Upgrades(FromEvents(stream))
	if len(upgrades) != 1 || upgrades[0].Text("name") != "engineer" || upgrades[0].Text("color") != "blue" || upgrades[0].Text("profile") != "developer" {
		t.Errorf("upgrades: %+v", upgrades)
	}
}
