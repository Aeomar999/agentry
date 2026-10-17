package coordinator

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// Two teams on a real loop: Software and Writing. The test plays the
// coordinators, ending their turns with decisions.
func twoTeams(t *testing.T) (*store.Store, context.CancelFunc, string) {
	t.Helper()
	fakeClaudeOnPath(t)
	repo := gitRepo(t)
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), rules.Check)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	writing, _ := agents.TemplateNamed("Writing")
	stream, _ := s.Stream(events.AgentsStream)
	for _, p := range agents.AddTeam(writing, agents.FromEvents(stream), you) {
		s.Propose(p)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go newLoop(s, t.TempDir()).Run(ctx)
	return s, cancel, repo
}

func TestACoordinatorAsksAnotherTeamAndCarriesOnWithTheAnswer(t *testing.T) {
	s, cancel, repo := twoTeams(t)
	defer cancel()
	const id = "task-ask-1"
	defer sessions.Stop(sessions.SocketFor(id))
	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Ship the TODO tool", "repo": repo, "coordinator": "tech-lead", "teamChosen": true}})

	task := waitForTurn(t, s, id, "turn-1", "tech-lead")
	if brief := readBrief(t, task, "turn-1"); !strings.Contains(brief, "- Writing, coordinated by editor: ") || strings.Contains(brief, "didn't choose a team") {
		t.Errorf("the tech lead's brief should list the Writing team, and say nothing about choosing one:\n%s", brief)
	}
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "request", "team": "writing", "request": "Write the user guide for the TODO tool.\nKeep it to one page."}`)

	task = waitUntil(t, s, id, "the request", func(task *tasks.Task) bool { return task.WaitingOn != "" })
	child := task.WaitingOn
	defer sessions.Stop(sessions.SocketFor(child))
	childTask := waitForTurn(t, s, child, "turn-1", "editor")
	if childTask.Parent != id || childTask.Title != "Write the user guide for the TODO tool." || !strings.Contains(childTask.Description, "for "+id) {
		t.Errorf("the Writing team's task: parent %q, title %q, description %q", childTask.Parent, childTask.Title, childTask.Description)
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
	s, cancel, repo := twoTeams(t)
	defer cancel()
	const id = "task-route-1"
	defer sessions.Stop(sessions.SocketFor(id))
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
