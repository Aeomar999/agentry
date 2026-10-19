package coordinator

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// Software and a Visionaries team on a real loop. The Visionaries team is
// Writing's agents by another name, so its coordinator is editor.
func withVisionaries(t *testing.T) (*store.Store, string) {
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
	visionaries, _ := agents.TemplateNamed("Writing")
	visionaries.Team = "visionaries" // in any case
	for _, p := range agents.AddTeam(visionaries, agents.FromEvents(mustStream(t, s, events.AgentsStream)), you) {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go newLoop(s, t.TempDir()).Run(ctx)
	return s, repo
}

func startTask(t *testing.T, s *store.Store, id, repo, coordinator string) *tasks.Task {
	t.Helper()
	t.Cleanup(func() { sessions.Stop(sessions.SocketFor(id)) })
	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Think up features", "repo": repo, "coordinator": coordinator, "teamChosen": true}})
	return waitForTurn(t, s, id, "turn-1", coordinator)
}

func TestAVisionariesDoneKeepsItsIdeas(t *testing.T) {
	s, repo := withVisionaries(t)
	const id = "task-vision-1"
	task := startTask(t, s, id, repo, "editor")
	if brief := readBrief(t, task, "turn-1"); !strings.Contains(brief, IdeasFormat) {
		t.Errorf("the editor's brief should say how to give ideas:\n%s", brief)
	}

	endTurn(t, s, id, "turn-1", "editor", `{"action": "done", "summary": "Two ideas.", "ideas": [
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M", "backedBy": ["dreamer"], "doubtedBy": ["skeptic"]},
		{"title": "Dark mode", "problem": "Bright at night", "proposal": "A dark theme", "size": "S", "kind": "epic"}]}`)
	task = waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if task.Summary != "Two ideas." {
		t.Errorf("summary %q", task.Summary)
	}
	list, err := ideas.List(s, repo)
	if err != nil || len(list) != 2 {
		t.Fatalf("ideas: %v %v", list, err)
	}
	for _, idea := range list {
		if idea.State != ideas.Open || idea.Task != id || idea.Repo != repo {
			t.Errorf("%s: %s, from %q, about %q", idea.Title, idea.State, idea.Task, idea.Repo)
		}
	}
	if search := list[1]; search.Title != "Search the board" || !slices.Equal(search.BackedBy, []string{"dreamer"}) || list[0].Kind != tasks.Epic {
		t.Errorf("ideas, newest first: %+v, %+v", list[0], search)
	}
}

func TestARefusedIdeaDoesntStopTheDone(t *testing.T) {
	s, repo := withVisionaries(t)
	const id = "task-vision-2"
	startTask(t, s, id, repo, "editor")

	endTurn(t, s, id, "turn-1", "editor", `{"action": "done", "summary": "One good idea.", "ideas": [
		{"title": "Huge", "problem": "p", "proposal": "f", "size": "XL"},
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M"}]}`)
	task := waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if !strings.HasPrefix(task.Summary, "One good idea.") || !strings.Contains(task.Summary, `"Huge": an idea's size is S, M or L, not XL`) {
		t.Errorf("the summary should say which idea was refused:\n%s", task.Summary)
	}
	if list, _ := ideas.List(s, repo); len(list) != 1 || list[0].Title != "Search the board" {
		t.Errorf("ideas: %v", list)
	}
}

// Only a Visionaries session suggests ideas: a Software done carrying some
// goes through and keeps none.
func TestASoftwareDoneKeepsNoIdeas(t *testing.T) {
	s, repo := withVisionaries(t)
	const id = "task-vision-6"
	startTask(t, s, id, repo, "tech-lead")

	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "done", "summary": "Built it.", "ideas": [
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M"}]}`)
	task := waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if task.Summary != "Built it." {
		t.Errorf("summary %q", task.Summary)
	}
	if list, err := ideas.List(s, repo); err != nil || len(list) != 0 {
		t.Errorf("ideas: %v %v", list, err)
	}
}

// The story's "dismissals reach a Visionaries brief": a Visionaries
// coordinator reads what you dismissed in its repo, and a Software one
// doesn't, nor how to suggest ideas.
func TestAVisionariesBriefNamesTheDismissedIdeas(t *testing.T) {
	s, repo := withVisionaries(t)
	startTask(t, s, "task-vision-3", repo, "editor")
	endTurn(t, s, "task-vision-3", "turn-1", "editor", `{"action": "done", "summary": "One idea.", "ideas": [
		{"title": "Dark mode", "problem": "Bright at night", "proposal": "A dark theme", "size": "S"}]}`)
	waitUntil(t, s, "task-vision-3", "done", func(task *tasks.Task) bool { return task.IsDone })
	if _, rejection, err := s.Propose(events.Proposal{Stream: "idea-1", Actor: you, Type: events.IdeaDismissed,
		Data: map[string]any{"note": "The OS does it"}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}

	vision := startTask(t, s, "task-vision-4", repo, "editor")
	if brief := readBrief(t, vision, "turn-1"); !strings.Contains(brief, "## Ideas the user dismissed\n\n- Dark mode: The OS does it") {
		t.Errorf("the editor's brief should name the dismissed idea:\n%s", brief)
	}
	software := startTask(t, s, "task-vision-5", repo, "tech-lead")
	if brief := readBrief(t, software, "turn-1"); strings.Contains(brief, "Dark mode") || strings.Contains(brief, `"ideas"`) {
		t.Errorf("a Software brief has no ideas:\n%s", brief)
	}
}
