package agents

import (
	"maps"
	"slices"
	"strings"
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

// A new agent made the coordinator in a team edit takes the template the
// team was made from, through a rename of the team in the same edit.
func TestATeamEditGivesANewCoordinatorTheTeamsTemplate(t *testing.T) {
	roster := Roster{
		{Name: "tech-lead", Coordinates: true, Template: "software"},
		{Name: "lead", Coordinates: true, Team: "Visionaries", Template: "visionaries"},
		{Name: "thinker", Team: "Visionaries"},
	}
	edit := TeamEdit{Team: "Visionaries", To: "Dreamers", Removed: []string{"lead"}, Agents: []EditedAgent{
		{Name: "new-lead", Coordinates: true, Prompt: "Lead."},
		{Was: "thinker", Name: "thinker"},
	}}
	roster = roster.Next(events.Event{Proposal: edit.Proposal(you)})
	if lead, ok := roster.Coordinator("Dreamers"); !ok || lead.Name != "new-lead" || lead.Template != "visionaries" {
		t.Errorf("Dreamers' coordinator = %+v, %v, want new-lead made from visionaries", lead, ok)
	}
	if template := roster.TemplateOf("Dreamers"); template != "visionaries" {
		t.Errorf("Dreamers is made from %q, want visionaries", template)
	}
}

// An agent of the team with no template, made its coordinator, takes the
// team's template.
func TestATeamEditGivesAnAgentMadeCoordinatorTheTeamsTemplate(t *testing.T) {
	roster := Roster{
		{Name: "tech-lead", Coordinates: true, Template: "software"},
		{Name: "lead", Coordinates: true, Team: "Visionaries", Template: "visionaries"},
		{Name: "thinker", Team: "Visionaries"},
	}
	edit := TeamEdit{Team: "Visionaries", To: "Visionaries", Agents: []EditedAgent{
		{Was: "lead", Name: "lead"},
		{Was: "thinker", Name: "thinker", Coordinates: true},
	}}
	roster = roster.Next(events.Event{Proposal: edit.Proposal(you)})
	if lead, ok := roster.Coordinator("Visionaries"); !ok || lead.Name != "thinker" || lead.Template != "visionaries" {
		t.Errorf("Visionaries' coordinator = %+v, %v, want thinker made from visionaries", lead, ok)
	}
	if template := roster.TemplateOf("Visionaries"); template != "visionaries" {
		t.Errorf("Visionaries is made from %q, want visionaries", template)
	}
}

// A link makes every agent of the team, and of no other, made from the
// template, the default team's agents that name none too.
func TestALinkMakesEveryAgentOfTheTeamFromTheTemplate(t *testing.T) {
	roster := Roster{
		{Name: "tech-lead", Coordinates: true, Template: "software"},
		{Name: "chief", Coordinates: true, Team: "Papers"},
		{Name: "engineer", Template: "software"},
		{Name: "courier", Team: "Papers", Template: "old-papers"},
		{Name: "data-lead", Coordinates: true, Team: "Data", Template: "research"},
	}
	cases := []struct {
		team, template string
		want           map[string]string
	}{
		{"Papers", "papers", map[string]string{"tech-lead": "software", "chief": "papers", "engineer": "software", "courier": "papers", "data-lead": "research"}},
		{DefaultTeam, "my-software", map[string]string{"tech-lead": "my-software", "chief": "", "engineer": "my-software", "courier": "old-papers", "data-lead": "research"}},
	}
	for _, c := range cases {
		linked := roster.Next(events.Event{Proposal: TeamLinked(c.team, c.template, you)})
		got := map[string]string{}
		for _, a := range linked {
			got[a.Name] = a.Template
		}
		if !maps.Equal(got, c.want) {
			t.Errorf("linking %s to %s: templates %v, want %v", c.team, c.template, got, c.want)
		}
	}
}

// A team edit after a link gives the agents it adds the team's link, so
// the whole team stays linked, through a rename of the team.
func TestATeamEditAfterALinkLinksItsNewAgents(t *testing.T) {
	roster := Roster{
		{Name: "tech-lead", Coordinates: true, Template: "software"},
		{Name: "chief", Coordinates: true, Team: "Papers", Engine: "claude", Prompt: "Lead."},
	}
	roster = roster.Next(events.Event{Proposal: TeamLinked("Papers", "papers", you)})
	edit := TeamEdit{Team: "Papers", To: "Letters", Agents: []EditedAgent{
		{Was: "chief", Name: "chief", Coordinates: true, Engine: "claude", Prompt: "Lead."},
		{Name: "courier", Engine: "claude", Prompt: "Take them."},
	}}
	roster = roster.Next(events.Event{Proposal: edit.Proposal(you)})
	for _, name := range []string{"chief", "courier"} {
		if a, _ := roster.Get(name); a.Team != "Letters" || a.Template != "papers" {
			t.Errorf("%s = %+v, want an agent of Letters made from papers", name, a)
		}
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

// EditTeam trims each row, takes a row named as an agent of the team for
// that agent unless another row was it, removes the agents no row was, and
// gives a new agent with no prompt a default one, but not a kept agent.
func TestEditTeamReadsThePanelsRows(t *testing.T) {
	roster := Roster{
		{Name: "tech-lead", Coordinates: true},
		{Name: "a", Team: "Data", Coordinates: true},
		{Name: "b", Team: "Data"},
		{Name: "c", Team: "Data"},
	}
	edit := EditTeam(roster, "Data", " Numbers ", []EditedAgent{
		{Name: " a ", Coordinates: true},
		{Was: "b", Name: "c", Prompt: " Count. "},
		{Name: "b", Description: "Plot."},
	})
	got := []EditedAgent{{Was: "a", Name: "a", Coordinates: true}, {Was: "b", Name: "c", Prompt: "Count."}, {Name: "b", Description: "Plot.", Prompt: edit.Agents[2].Prompt}}
	if edit.Team != "Data" || edit.To != "Numbers" || !slices.Equal(edit.Agents, got) || !slices.Equal(edit.Removed, []string{"c"}) {
		t.Errorf("the edit is %+v, want a kept, b renamed c, a new b, and c removed", edit)
	}
	if !strings.Contains(edit.Agents[2].Prompt, "Your job: Plot.") || edit.Agents[0].Prompt != "" {
		t.Errorf("prompts %q and %q, want a default for the new b only", edit.Agents[2].Prompt, edit.Agents[0].Prompt)
	}
}

// A team.edited in a log edited by hand, keeping as its coordinator an
// agent the team didn't have, replays without it rather than panicking.
func TestAHandEditedTeamEditReplays(t *testing.T) {
	roster := Roster{
		{Name: "tech-lead", Coordinates: true, Template: "software"},
		{Name: "lead", Coordinates: true, Team: "Data"},
	}
	edit := TeamEdit{Team: "Data", To: "Data", Agents: []EditedAgent{
		{Was: "lead", Name: "lead"},
		{Was: "stranger", Name: "stranger", Coordinates: true},
	}}
	roster = roster.Next(events.Event{Proposal: edit.Proposal(you)})
	if _, ok := roster.Get("stranger"); ok || len(roster) != 2 {
		t.Errorf("the roster is %+v, want tech-lead and lead", roster)
	}
}

// Renames follow an agent through renames to the name it has now, two
// agents swapping names in one edit too, and forget a removed agent.
func TestRenamesFollowAnAgentToItsNameNow(t *testing.T) {
	stream := []events.Event{
		{Proposal: Agent{Name: "a"}.Renaming(you, "b")},
		{Proposal: TeamEdit{Team: "Data", To: "Data", Agents: []EditedAgent{{Was: "b", Name: "c"}, {Was: "x", Name: "y"}}}.Proposal(you)},
		{Proposal: TeamEdit{Team: "Data", To: "Data", Agents: []EditedAgent{{Was: "c", Name: "y"}, {Was: "y", Name: "c"}}}.Proposal(you)},
		{Proposal: Agent{Name: "z"}.Renaming(you, "w")},
		{Proposal: Agent{Name: "w"}.Removal(you)},
	}
	renames := RenamesIn(stream)
	for old, now := range map[string]string{"a": "y", "b": "y", "x": "c", "c": "y", "y": "c"} {
		if renames[old] != now {
			t.Errorf("%s is %q now, want %s", old, renames[old], now)
		}
	}
	if _, ok := renames["z"]; ok {
		t.Error("z, renamed w and removed, is still followed")
	}
}

// Removals keep each agent a team edit removed with its team, by the name
// the team has now, until an agent has the name again.
func TestRemovalsKeepAnAgentsTeamByItsNameNow(t *testing.T) {
	stream := []events.Event{
		{Proposal: TeamEdit{Team: "Data", To: "Numbers", Agents: []EditedAgent{{Was: "b", Name: "b"}}, Removed: []string{"a", "c", "d", "f"}}.Proposal(you)},
		{Proposal: TeamRenaming("Numbers", "Figures", you)},
		{Proposal: TeamEdit{Team: "Figures", To: "Charts", Agents: []EditedAgent{{Was: "b", Name: "b"}}}.Proposal(you)},
		{Proposal: Agent{Name: "c", Team: "Other"}.Proposal(you, false)},
		{Proposal: Agent{Name: "f", Team: "Other"}.Proposal(you, false)},
		{Proposal: Agent{Name: "x"}.Renaming(you, "d")},
		{Proposal: TeamEdit{Team: "Other", To: "Other", Agents: []EditedAgent{{Name: "e"}}, Removed: []string{"c"}}.Proposal(you)},
	}
	removals := RemovalsIn(stream)
	if want := (Removals{"a": "Charts", "c": "Other"}); !maps.Equal(removals, want) {
		t.Errorf("removals = %v, want %v", removals, want)
	}
}

// A team edit whose kept or new row takes a removed agent's name drops
// that removal: the name is an agent's again.
func TestRemovalsForgetANameATeamEditGivesAgain(t *testing.T) {
	stream := []events.Event{
		{Proposal: TeamEdit{Team: "Data", To: "Data", Agents: []EditedAgent{{Was: "b", Name: "b"}}, Removed: []string{"a", "c", "d"}}.Proposal(you)},
		{Proposal: TeamEdit{Team: "Other", To: "Other", Agents: []EditedAgent{{Was: "x", Name: "c"}, {Name: "a"}}}.Proposal(you)},
	}
	removals := RemovalsIn(stream)
	if want := (Removals{"d": "Data"}); !maps.Equal(removals, want) {
		t.Errorf("removals = %v, want %v", removals, want)
	}
}

// A team edit sets or clears an agent's resume setting like its other
// fields, and a kept agent's version changes with it.
func TestATeamEditCarriesTheResumeSetting(t *testing.T) {
	roster := Roster{
		{Name: "data-lead", Coordinates: true, Team: "Data", Engine: "claude", Prompt: "Lead."},
		{Name: "checker", Team: "Data", Engine: "claude", Prompt: "Check.", Column: Reviews},
	}
	before := roster.TeamVersion("Data")
	if checker, _ := roster.Get("checker"); checker.Edited().Resume != ResumeDefault || checker.ResumesSessions() {
		t.Fatalf("checker before the edit: %+v, want a reviewer that starts fresh", checker.Edited())
	}
	edit := TeamEdit{Team: "Data", To: "Data", Agents: []EditedAgent{
		{Was: "data-lead", Name: "data-lead", Coordinates: true, Engine: "claude", Prompt: "Lead."},
		{Was: "checker", Name: "checker", Engine: "claude", Prompt: "Check.", Column: Reviews, Resume: ResumesSession},
		{Name: "builder", Engine: "claude", Prompt: "Build.", Resume: StartsFresh},
	}}
	roster = roster.Next(events.Event{Proposal: edit.Proposal(you)})
	checker, _ := roster.Get("checker")
	builder, _ := roster.Get("builder")
	if checker.Resume != ResumesSession || !checker.ResumesSessions() || builder.Resume != StartsFresh || builder.ResumesSessions() {
		t.Errorf("checker resume %q, builder resume %q; want the checker resuming and the builder fresh", checker.Resume, builder.Resume)
	}
	if roster.TeamVersion("Data") == before {
		t.Error("the team's version didn't change with the setting")
	}
	rows := EditTeam(roster, "Data", "Data", []EditedAgent{{Name: "data-lead", Coordinates: true, Engine: "claude", Prompt: "Lead."},
		{Name: "checker", Engine: "claude", Prompt: "Check.", Column: Reviews, Resume: " fresh "}})
	if rows.Agents[1].Resume != StartsFresh {
		t.Errorf("the panel's row reads resume %q, want it trimmed to %q", rows.Agents[1].Resume, StartsFresh)
	}
}
