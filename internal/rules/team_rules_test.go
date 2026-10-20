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

// dataAsItIs is the Data team as dataTeam makes it, as an edit of its
// version in state keeps it.
func dataAsItIs(state State) agents.TeamEdit {
	return agents.TeamEdit{Version: state.Roster().TeamVersion("Data"), Team: "Data", To: "Data", Agents: []agents.EditedAgent{
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
	edit := dataAsItIs(state)
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
		edit := dataAsItIs(state)
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

// An edit that removes the team's coordinator and names a new one moves
// the tasks the old one only coordinates, so they don't keep it; a turn it
// is taking does, until it ends.
func TestATeamEditReplacingItsCoordinatorRemovesTheOldOne(t *testing.T) {
	s, state := openSeededState(t)
	dataTeam(t, s)
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	turn := func(p events.Proposal) {
		t.Helper()
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatalf("%s: %v %v", p.Type, rejection, err)
		}
	}
	turn(events.Proposal{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}})
	turn(events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "data-lead", "hash": "h"}})
	replaced := func() agents.TeamEdit {
		edit := dataAsItIs(state)
		edit.Agents[0] = agents.EditedAgent{Name: "chief", Engine: "claude", Prompt: "Lead.", Coordinates: true}
		edit.Removed = []string{"data-lead"}
		return edit
	}

	want := "data-lead has a turn running on task-1: wait for it to end, or keep data-lead"
	if _, rejection, err := s.Propose(replaced().Proposal(you)); err != nil || rejection == nil || rejection.Reason != want {
		t.Errorf("replacing data-lead during its turn: refused with %v, %v, want %q", rejection, err, want)
	}
	turn(events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Plot", "repo": "/src/app", "coordinator": "data-lead"}})
	turn(events.Proposal{Stream: "task-2", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w2", "branch": "b2", "port": 7902}})
	turn(events.Proposal{Stream: "task-2", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "data-lead", "hash": "h"}})
	want = "data-lead has turns running on task-1, task-2: wait for them to end, or keep data-lead"
	if _, rejection, err := s.Propose(replaced().Proposal(you)); err != nil || rejection == nil || rejection.Reason != want {
		t.Errorf("replacing data-lead during two turns: refused with %v, %v, want %q", rejection, err, want)
	}
	for _, task := range []string{"task-1", "task-2"} {
		turn(events.Proposal{Stream: task, Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "data-lead", "report": "r"}})
	}
	if _, rejection, err := s.Propose(replaced().Proposal(you)); rejection != nil || err != nil {
		t.Errorf("replacing data-lead, which only coordinates task-1 and task-2: %v %v", rejection, err)
	}
}

// Of the steps handed to the coordinator an edit replaces, one on a task it
// coordinates moves to the new coordinator with the task, so it doesn't keep
// the old one; one handed to it on a task another agent coordinates, before
// it became the coordinator, does, until it is taken.
func TestATeamEditReplacingItsCoordinatorKeepsItForAStepHandedToItBefore(t *testing.T) {
	s, state := openSeededState(t)
	dataTeam(t, s)
	propose := func(p events.Proposal) {
		t.Helper()
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatalf("%s: %v %v", p.Type, rejection, err)
		}
	}
	analystLeads := dataAsItIs(state)
	analystLeads.Agents[0].Coordinates, analystLeads.Agents[1].Coordinates = false, true
	propose(analystLeads.Proposal(you))
	propose(events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Plot", "repo": "/src/app", "coordinator": "analyst"}})

	replaced := dataAsItIs(state)
	replaced.Agents[0].Coordinates = false
	replaced.Agents[1] = agents.EditedAgent{Name: "chief", Engine: "claude", Prompt: "Lead.", Coordinates: true}
	replaced.Removed = []string{"analyst"}
	want := "analyst is still needed by task-1: finish those first, or keep analyst"
	if _, rejection, err := s.Propose(replaced.Proposal(you)); err != nil || rejection == nil || rejection.Reason != want {
		t.Errorf("replacing analyst, handed task-1 by data-lead and task-2's first step: refused with %v, %v, want %q", rejection, err, want)
	}
}

// An edit the rules accept is written whole: swapped names, a removed agent
// no task needs, a new one, and the coordinator and the team's name moved.
func TestATeamEditIsWrittenWhole(t *testing.T) {
	s, state := openSeededState(t)
	dataTeam(t, s)
	edit := dataAsItIs(state)
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

// Of two edits of the same version, the second is refused once the first
// is saved, rather than saving over it.
func TestATeamEditOfAVersionNoLongerTheTeamsIsRefused(t *testing.T) {
	s, state := openSeededState(t)
	dataTeam(t, s)
	first, second := dataAsItIs(state), dataAsItIs(state)
	first.Agents[2].Prompt = "Chart well."
	second.Agents[2].Prompt = "Chart fast."
	if _, rejection, err := s.Propose(first.Proposal(you)); rejection != nil || err != nil {
		t.Fatalf("the first edit: %v %v", rejection, err)
	}
	_, rejection, err := s.Propose(second.Proposal(you))
	if err != nil {
		t.Fatal(err)
	}
	if want := "the Data team changed since you opened it: reload it"; rejection == nil || rejection.Reason != want {
		t.Errorf("the second edit was refused with %v, want %q", rejection, want)
	}
	if charter, _ := state.Roster().Get("charter"); charter.Prompt != "Chart well." {
		t.Errorf("charter's prompt is %q, want the first edit's", charter.Prompt)
	}
}

// A team is linked to a built-in template or to one of yours, saved or
// removed: the teams of a removed one keep its base. A link of a team the
// roster hasn't, or to no template it knows, is refused.
func TestATeamIsLinkedToATemplateItKnows(t *testing.T) {
	s := openSeeded(t)
	dataTeam(t, s)
	for _, p := range []events.Proposal{
		savingTemplate("papers", "Readers", research()),
		savingTemplate("old-papers", "Old Readers", research()),
		{Stream: events.TemplatesStream, Actor: you, Type: events.TemplateRemoved, Data: map[string]any{"id": "old-papers"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatalf("%s: %v %v", p.Type, rejection, err)
		}
	}
	cases := []struct {
		name, team, template, want string
	}{
		{"built in", "Data", "research", ""},
		{"yours", "Data", "papers", ""},
		{"yours, removed", agents.DefaultTeam, "old-papers", ""},
		{"no such team", "Dreamers", "research", "there is no team called Dreamers"},
		{"a team in another case", "data", "research", "there is no team called data"},
		{"no such template", "Data", "dreamers", "there is no template with the id dreamers"},
		{"no template", "Data", "", "a team is linked to a template, and this link names none"},
	}
	for _, c := range cases {
		_, rejection, err := s.Propose(agents.TeamLinked(c.team, c.template, you))
		if err != nil {
			t.Fatal(err)
		}
		if reason := reasonOf(rejection); reason != c.want {
			t.Errorf("%s: refused with %q, want %q", c.name, reason, c.want)
		}
	}
}

func reasonOf(rejection *events.Rejection) string {
	if rejection == nil {
		return ""
	}
	return rejection.Reason
}
