package agents

import (
	"strings"
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
	coordinator, ok := roster.Coordinator("")
	if !ok || coordinator.Name != "tech-lead" {
		t.Errorf("the coordinator: %+v", coordinator)
	}
	if assignees := roster.Assignees(DefaultTeam); len(assignees) != 4 || assignees[0].Name != "engineer" {
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
	want := map[string]string{"tech-lead": "read-only", "engineer": "developer", "reviewer": "read-only", "tester": "developer", "deployer": "developer"}
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

// An untouched default deployer from before task-44 gets the prompt that
// reports the pushed commit's pipeline.
func TestUpgradesBringTheDeployerItsPipelineReport(t *testing.T) {
	old := events.Actor{Kind: events.System, ID: "defaults"}
	before := Agent{Name: "deployer", Engine: "claude", Profile: "developer", Prompt: "You are the deployer. Push the task branch to dev."}
	stream := []events.Event{{Proposal: before.Proposal(old, false)}}
	for _, p := range Upgrades(FromEvents(stream)) {
		stream = append(stream, events.Event{Proposal: p})
	}
	deployer, _ := FromEvents(stream).Get("deployer")
	if !strings.Contains(deployer.Prompt, "glab ci list --sha <hash>") || !strings.Contains(deployer.Prompt, "gh run list --commit <hash>") {
		t.Errorf("the deployer's prompt after the upgrade: %q", deployer.Prompt)
	}
}

// Knowledge is the agent's own: settings changes keep it.
func TestKnowledgeSurvivesSettingsChanges(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	engineer := Agent{Name: "engineer", Engine: "claude", Prompt: "x"}
	stream := []events.Event{
		{Proposal: engineer.Proposal(you, false)},
		{Proposal: engineer.Learned(events.Actor{Kind: events.Agent, ID: "engineer"}, "Use gofmt -s.")},
		{Proposal: Agent{Name: "engineer", Engine: "codex", Prompt: "y"}.Proposal(you, true)},
	}
	if got, _ := FromEvents(stream).Get("engineer"); got.Knowledge != "Use gofmt -s." || got.Engine != "codex" {
		t.Errorf("after a settings change: %+v", got)
	}
}

func TestTeamsComeFromTemplatesWithTheirOwnCoordinator(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	writing, _ := TemplateNamed("writing")
	log := seeded()
	for _, p := range AddTeam(writing, FromEvents(log), you) {
		log = append(log, events.Event{Proposal: p})
	}
	roster := FromEvents(log)
	if teams := roster.Teams(); len(teams) != 2 || teams[0] != DefaultTeam || teams[1] != "Writing" {
		t.Fatalf("teams: %v", teams)
	}
	editor, ok := roster.Coordinator("Writing")
	if !ok || editor.Name != "editor" || len(editor.Prompt) < 200 {
		t.Errorf("the Writing team's coordinator: %+v", editor)
	}
	if first, _ := roster.Coordinator(""); first.Name != "tech-lead" {
		t.Errorf("tasks that name no team still go to the first team's coordinator, not %s", first.Name)
	}
	if assignees := roster.Assignees("Writing"); len(assignees) != 3 || assignees[2].Name != "fact-checker" {
		t.Errorf("the Writing team's agents: %+v", assignees)
	}

	// Adding it again keeps everything you have: a new team, new names.
	for _, p := range AddTeam(writing, roster, you) {
		log = append(log, events.Event{Proposal: p})
	}
	again := FromEvents(log)
	if lead, _ := again.Coordinator("Writing 2"); lead.Name != "editor-2" {
		t.Errorf("a second Writing team: %+v", again.Teams())
	}
}

// The Visionaries think and suggest; every one of them reads the board and
// changes nothing.
func TestTheVisionariesTeam(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	visionaries, ok := TemplateNamed("visionaries")
	if !ok {
		t.Fatal("no Visionaries template")
	}
	log := seeded()
	for _, p := range AddTeam(visionaries, FromEvents(log), you) {
		log = append(log, events.Event{Proposal: p})
	}
	roster := FromEvents(log)
	if len(roster) != 10 {
		t.Fatalf("want the 5 defaults and 5 visionaries, got %d agents", len(roster))
	}
	lead, ok := roster.Coordinator("Visionaries")
	if !ok || lead.Name != "visionary-lead" || lead.Team != "Visionaries" {
		t.Errorf("the Visionaries' coordinator: %+v", lead)
	}
	var names []string
	for _, agent := range roster.Assignees("Visionaries") {
		names = append(names, agent.Name)
	}
	if strings.Join(names, " ") != "product-strategist user-advocate technologist skeptic" {
		t.Errorf("the Visionaries' agents: %v", names)
	}
	for _, name := range []string{"visionary-lead", "product-strategist", "user-advocate", "technologist", "skeptic"} {
		agent, _ := roster.Get(name)
		if agent.Team != "Visionaries" || agent.Profile != "read-mostly" || agent.Engine != "claude" || len(agent.Prompt) < 400 {
			t.Errorf("%s: team %q, profile %q, engine %q, prompt %d chars", name, agent.Team, agent.Profile, agent.Engine, len(agent.Prompt))
		}
		wantColumn := ""
		if name == "skeptic" {
			wantColumn = Reviews // the skeptic checks the list
		}
		if agent.Column != wantColumn {
			t.Errorf("%s: column %q, want %q", name, agent.Column, wantColumn)
		}
	}

	// Adding it again keeps everything you have: a new team, new names.
	for _, p := range AddTeam(visionaries, roster, you) {
		log = append(log, events.Event{Proposal: p})
	}
	again := FromEvents(log)
	if lead, _ := again.Coordinator("Visionaries 2"); lead.Name != "visionary-lead-2" {
		t.Errorf("a second Visionaries team: %v", again.Teams())
	}
	if skeptic, ok := again.Get("skeptic-2"); !ok || skeptic.Team != "Visionaries 2" {
		t.Errorf("the second skeptic: %+v", skeptic)
	}
}

// The Networking team diagnoses with its two checkers and changes config
// with its engineer, under the lead.
func TestTheNetworkingTeam(t *testing.T) {
	networking, ok := TemplateNamed("networking")
	if !ok {
		t.Fatal("no Networking template")
	}
	log := seeded()
	for _, p := range AddTeam(networking, FromEvents(log), you) {
		log = append(log, events.Event{Proposal: p})
	}
	roster := FromEvents(log)
	if len(roster) != 9 {
		t.Fatalf("want the 5 defaults and 4 networking agents, got %d agents", len(roster))
	}
	lead, ok := roster.Coordinator("Networking")
	if !ok || lead.Name != "network-lead" || lead.Team != "Networking" {
		t.Errorf("the Networking team's coordinator: %+v", lead)
	}
	var names []string
	for _, agent := range roster.Assignees("Networking") {
		names = append(names, agent.Name)
	}
	if strings.Join(names, " ") != "network-engineer network-verifier network-security" {
		t.Errorf("the Networking team's agents: %v", names)
	}
	for _, want := range []struct{ name, profile, column string }{
		{"network-lead", "read-only", ""},
		{"network-engineer", "developer", ""},
		{"network-verifier", "network-diagnosis", Reviews},
		{"network-security", "network-diagnosis", Reviews},
	} {
		agent, _ := roster.Get(want.name)
		if agent.Team != "Networking" || agent.Profile != want.profile || agent.Column != want.column || agent.Engine != "claude" || len(agent.Prompt) < 400 {
			t.Errorf("%s: team %q, profile %q, column %q, engine %q, prompt %d chars", want.name, agent.Team, agent.Profile, agent.Column, agent.Engine, len(agent.Prompt))
		}
		if agent.Coordinates != (want.name == "network-lead") {
			t.Errorf("%s coordinates: %v", want.name, agent.Coordinates)
		}
	}
}

// Adding a template must not rename or reorder the teams people already add.
func TestTheTemplatesAndTheirAgents(t *testing.T) {
	want := map[string]string{
		DefaultTeam:   "tech-lead engineer reviewer tester deployer",
		"Writing":     "editor researcher writer fact-checker",
		"Research":    "research-lead investigator analyst critic",
		"Design":      "product-lead product-designer ux-writer design-critic",
		"Visionaries": "visionary-lead product-strategist user-advocate technologist skeptic",
		"Networking":  "network-lead network-engineer network-verifier network-security",
	}
	var teams []string
	for _, template := range Templates {
		teams = append(teams, template.Team)
		var names []string
		for _, agent := range template.Agents {
			names = append(names, agent.Name)
		}
		if got := strings.Join(names, " "); got != want[template.Team] {
			t.Errorf("%s: %s, want %s", template.Team, got, want[template.Team])
		}
	}
	if got := strings.Join(teams, ", "); got != DefaultTeam+", Writing, Research, Design, Visionaries, Networking" {
		t.Errorf("templates: %s", got)
	}
}

// A built-in template's id is its team's name in lowercase: adding a team
// finds a built-in template by its team's name before a saved one by id,
// and the rules refuse a saved template a built-in's id, so no saved
// template is hidden behind a built-in one.
func TestEveryTemplatesIDIsItsTeamInLowercase(t *testing.T) {
	for _, template := range Templates {
		if template.ID != strings.ToLower(template.Team) {
			t.Errorf("the %s template's id is %s, not %s", template.Team, template.ID, strings.ToLower(template.Team))
		}
	}
}

func TestEveryTemplateHasACoordinatorAndPrompts(t *testing.T) {
	for _, template := range Templates {
		coordinators := 0
		for _, agent := range template.prompted() {
			if len(agent.Prompt) < 400 || agent.Description == "" {
				t.Errorf("%s/%s: prompt %d chars, description %q", template.Team, agent.Name, len(agent.Prompt), agent.Description)
			}
			if agent.Coordinates {
				coordinators++
			}
		}
		if coordinators != 1 {
			t.Errorf("%s has %d coordinators, want 1", template.Team, coordinators)
		}
	}
}

// A team made from a template records it on every agent, so it is known
// whatever the team is called: a second Visionaries team is one too.
func TestATeamRecordsItsTemplate(t *testing.T) {
	visionaries, _ := TemplateNamed("visionaries")
	log := seeded()
	for range 2 {
		for _, p := range AddTeam(visionaries, FromEvents(log), you) {
			if p.Text("template") != "visionaries" {
				t.Errorf("%s: template %q", p.Text("name"), p.Text("template"))
			}
			log = append(log, events.Event{Proposal: p})
		}
	}
	roster := FromEvents(log)
	for team, want := range map[string]string{DefaultTeam: "software", "Visionaries": "visionaries", "Visionaries 2": "visionaries", "Writing": ""} {
		if got := roster.TemplateOf(team); got != want {
			t.Errorf("%s: template %q, want %q", team, got, want)
		}
	}
	if skeptic, _ := roster.Get("skeptic-2"); skeptic.Template != "visionaries" {
		t.Errorf("the second skeptic: template %q", skeptic.Template)
	}
}

// A team you make yourself has no template, even with a template's name.
func TestAHandMadeTeamHasNoTemplate(t *testing.T) {
	log := seeded()
	for _, agent := range []Agent{{Name: "lead", Coordinates: true, Team: "Visionaries"}, {Name: "thinker", Team: "Visionaries"}} {
		agent.Engine, agent.Prompt = "claude", "Think."
		log = append(log, events.Event{Proposal: agent.Proposal(you, false)})
	}
	if got := FromEvents(log).TemplateOf("Visionaries"); got != "" {
		t.Errorf("a hand-made Visionaries team: template %q", got)
	}
}

// before is the event p as Agentry wrote it before agents recorded their
// template: without the field at all.
func before(p events.Proposal) events.Event {
	data := map[string]any{}
	for field, value := range p.Data {
		if field != "template" {
			data[field] = value
		}
	}
	p.Data = data
	return events.Event{Proposal: p}
}

// Agents from before the field find their template by their team's name,
// as AddTeam names teams; the log needs no new event.
func TestAnOldLogFindsItsTemplatesByName(t *testing.T) {
	want := map[string]string{"": "software", "Visionaries": "visionaries", "visionaries 3": "visionaries", "Visionaries 2": "visionaries",
		"Writing 12": "writing", "Visionaries Two": "", "Visionaries2": "", "Visionaries 2b": "", "Thinkers": ""}
	var log []events.Event
	for team := range want {
		log = append(log, before(agent("lead of "+team, team, "").Proposal(you, false)))
	}
	roster := FromEvents(log)
	for team, template := range want {
		if got, _ := roster.Get("lead of " + team); got.Template != template {
			t.Errorf("team %q: template %q, want %q", team, got.Template, template)
		}
	}
}

// An old agent.changed keeps the template the agent has, unless it moves
// the agent to another team.
func TestAnOldChangeKeepsTheTemplate(t *testing.T) {
	made := agent("thinker", "Thinkers", "visionaries")
	hand := agent("lead", "Visionaries", "")
	moved := agent("mover", "Thinkers", "visionaries")
	log := []events.Event{{Proposal: made.Proposal(you, false)}, {Proposal: hand.Proposal(you, false)}, {Proposal: moved.Proposal(you, false)}}
	moved.Team = "Writing"
	log = append(log, before(made.Proposal(you, true)), before(hand.Proposal(you, true)), before(moved.Proposal(you, true)))
	roster := FromEvents(log)
	for name, want := range map[string]string{"thinker": "visionaries", "lead": "", "mover": "writing"} {
		if got, _ := roster.Get(name); got.Template != want {
			t.Errorf("%s: template %q, want %q", name, got.Template, want)
		}
	}
}

var you = events.Actor{Kind: events.Human, ID: "you"}

func agent(name, team, template string) Agent {
	return Agent{Name: name, Team: team, Template: template, Engine: "claude", Prompt: "Work."}
}

func TestATeamIsNamedInAnyCase(t *testing.T) {
	roster := Roster{{Name: "tech-lead", Coordinates: true}, {Name: "data-lead", Coordinates: true, Team: "Data"}}
	for asked, want := range map[string]string{"Data": "Data", " data ": "Data", "SOFTWARE": "Software", "Datta": "", "": ""} {
		if got := roster.TeamNamed(asked); got != want {
			t.Errorf("TeamNamed(%q) = %q, want %q", asked, got, want)
		}
	}
	// An old log can hold two teams named in different cases: the first made is the one meant.
	twins := Roster{{Name: "data-lead", Coordinates: true, Team: "data"}, {Name: "chief", Coordinates: true, Team: "Data"}}
	for _, asked := range []string{"Data", "data", "DATA"} {
		if got := twins.TeamNamed(asked); got != "data" {
			t.Errorf("with twins, TeamNamed(%q) = %q, want the first, %q", asked, got, "data")
		}
	}
}

func TestATeamRenamedMovesEveryAgent(t *testing.T) {
	roster := Roster{
		{Name: "tech-lead", Coordinates: true, Template: "software"},
		{Name: "engineer", Template: "software"},
		{Name: "data-lead", Coordinates: true, Team: "Data", Knowledge: "Know."},
		{Name: "analyst", Team: "Data", Column: Reviews},
	}
	roster = roster.Next(events.Event{Proposal: TeamRenaming("Data", "Analytics", you)})
	if lead, ok := roster.Coordinator("Analytics"); !ok || lead.Name != "data-lead" || lead.Knowledge != "Know." {
		t.Errorf("Analytics' coordinator = %+v, %v, want data-lead with its knowledge", lead, ok)
	}
	if analyst, _ := roster.Get("analyst"); analyst.Team != "Analytics" || analyst.Column != Reviews {
		t.Errorf("analyst = %+v, want in Analytics, still reviewing", analyst)
	}
	if teams := strings.Join(roster.Teams(), ","); teams != "Software,Analytics" {
		t.Errorf("teams = %s, want Software,Analytics", teams)
	}

	// The team that names none gets the new name; its template stays.
	roster = roster.Next(events.Event{Proposal: TeamRenaming(DefaultTeam, "Platform", you)})
	for _, name := range []string{"tech-lead", "engineer"} {
		if a, _ := roster.Get(name); a.Team != "Platform" || a.Template != "software" {
			t.Errorf("%s = %+v, want in Platform from the software template", name, a)
		}
	}
	if teams := strings.Join(roster.Teams(), ","); teams != "Platform,Analytics" {
		t.Errorf("teams = %s, want Platform,Analytics", teams)
	}
}
