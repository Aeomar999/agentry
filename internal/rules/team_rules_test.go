package rules

import (
	"slices"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// dataTeam adds the Data team to a seeded store: data-lead coordinates
// analyst and charter, and an open task-1 needs analyst.
func dataTeam(t *testing.T, s *store.Store) {
	t.Helper()
	for _, a := range []agents.Agent{
		{Name: "data-lead", Engine: "claude", Prompt: "Lead.", Coordinates: true, Team: "Data"},
		{Name: "analyst", Engine: "claude", Prompt: "Analyse.", Team: "Data"},
		{Name: "charter", Engine: "claude", Prompt: "Chart.", Team: "Data"},
	} {
		if _, rejection, err := s.Propose(a.Proposal(you, false)); rejection != nil || err != nil {
			t.Fatalf("adding %s: %v %v", a.Name, rejection, err)
		}
	}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Count", "repo": "/src/app", "coordinator": "data-lead"}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "data-lead"}, Type: events.TaskAssigned, Data: map[string]any{"agent": "analyst", "instructions": "Count."}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatalf("%s: %v %v", p.Type, rejection, err)
		}
	}
}

// dataAsItIs is the Data team as dataTeam makes it, as an edit keeps it.
func dataAsItIs() agents.TeamEdit {
	return agents.TeamEdit{Team: "Data", To: "Data", Agents: []agents.EditedAgent{
		{Was: "data-lead", Name: "data-lead", Engine: "claude", Prompt: "Lead.", Coordinates: true},
		{Was: "analyst", Name: "analyst", Engine: "claude", Prompt: "Analyse."},
		{Was: "charter", Name: "charter", Engine: "claude", Prompt: "Chart."},
	}}
}

// The dry run lists every problem of an edit at once, each agent's by its
// row and field, the team's by the team's field, in the edit's order.
func TestATeamEditsProblemsAreListedAtOnce(t *testing.T) {
	s, state := openSeededState(t)
	dataTeam(t, s)
	edit := dataAsItIs()
	edit.To = "Research"                                                                             // a built-in template's team
	edit.Agents[0].Coordinates = false                                                               // no coordinator
	edit.Agents[1] = agents.EditedAgent{Was: "analyst", Name: "Analyst", Engine: "gpt", Prompt: " "} // a bad name, engine and prompt
	edit.Agents[2].Name = "engineer"                                                                 // the Software team's
	edit.Agents = append(edit.Agents, agents.EditedAgent{Name: "engineer", Engine: "claude", Prompt: "Twice.", Color: "mauve"})

	var got []string
	for _, problem := range TeamEditProblems(state, edit.Proposal(you)) {
		got = append(got, problem.Field+"@"+string(rune('0'+problem.Row+1)))
	}
	want := []string{"team@0", "coordinates@0", "name@2", "engine@2", "prompt@2", "name@3", "color@4", "name@4"}
	if !slices.Equal(got, want) {
		t.Errorf("problems (field@row+1) = %v, want %v", got, want)
	}
	if _, rejection, err := s.Propose(edit.Proposal(you)); err != nil || rejection == nil || rejection.Reason != TeamEditProblems(state, edit.Proposal(you))[0].Reason {
		t.Errorf("the edit was refused with %v, %v, want its first problem", rejection, err)
	}
}

// The rule refuses an edit that doesn't account for the team's agents as
// they are: one it neither keeps nor removes, one it keeps twice, one it
// removes from another team, or one an open task still needs.
func TestATeamEditAccountsForEveryAgent(t *testing.T) {
	s, state := openSeededState(t)
	dataTeam(t, s)
	cases := []struct {
		name   string
		edit   func(e *agents.TeamEdit)
		reason string
	}{
		{"an agent left out", func(e *agents.TeamEdit) { e.Agents = e.Agents[:2] }, "the edit neither keeps nor removes charter"},
		{"an agent kept and removed", func(e *agents.TeamEdit) { e.Removed = []string{"charter"} }, "charter can't be both kept and removed"},
		{"another team's agent removed", func(e *agents.TeamEdit) { e.Removed = []string{"engineer"} }, "engineer isn't an agent of the Data team to remove"},
		{"an agent kept twice", func(e *agents.TeamEdit) { e.Agents[2].Was = "analyst" }, "two of the team's agents were analyst"},
		{"another team's agent kept", func(e *agents.TeamEdit) { e.Agents[2].Was = "engineer" }, "engineer isn't an agent of the Data team"},
		{"an agent a task needs removed", func(e *agents.TeamEdit) { e.Agents, e.Removed = slices.Delete(e.Agents, 1, 2), []string{"analyst"} },
			"analyst is still needed by task-1: finish those first, or keep analyst"},
		{"an unknown team", func(e *agents.TeamEdit) { e.Team = "data" }, "there is no team called data"},
		{"a team name taken", func(e *agents.TeamEdit) { e.To = "software" }, "there is a team called software already"},
	}
	for _, c := range cases {
		edit := dataAsItIs()
		c.edit(&edit)
		_, rejection, err := s.Propose(edit.Proposal(you))
		if err != nil {
			t.Fatal(err)
		}
		if rejection == nil || rejection.Reason != c.reason {
			t.Errorf("%s: refused with %v, want %q", c.name, rejection, c.reason)
		}
	}
	if roster := state.Roster(); len(roster) != 8 {
		t.Errorf("the roster has %d agents, want the 5 seeded and Data's 3", len(roster))
	}
}

// An edit the rules accept is written whole: swapped names, a removed agent
// no task needs, a new one, and the coordinator and the team's name moved.
func TestATeamEditIsWrittenWhole(t *testing.T) {
	s, state := openSeededState(t)
	dataTeam(t, s)
	edit := dataAsItIs()
	edit.To = "Numbers"
	edit.Agents[0].Coordinates = false
	edit.Agents[1].Name, edit.Agents[1].Coordinates = "data-lead", true // analyst takes data-lead's name, and coordinates
	edit.Agents[0].Name = "analyst"
	edit.Agents = append(edit.Agents[:2], agents.EditedAgent{Name: "plotter", Engine: "claude", Prompt: "Plot."})
	edit.Removed = []string{"charter"}
	if _, rejection, err := s.Propose(edit.Proposal(you)); rejection != nil || err != nil {
		t.Fatalf("the edit: %v %v", rejection, err)
	}
	roster := state.Roster()
	if lead, ok := roster.Coordinator("Numbers"); !ok || lead.Name != "data-lead" || lead.Prompt != "Analyse." {
		t.Errorf("Numbers' coordinator = %+v, want analyst, renamed data-lead", lead)
	}
	if _, ok := roster.Get("charter"); ok || roster.TeamNamed("Data") != "" {
		t.Error("charter, or the Data team, is still there")
	}
}
