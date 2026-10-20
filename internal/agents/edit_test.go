package agents

import (
	"slices"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// A team edit renames, changes, adds and removes the team's agents in one
// event: a renamed agent keeps what it knows, where it runs and its place,
// and a new one goes last.
func TestATeamEditKeepsWhatItDoesntTouch(t *testing.T) {
	roster := Roster{
		{Name: "tech-lead", Coordinates: true, Template: "software"},
		{Name: "data-lead", Coordinates: true, Team: "Data", Engine: "claude", Prompt: "Lead."},
		{Name: "analyst", Team: "Data", Engine: "claude", Prompt: "Analyse.", Knowledge: "Know.", Workers: []string{"box"}},
		{Name: "charter", Team: "Data", Engine: "claude", Prompt: "Chart."},
	}
	edit := TeamEdit{Team: "Data", To: "Data", Removed: []string{"charter"}, Agents: []EditedAgent{
		{Was: "data-lead", Name: "data-lead", Coordinates: true, Engine: "claude", Prompt: "Lead."},
		{Was: "analyst", Name: "statistician", Engine: "codex", Prompt: "Count.", Color: "teal"},
		{Name: "plotter", Engine: "claude", Prompt: "Plot.", Column: Reviews},
	}}
	roster = roster.Next(events.Event{Proposal: edit.Proposal(you)})

	var names []string
	for _, a := range roster {
		names = append(names, a.Name)
	}
	if want := []string{"tech-lead", "data-lead", "statistician", "plotter"}; !slices.Equal(names, want) {
		t.Fatalf("the roster is %v, want %v", names, want)
	}
	statistician, _ := roster.Get("statistician")
	if statistician.Knowledge != "Know." || !slices.Equal(statistician.Workers, []string{"box"}) || statistician.Engine != "codex" ||
		statistician.Prompt != "Count." || statistician.Color != "teal" || statistician.Team != "Data" {
		t.Errorf("statistician = %+v, want analyst renamed and changed, with its knowledge and workers", statistician)
	}
	if plotter, _ := roster.Get("plotter"); plotter.Team != "Data" || plotter.Column != Reviews || plotter.Knowledge != "" {
		t.Errorf("plotter = %+v, want a new reviewer of Data", plotter)
	}
}

// A team edit can rename the team and move its coordinator: the template
// the team was made from goes with the coordinator, and stays through a
// rename, so the team keeps its features.
func TestATeamEditMovesTheCoordinatorWithItsTemplate(t *testing.T) {
	roster := Roster{
		{Name: "tech-lead", Coordinates: true, Template: "software"},
		{Name: "lead", Coordinates: true, Team: "Visionaries", Template: "visionaries"},
		{Name: "thinker", Team: "Visionaries", Template: "visionaries"},
	}
	edit := TeamEdit{Team: "Visionaries", To: "Dreamers", Agents: []EditedAgent{
		{Was: "lead", Name: "lead"},
		{Was: "thinker", Name: "head-thinker", Coordinates: true},
	}}
	roster = roster.Next(events.Event{Proposal: edit.Proposal(you)})
	if lead, ok := roster.Coordinator("Dreamers"); !ok || lead.Name != "head-thinker" {
		t.Errorf("Dreamers' coordinator = %+v, %v, want head-thinker", lead, ok)
	}
	if template := roster.TemplateOf("Dreamers"); template != "visionaries" {
		t.Errorf("Dreamers is made from %q, want visionaries", template)
	}
	if was, _ := roster.Get("lead"); was.Coordinates || was.Team != "Dreamers" {
		t.Errorf("lead = %+v, want an agent of Dreamers that doesn't coordinate", was)
	}
	if teams := roster.Teams(); !slices.Equal(teams, []string{DefaultTeam, "Dreamers"}) {
		t.Errorf("teams = %v, want Software and Dreamers", teams)
	}
}

// The default team's agents may name no team: an edit that keeps its name
// leaves them, and its new agents, naming none.
func TestAnEditOfTheDefaultTeamKeepsItNamingNone(t *testing.T) {
	roster := Roster{{Name: "tech-lead", Coordinates: true, Template: "software", untouched: true, Prompt: "Lead."}}
	edit := TeamEdit{Team: DefaultTeam, To: DefaultTeam, Agents: []EditedAgent{
		{Was: "tech-lead", Name: "tech-lead", Coordinates: true, Prompt: "Lead."},
		{Name: "helper", Prompt: "Help."},
	}}
	roster = roster.Next(events.Event{Proposal: edit.Proposal(you)})
	for _, a := range roster {
		if a.Team != "" {
			t.Errorf("%s names team %q, want none", a.Name, a.Team)
		}
	}
	if lead, _ := roster.Get("tech-lead"); !lead.untouched || lead.Template != "software" {
		t.Errorf("tech-lead = %+v, want it untouched, still made from software", lead)
	}
}

// A team's version changes with what the panel edits, wherever it is
// edited, and not with what an agent learns; other teams don't change it.
func TestATeamsVersionFollowsWhatThePanelEdits(t *testing.T) {
	roster := Roster{
		{Name: "tech-lead", Coordinates: true},
		{Name: "data-lead", Coordinates: true, Team: "Data", Prompt: "Lead."},
	}
	version := roster.TeamVersion("Data")
	learned := roster.Next(events.Event{Proposal: Agent{Name: "data-lead"}.Learned(you, "Know.")})
	other := roster.Next(events.Event{Proposal: Agent{Name: "tech-lead", Coordinates: true, Prompt: "New."}.Proposal(you, true)})
	if learned.TeamVersion("Data") != version || other.TeamVersion("Data") != version {
		t.Error("learning, or a change in another team, changed Data's version")
	}
	changed := roster.Next(events.Event{Proposal: Agent{Name: "data-lead", Coordinates: true, Team: "Data", Prompt: "Lead well."}.Proposal(you, true)})
	renamed := roster.Next(events.Event{Proposal: TeamRenaming("Data", "Numbers", you)})
	if changed.TeamVersion("Data") == version || renamed.TeamVersion("Numbers") == version {
		t.Error("a changed prompt, or a team rename, kept Data's version")
	}
}
