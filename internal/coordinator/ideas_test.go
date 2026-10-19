package coordinator

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// Software and a Visionaries team on a real loop. The Visionaries team is
// Writing's agents made as the Visionaries' template, so its coordinator is
// editor.
func withVisionaries(t *testing.T) (*store.Store, *current.State, string) {
	t.Helper()
	s, state, repo := withTeams(t, true)
	runLoop(t, s, state)
	return s, state, repo
}

// withTeams is a store with the Software team, and the Visionaries if
// visionaries, and a repo, with no loop running yet.
func withTeams(t *testing.T, visionaries bool) (*store.Store, *current.State, string) {
	t.Helper()
	fakeClaudeOnPath(t)
	repo := gitRepo(t)
	s, state := storetest.OpenWith(t, filepath.Join(t.TempDir(), "agentry.db"), storetest.WithIdeas())
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	if !visionaries {
		return s, state, repo
	}
	team, _ := agents.TemplateNamed("Writing")
	team.ID, team.Team = "visionaries", "visionaries" // its name in any case
	for _, p := range agents.AddTeam(team, agents.FromEvents(mustStream(t, s, events.AgentsStream)), you) {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	return s, state, repo
}

// runLoop runs a loop on the store until the test ends.
func runLoop(t *testing.T, s *store.Store, state *current.State) {
	t.Helper()
	run(t, newLoop(s, state, t.TempDir()), state)
}

// run runs the loop until the test ends.
func run(t *testing.T, loop *Loop, state *current.State) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		loop.Run(ctx)
	}()
	// Registered after the temp dirs and the store, so it runs before them.
	// The loop stops first: it may still be removing a done task's worktree
	// in the repo, or starting a session that stopping would leave it
	// waiting on.
	t.Cleanup(func() {
		cancel()
		<-stopped
		for _, id := range state.TaskIDs() {
			sessions.Stop(sessions.SocketFor(id))
		}
	})
}

func startTask(t *testing.T, s *store.Store, id, repo, coordinator string) *tasks.Task {
	t.Helper()
	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Think up features", "repo": repo, "coordinator": coordinator, "teamChosen": true}})
	return waitForTurn(t, s, id, "turn-1", coordinator)
}

