package coordinator

import (
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// Two teams on a real loop: Software and Writing. The test plays the
// coordinators, ending their turns with decisions.
func twoTeams(t *testing.T) (*store.Store, string) {
	t.Helper()
	fakeClaudeOnPath(t)
	repo := gitRepo(t)
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	writing, _ := agents.TemplateNamed("Writing")
	stream, _ := s.Stream(events.AgentsStream)
	for _, p := range agents.AddTeam(writing, agents.FromEvents(stream), you) {
		s.Propose(p)
	}
	run(t, newLoop(s, state, t.TempDir()), state)
	return s, repo
}

func TestACoordinatorAsksAnotherTeamAndCarriesOnWithTheAnswer(t *testing.T) {
	s, repo := twoTeams(t)
	const id = "task-1"
	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Ship the TODO tool", "repo": repo, "coordinator": "tech-lead", "teamChosen": true, "budget": 900_000, "maxTurns": 9}})

	task := waitForTurn(t, s, id, "turn-1", "tech-lead")
	if brief := readBrief(t, task, "turn-1"); !strings.Contains(brief, "- Writing, coordinated by editor: ") || strings.Contains(brief, "didn't choose a team") {
		t.Errorf("the tech lead's brief should list the Writing team, and say nothing about choosing one:\n%s", brief)
	}
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "request", "team": "writing", "request": "Write the user guide for the TODO tool.\nKeep it to one page."}`)

	task = waitUntil(t, s, id, "the request", func(task *tasks.Task) bool { return task.WaitingOn != "" })
	child := task.WaitingOn
	childTask := waitForTurn(t, s, child, "turn-1", "editor")
	if childTask.Parent != id || childTask.Title != "Write the user guide for the TODO tool." || !strings.Contains(childTask.Description, "for "+id) {
		t.Errorf("the Writing team's task: parent %q, title %q, description %q", childTask.Parent, childTask.Title, childTask.Description)
	}
	if limits := childTask.Limits(); limits != task.Limits() {
		t.Errorf("the request takes its asker's limits %+v: %+v", task.Limits(), limits)
	}
	if task.Status() != tasks.Working || task.OpenTurn() != nil {
		t.Errorf("the asking task waits, with no turn of its own: %s", task.Status())
	}

	// The editor finishes: the summary goes back, and the tech lead carries on.
	endTurn(t, s, child, "turn-1", "editor", `{"action": "done", "summary": "The guide is in docs/guide.md."}`)
	task = waitForTurn(t, s, id, "turn-2", "tech-lead")
	if brief := readBrief(t, task, "turn-2"); !strings.Contains(brief, child+" is done: The guide is in docs/guide.md.") {
		t.Errorf("the tech lead should read the Writing team's summary:\n%s", brief)
	}

	// Asking a team that isn't there is an issue to fix, not a stall.
	endTurn(t, s, id, "turn-2", "tech-lead", `{"action": "request", "team": "Legal", "request": "Check the licence."}`)
	task = waitForTurn(t, s, id, "turn-3", "tech-lead")
	if brief := readBrief(t, task, "turn-3"); !strings.Contains(brief, "there is no team called Legal") {
		t.Errorf("the tech lead should hear why:\n%s", brief)
	}
}

func TestATaskWithNoTeamChosenGoesToTheRightOne(t *testing.T) {
	s, repo := twoTeams(t)
	const id = "task-1"
	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Write a launch blog post", "repo": repo, "coordinator": "tech-lead"}})

	task := waitForTurn(t, s, id, "turn-1", "tech-lead")
	if brief := readBrief(t, task, "turn-1"); !strings.Contains(brief, "The user didn't choose a team") {
		t.Errorf("the brief should ask the tech lead whose work it is:\n%s", brief)
	}
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "transfer", "team": "Writing", "reason": "It is a blog post."}`)
	task = waitForTurn(t, s, id, "turn-2", "editor")
	if task.Coordinator != "editor" || !task.TeamChosen {
		t.Errorf("the editor takes it: coordinator %q", task.Coordinator)
	}
	if brief := readBrief(t, task, "turn-2"); !strings.Contains(brief, "This task is your team's work: It is a blog post.") || strings.Contains(brief, "didn't choose a team") {
		t.Errorf("the editor's brief:\n%s", brief)
	}
}

