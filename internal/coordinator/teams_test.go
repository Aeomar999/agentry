package coordinator

import (
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
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
	for _, p := range agents.CustomTeam("Data", members, you) {
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