func TestAVisionariesDoneKeepsItsIdeas(t *testing.T) {
	s, state, repo := withVisionaries(t)
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
	list := ideas.List(state, repo)
	if len(list) != 2 {
		t.Fatalf("ideas: %v", list)
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
	s, state, repo := withVisionaries(t)
	const id = "task-vision-2"
	startTask(t, s, id, repo, "editor")

	endTurn(t, s, id, "turn-1", "editor", `{"action": "done", "summary": "One good idea.", "ideas": [
		{"title": "Huge", "problem": "p", "proposal": "f", "size": "XL"},
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M"}]}`)
	task := waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if !strings.HasPrefix(task.Summary, "One good idea.") || !strings.Contains(task.Summary, `"Huge": an idea's size is S, M or L, not XL`) {
		t.Errorf("the summary should say which idea was refused:\n%s", task.Summary)
	}
	if list := ideas.List(state, repo); len(list) != 1 || list[0].Title != "Search the board" {
		t.Errorf("ideas: %v", list)
	}
}

func TestADoneNamingOneTitleTwiceKeepsOneIdea(t *testing.T) {
	s, state, repo := withVisionaries(t)
	const id = "task-vision-7"
	startTask(t, s, id, repo, "editor")

	endTurn(t, s, id, "turn-1", "editor", `{"action": "done", "summary": "One idea, twice.", "ideas": [
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M"},
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M"}]}`)
	waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if list := ideas.List(state, repo); len(list) != 1 {
		t.Errorf("ideas: %v", list)
	}
}

// An idea that says it repeats an idea the repo doesn't have, here a title
// in place of the id, is kept as a new idea, and the summary says so.
func TestAnIdeaRepeatingNoIdeaIsKeptAsANewOne(t *testing.T) {
	s, state, repo := withVisionaries(t)
	const id = "task-vision-8"
	startTask(t, s, id, repo, "editor")

	endTurn(t, s, id, "turn-1", "editor", `{"action": "done", "summary": "One idea.", "ideas": [
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M", "repeats": "Board search"}]}`)
	task := waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	const note = `"Search the board" repeats Board search, which doesn't exist; kept as a new idea.`
	if task.Summary != "One idea.\n\n"+note {
		t.Errorf("the summary should say the idea was kept as a new one:\n%s", task.Summary)
	}
	list := ideas.List(state, repo)
	if len(list) != 1 {
		t.Fatalf("ideas: %v", list)
	}
	if idea := list[0]; idea.Title != "Search the board" || idea.Repeats != "" || idea.State != ideas.Open {
		t.Errorf("want the idea kept, open, repeating nothing: %+v", idea)
	}
}

// Only a Visionaries session suggests ideas: a Software done carrying some
// goes through, keeps none and says so.
func TestASoftwareDoneKeepsNoIdeas(t *testing.T) {
	s, state, repo := withVisionaries(t)
	const id = "task-vision-6"
	startTask(t, s, id, repo, "tech-lead")

	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "done", "summary": "Built it.", "ideas": [
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M"}]}`)
	task := waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if !strings.HasPrefix(task.Summary, "Built it.\n\n") || !strings.Contains(task.Summary, "couldn't keep these ideas") {
		t.Errorf("the summary should say the ideas weren't kept:\n%s", task.Summary)
	}
	if list := ideas.List(state, repo); len(list) != 0 {
		t.Errorf("ideas: %v", list)
	}
}

// A done from a coordinator the roster no longer has, here renamed while its
// session ran, keeps no ideas and says so.
func TestADoneFromACoordinatorNoLongerThereSaysItsIdeasWerentKept(t *testing.T) {
	s, state, repo := withVisionaries(t)
	const id = "task-vision-8"
	startTask(t, s, id, repo, "editor")
	if _, rejection, err := s.Propose(agents.Agent{Name: "editor"}.Renaming(you, "chief-editor")); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}

	endTurn(t, s, id, "turn-1", "editor", `{"action": "done", "summary": "One idea.", "ideas": [
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M"}]}`)
	task := waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if !strings.Contains(task.Summary, "couldn't keep these ideas") {
		t.Errorf("the summary should say the ideas weren't kept:\n%s", task.Summary)
	}
	if list := ideas.List(state, repo); len(list) != 0 {
		t.Errorf("ideas: %v", list)
	}
}

// The story's "dismissals reach a Visionaries brief": a Visionaries
// coordinator reads what you dismissed and accepted in its repo, and a Software one
// doesn't, nor how to suggest ideas.
func TestAVisionariesBriefNamesTheDismissedIdeas(t *testing.T) {
	s, state, repo := withVisionaries(t)
	startTask(t, s, "task-vision-3", repo, "editor")
	endTurn(t, s, "task-vision-3", "turn-1", "editor", `{"action": "done", "summary": "One idea.", "ideas": [
		{"title": "Dark mode", "problem": "Bright at night", "proposal": "A dark theme", "size": "S"}]}`)
	waitUntil(t, s, "task-vision-3", "done", func(task *tasks.Task) bool { return task.IsDone })
	if _, rejection, err := s.Propose(events.Proposal{Stream: "idea-1", Actor: you, Type: ideas.IdeaDismissed,
		Data: map[string]any{"note": "The OS does it"}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}

	vision := startTask(t, s, "task-vision-4", repo, "editor")
	if brief := readBrief(t, vision, "turn-1"); !strings.Contains(brief, "## Ideas the user dismissed\n\n- Dark mode (idea-1): The OS does it") {
		t.Errorf("the editor's brief should name the dismissed idea:\n%s", brief)
	}
	accepted := mustCreate(t, s, events.Proposal{Actor: agentry, Type: ideas.IdeaSuggested, Data: map[string]any{"title": "Search",
		"problem": "p", "proposal": "f", "size": "S", "task": "task-vision-3", "repo": repo}})
	got, err := ideas.Accept(s, state, accepted, ideas.Choice{})
	if err != nil || !got.Accepted {
		t.Fatalf("accepting: %+v %v", got, err)
	}
	again := startTask(t, s, "task-vision-6", repo, "editor")
	if brief := readBrief(t, again, "turn-1"); !strings.Contains(brief, "## Ideas already accepted\n\n- Search (idea-2): became "+got.Task) ||
		!strings.Contains(brief, `"repeats"`) {
		t.Errorf("the editor's brief should name the accepted idea and how to repeat one:\n%s", brief)
	}
	software := startTask(t, s, "task-vision-5", repo, "tech-lead")
	if brief := readBrief(t, software, "turn-1"); strings.Contains(brief, "Dark mode") || strings.Contains(brief, `"ideas"`) {
		t.Errorf("a Software brief has no ideas:\n%s", brief)
	}
}

// A team suggests ideas because of the template it was made from, not its
// name: a second Visionaries team does, a team you made and called
// Visionaries doesn't, and a log from before agents recorded their template
// still has its Visionaries teams.
func TestATeamSuggestsIdeasByItsTemplate(t *testing.T) {
	visionaries, _ := agents.TemplateNamed(ideas.VisionariesTeam)
	var log []events.Event
	add := func(p events.Proposal) { log = append(log, events.Event{Proposal: p}) }
	for _, p := range agents.SeedProposals(nil) {
		add(p)
	}
	for range 2 {
		for _, p := range agents.AddTeam(visionaries, agents.FromEvents(log), you) {
			add(p)
		}
	}
	add(agents.Agent{Name: "lead", Team: "visionaries", Coordinates: true, Engine: "claude", Prompt: "Think."}.Proposal(you, false))
	for _, team := range []string{"Old Visionaries", "Old Visionaries 2"} {
		p := agents.Agent{Name: team, Team: strings.TrimPrefix(team, "Old "), Coordinates: true, Engine: "claude", Prompt: "Think."}.Proposal(you, false)
		delete(p.Data, "template") // as Agentry wrote it before the field
		add(p)
	}
	roster := agents.FromEvents(log)
	for name, want := range map[string]bool{"tech-lead": false, "visionary-lead": true, "visionary-lead-2": true, "lead": false,
		"Old Visionaries": true, "Old Visionaries 2": true} {
		if coordinator, _ := roster.Get(name); ideas.Suggests(coordinator) != want {
			t.Errorf("%s of team %q (template %q): suggests ideas %v, want %v", name, coordinator.TeamName(), coordinator.Template, !want, want)
		}
	}
}