// A team you define takes tasks like a template's: the tech lead's brief
// lists it under Other teams, and a task passed to it goes to its lead.
func TestACustomTeamTakesTasksLikeAnyOther(t *testing.T) {
	s, repo := twoTeams(t)
	stream, _ := s.Stream(events.AgentsStream)
	members := []agents.Agent{
		{Name: "data-lead", Description: "Leads the data work.", Engine: "claude", Coordinates: true},
		{Name: "analyst", Description: "Analyses the numbers.", Engine: "claude"},
	}
	if problem := agents.CheckCustomTeam("Data", members, agents.FromEvents(stream)); problem != "" {
		t.Fatal(problem)
	}
	for _, p := range agents.CustomTeam("Data", "", members, you) {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	const id = "task-1"
	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Chart the sign-ups", "repo": repo, "coordinator": "tech-lead"}})

	task := waitForTurn(t, s, id, "turn-1", "tech-lead")
	if brief := readBrief(t, task, "turn-1"); !strings.Contains(brief, "- Data, coordinated by data-lead: Leads the data work.") {
		t.Errorf("the tech lead's brief should list the Data team:\n%s", brief)
	}
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "transfer", "team": "data", "reason": "It is data work."}`)
	if task = waitForTurn(t, s, id, "turn-2", "data-lead"); task.Coordinator != "data-lead" {
		t.Errorf("the Data team's lead takes it: coordinator %q", task.Coordinator)
	}
}

// A team renamed is listed by its new name in other coordinators' briefs,
// and takes tasks by it.
func TestARenamedTeamIsListedByItsNewName(t *testing.T) {
	s, repo := twoTeams(t)
	if _, rejection, err := s.Propose(agents.TeamRenaming("Writing", "Docs", you)); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	const id = "task-1"
	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Ship the TODO tool", "repo": repo, "coordinator": "tech-lead"}})

	task := waitForTurn(t, s, id, "turn-1", "tech-lead")
	if brief := readBrief(t, task, "turn-1"); !strings.Contains(brief, "- Docs, coordinated by editor: ") || strings.Contains(brief, "- Writing, ") {
		t.Errorf("the tech lead's brief should list the Docs team, and not Writing:\n%s", brief)
	}
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "transfer", "team": "docs", "reason": "It is writing."}`)
	if task = waitForTurn(t, s, id, "turn-2", "editor"); task.Coordinator != "editor" {
		t.Errorf("the Docs team's editor takes it: coordinator %q", task.Coordinator)
	}
}

// rosterWith is the default roster after these proposals, in order.
func rosterWith(proposals ...events.Proposal) agents.Roster {
	roster := defaultRoster()
	for _, p := range proposals {
		roster = roster.Next(events.Event{Proposal: p})
	}
	return roster
}

// withTemplates is the default roster with a team added from each of these
// built-in templates.
func withTemplates(ids ...string) agents.Roster {
	roster := defaultRoster()
	for _, id := range ids {
		template, _ := agents.TemplateWithID(id)
		for _, p := range agents.AddTeam(template, roster, you) {
			roster = roster.Next(events.Event{Proposal: p})
		}
	}
	return roster
}

// otherTeamsOf is the "Other teams" section of lead's brief.
func otherTeamsOf(t *testing.T, roster agents.Roster, lead string) string {
	t.Helper()
	coordinator, ok := roster.Get(lead)
	if !ok {
		t.Fatalf("no %s in the roster", lead)
	}
	brief := coordinatorBrief(started().task(), coordinator, roster, family{}, nil, nil, nil, nil)
	_, section, _ := strings.Cut(brief, "## Other teams\n\n")
	section, _, _ = strings.Cut(section, "\n\n## ")
	return section
}

// With today's teams the routing sentence reads as it always has.
func TestTheRoutingSentenceOfTheDefaultTeamsIsUnchanged(t *testing.T) {
	roster := withTemplates("writing", "research", "design", "visionaries", "networking")
	want := "You are in the Software team. When this task needs something only another team does well (a document " +
		"from Writing, research from Research, a design from Design, code from Software), ask them with a \"request\" " +
		"decision: they get a task of their own, this one waits for it, and their summary comes back to you here, so " +
		"you carry on. Ask for one thing at a time, and say exactly what you need and what done looks like."
	if section := otherTeamsOf(t, roster, "tech-lead"); !strings.Contains(section, "\n\n"+want+"\n\n") {
		t.Errorf("the routing sentence changed:\n%s", section)
	}
}

// The routing sentence names only the teams there are, by their names now.
func TestTheRoutingSentenceNamesTheTeamsThereAre(t *testing.T) {
	data := rosterWith(agents.CustomTeam("Data", "", []agents.Agent{
		{Name: "data-lead", Description: "Leads the data work.", Engine: "claude", Coordinates: true},
	}, you)...)
	if section := otherTeamsOf(t, data, "data-lead"); !strings.Contains(section, "only another team does well (code from Software), ask") {
		t.Errorf("Software only, plus a custom team:\n%s", section)
	}
	docs := withTemplates("writing", "research", "design")
	docs = docs.Next(events.Event{Proposal: agents.TeamRenaming("Writing", "Docs", you)})
	if section := otherTeamsOf(t, docs, "tech-lead"); !strings.Contains(section, "(a document from Docs, research from Research, a design from Design, code from Software)") {
		t.Errorf("a renamed Writing team:\n%s", section)
	}
	alone := rosterWith(agents.CustomTeam("Legal", "", []agents.Agent{
		{Name: "counsel", Engine: "claude", Coordinates: true},
	}, you)...)
	alone = alone.Next(events.Event{Proposal: agents.TeamRenaming("Software", "Build", you)})
	if section := otherTeamsOf(t, alone, "counsel"); !strings.Contains(section, "(code from Build)") {
		t.Errorf("a renamed Software team:\n%s", section)
	}
}

// A team's line comes from the template its coordinator was made from, not
// its name, and ends without a colon when there is nothing to say.
func TestOtherTeamsAreDescribedByTheirTemplateOrTheirCoordinator(t *testing.T) {
	writing, _ := agents.TemplateWithID("writing")
	renamed := withTemplates("writing")
	renamed = renamed.Next(events.Event{Proposal: agents.TeamRenaming("Writing", "Docs", you)})
	if section := otherTeamsOf(t, renamed, "tech-lead"); !strings.Contains(section, "- Docs, coordinated by editor: "+writing.Description+"\n") {
		t.Errorf("a renamed Writing team keeps its template's description:\n%s", section)
	}

	custom := rosterWith(
		append(agents.CustomTeam("Data", "", []agents.Agent{{Name: "data-lead", Description: "Leads the data work.", Engine: "claude", Coordinates: true}}, you),
			agents.CustomTeam("Legal", "", []agents.Agent{{Name: "counsel", Engine: "claude", Coordinates: true}}, you)...)...)
	section := otherTeamsOf(t, custom, "tech-lead")
	if !strings.Contains(section, "- Data, coordinated by data-lead: Leads the data work.\n") {
		t.Errorf("a custom team is described by its coordinator:\n%s", section)
	}
	if !strings.Contains(section, "- Legal, coordinated by counsel\n") {
		t.Errorf("a team with no description has no colon:\n%s", section)
	}

	// A custom team named like a template isn't described as one.
	named := rosterWith(agents.CustomTeam("Writing", "", []agents.Agent{
		{Name: "scribe", Description: "Takes notes.", Engine: "claude", Coordinates: true},
	}, you)...)
	if section := otherTeamsOf(t, named, "tech-lead"); !strings.Contains(section, "- Writing, coordinated by scribe: Takes notes.\n") {
		t.Errorf("a custom Writing team is described by its own coordinator:\n%s", section)
	}
}

// An agent with no description is listed by its name alone.
func TestAnAgentWithNoDescriptionIsListedByName(t *testing.T) {
	roster := rosterWith(agents.CustomTeam("Legal", "", []agents.Agent{
		{Name: "counsel", Engine: "claude", Coordinates: true},
		{Name: "drafter", Engine: "claude"},
		{Name: "checker", Description: "Checks the drafts.", Engine: "claude"},
	}, you)...)
	counsel, _ := roster.Get("counsel")
	brief := coordinatorBrief(started().task(), counsel, roster, family{}, nil, nil, nil, nil)
	if !strings.Contains(brief, "## The agents you can hand work to\n\n- drafter\n- checker: Checks the drafts.\n") {
		t.Errorf("drafter should be listed by name alone:\n%s", brief)
	}
}

// A team linked to a template of yours is routed to by the template's base.
func TestTheRoutingSentenceFollowsYourTemplatesBase(t *testing.T) {
	roster := rosterWith(append(agents.CustomTeam("Docs", "", []agents.Agent{
		{Name: "docs-lead", Engine: "claude", Coordinates: true},
	}, you), agents.TeamLinked("Docs", "handbook", you))...)
	bases := agents.Bases{Yours: agents.UserTemplates{{ID: "handbook", Team: "Handbook", BasedOn: "writing"}}}
	if got := routingExamples(roster, bases); got != " (a document from Docs, code from Software)" {
		t.Errorf("a team linked to your Writing-based template: %q", got)
	}
}

// A team linked to a template you removed is still routed to by its base.
func TestTheRoutingSentenceFollowsARemovedTemplatesBase(t *testing.T) {
	roster := rosterWith(append(agents.CustomTeam("Docs", "", []agents.Agent{
		{Name: "docs-lead", Engine: "claude", Coordinates: true},
	}, you), agents.TeamLinked("Docs", "handbook", you))...)
	bases := agents.Bases{Removed: agents.RemovedTemplates{"handbook": "writing"}}
	if got := routingExamples(roster, bases); got != " (a document from Docs, code from Software)" {
		t.Errorf("a team linked to your removed Writing-based template: %q", got)
	}
}

// The brief finds your templates' bases in the state it is given.
func TestTheBriefRoutesByYourTemplatesBaseInTheState(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	docs := []agents.Agent{{Name: "docs-lead", Prompt: "You lead the docs.", Engine: "claude", Coordinates: true}}
	for _, p := range agents.CustomTeam("Docs", "", docs, you) {
		propose(t, s, p)
	}
	handbook := agents.UserTemplate{ID: "handbook", Team: "Handbook", Agents: docs, From: "Docs", BasedOn: "writing"}
	propose(t, s, handbook.Save(you))
	propose(t, s, agents.TeamLinked("Docs", "handbook", you))

	lead, _ := state.Roster().Get("tech-lead")
	brief := coordinatorBrief(started().task(), lead, state.Roster(), family{}, nil, nil, state, nil)
	if !strings.Contains(brief, "(a document from Docs, code from Software)") {
		t.Errorf("Docs is linked to your Writing-based template:\n%s", brief)
	}
}
