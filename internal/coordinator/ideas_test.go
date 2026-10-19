package coordinator

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
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
	const id = "task-1"
	task := startTask(t, s, id, repo, "editor")
	if brief := readBrief(t, task, "turn-1"); !strings.Contains(brief, ideas.Feature().Coordinator.Decision) {
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
	const id = "task-1"
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
	const id = "task-1"
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
	const id = "task-1"
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

// unclaimedIdeas is the note on a done carrying ideas from a coordinator
// whose features don't read them.
const unclaimedIdeas = `Agentry couldn't keep "ideas", as no feature of this coordinator reads it.`

// Only a Visionaries session suggests ideas: a Software done carrying some
// goes through, keeps none and says so.
func TestASoftwareDoneKeepsNoIdeas(t *testing.T) {
	s, state, repo := withVisionaries(t)
	const id = "task-1"
	startTask(t, s, id, repo, "tech-lead")

	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "done", "summary": "Built it.", "ideas": [
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M"}]}`)
	task := waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if task.Summary != "Built it.\n\n"+unclaimedIdeas {
		t.Errorf("the summary should say the ideas weren't kept:\n%s", task.Summary)
	}
	if kept := state.OwnedIDs(ideas.Prefix); len(kept) != 0 {
		t.Errorf("no idea should be written: %v", kept)
	}
}

// A feature whose OnDone panics costs the done only its own note: the task
// is done, and the summary says the field wasn't kept and why.
func TestAPanickingOnDoneIsANoteAndTheDoneGoesThrough(t *testing.T) {
	s, state, repo := withTeams(t, true)
	loop := newLoop(s, state, t.TempDir())
	loop.Features = append(loop.Features, features.Feature{Name: "votes", Coordinator: &features.Coordinator{
		For: func(agents.Agent) bool { return true }, Field: "votes",
		OnDone: func(features.Core, *tasks.Task, json.RawMessage) string { panic("a bad hook") }}})
	run(t, loop, state)
	const id = "task-1"
	startTask(t, s, id, repo, "tech-lead")

	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "done", "summary": "Built it.", "votes": [1, 2]}`)
	task := waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if want := "Built it.\n\n" + `Agentry couldn't keep "votes": the votes feature's OnDone failed: a bad hook`; task.Summary != want {
		t.Errorf("summary:\n got %q\nwant %q", task.Summary, want)
	}
}

// A Visionaries done whose ideas Agentry can't read, here "backedBy" a name
// rather than a list, isn't applied: the coordinator is told why and asked
// again, and nothing is written, so the session's ideas aren't lost.
func TestADoneWithUnreadableIdeasIsSentBack(t *testing.T) {
	s, state, repo := withVisionaries(t)
	const id = "task-1"
	startTask(t, s, id, repo, "editor")

	endTurn(t, s, id, "turn-1", "editor", `{"action": "done", "summary": "One idea.", "ideas": [
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M", "backedBy": "dreamer"}]}`)
	task := waitForTurn(t, s, id, "turn-2", "editor")
	if task.IsDone {
		t.Error("the done should not be applied")
	}
	if reason := lastIssue(t, s, id); !strings.Contains(reason, `no usable decision: Agentry couldn't read its "ideas": json: `) {
		t.Errorf("the issue should name the field and why: %q", reason)
	}
	if kept := state.OwnedIDs(ideas.Prefix); len(kept) != 0 {
		t.Errorf("no idea should be written: %v", kept)
	}
}

// A feature whose Check panics doesn't hold up the done: it goes through,
// as if the Check had passed.
func TestAPanickingCheckLetsTheDoneThrough(t *testing.T) {
	s, state, repo := withTeams(t, true)
	loop := newLoop(s, state, t.TempDir())
	loop.Features = append(loop.Features, features.Feature{Name: "votes", Coordinator: &features.Coordinator{
		For: func(agents.Agent) bool { return true }, Field: "votes",
		Check:  func(json.RawMessage) error { panic("a bad hook") },
		OnDone: func(features.Core, *tasks.Task, json.RawMessage) string { return "" }}})
	run(t, loop, state)
	const id = "task-1"
	startTask(t, s, id, repo, "tech-lead")

	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "done", "summary": "Built it.", "votes": [1, 2]}`)
	if task := waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone }); task.Summary != "Built it." {
		t.Errorf("summary %q", task.Summary)
	}
}

// A feature the loop has no checked proposer for writes nothing: what its
// OnDone proposes is refused with the rule "features", and the log keeps no
// task of it.
func TestAFeatureWithNoLogWritesNothing(t *testing.T) {
	s, state, repo := withTeams(t, true)
	loop := newLoop(s, state, t.TempDir())
	loop.Features = append(loop.Features, features.Feature{Name: "votes", Writes: []events.Type{events.TaskCreated},
		Coordinator: &features.Coordinator{For: func(agents.Agent) bool { return true }, Field: "votes",
			OnDone: func(c features.Core, task *tasks.Task, _ json.RawMessage) string {
				_, rejection, err := c.Log.Create("task-", events.Proposal{Actor: agentry, Type: events.TaskCreated,
					Data: map[string]any{"title": "Count", "repo": task.Repo, "coordinator": "tech-lead", "about": task.ID}})
				return fmt.Sprintf("%v %v", rejection, err)
			}}})
	run(t, loop, state)
	const id = "task-1"
	startTask(t, s, id, repo, "tech-lead")

	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "done", "summary": "Built it.", "votes": [1, 2]}`)
	task := waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if !strings.Contains(task.Summary, "features: the votes feature may not write task.created") {
		t.Errorf("the hook should be refused by the features rule:\n%s", task.Summary)
	}
	if ids := state.TaskIDs(); len(ids) != 1 {
		t.Errorf("tasks %v, want only %s", ids, id)
	}
}

// A done from a coordinator the roster no longer has, here renamed while its
// session ran, keeps no ideas and says so.
func TestADoneFromACoordinatorNoLongerThereSaysItsIdeasWerentKept(t *testing.T) {
	s, state, repo := withVisionaries(t)
	const id = "task-1"
	startTask(t, s, id, repo, "editor")
	if _, rejection, err := s.Propose(agents.Agent{Name: "editor"}.Renaming(you, "chief-editor")); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}

	endTurn(t, s, id, "turn-1", "editor", `{"action": "done", "summary": "One idea.", "ideas": [
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M"}]}`)
	task := waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if task.Summary != "One idea.\n\n"+unclaimedIdeas {
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
	startTask(t, s, "task-1", repo, "editor")
	endTurn(t, s, "task-1", "turn-1", "editor", `{"action": "done", "summary": "One idea.", "ideas": [
		{"title": "Dark mode", "problem": "Bright at night", "proposal": "A dark theme", "size": "S"}]}`)
	waitUntil(t, s, "task-1", "done", func(task *tasks.Task) bool { return task.IsDone })
	if _, rejection, err := s.Propose(events.Proposal{Stream: "idea-1", Actor: you, Type: ideas.IdeaDismissed,
		Data: map[string]any{"note": "The OS does it"}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}

	vision := startTask(t, s, "task-2", repo, "editor")
	if brief := readBrief(t, vision, "turn-1"); !strings.Contains(brief, "## Ideas the user dismissed\n\n- Dark mode (idea-1): The OS does it") {
		t.Errorf("the editor's brief should name the dismissed idea:\n%s", brief)
	}
	accepted := mustCreate(t, s, events.Proposal{Actor: agentry, Type: ideas.IdeaSuggested, Data: map[string]any{"title": "Search",
		"problem": "p", "proposal": "f", "size": "S", "task": "task-1", "repo": repo}})
	got, err := ideas.Accept(s, state, accepted, ideas.Choice{})
	if err != nil || !got.Accepted {
		t.Fatalf("accepting: %+v %v", got, err)
	}
	again := startTask(t, s, "task-4", repo, "editor")
	if brief := readBrief(t, again, "turn-1"); !strings.Contains(brief, "## Ideas already accepted\n\n- Search (idea-2): became "+got.Task) ||
		!strings.Contains(brief, `"repeats"`) {
		t.Errorf("the editor's brief should name the accepted idea and how to repeat one:\n%s", brief)
	}
	software := startTask(t, s, "task-5", repo, "tech-lead")
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

// A Visionaries coordinator's brief through the ideas hook is the brief it
// had before the hook: the same sections, in the same place, and the same
// decision text. testdata/visionaries-brief.md was written by the code from
// before the hook, with the repo's path as <repo>.
func TestAVisionariesBriefIsAsItWasBeforeTheHook(t *testing.T) {
	brief, repo := visionariesBrief(t)
	want, err := os.ReadFile(filepath.Join("testdata", "visionaries-brief.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.ReplaceAll(brief, repo, "<repo>"); got != string(want) {
		t.Errorf("the brief changed:\n got %q\nwant %q", got, want)
	}
}

// A feature whose For or Brief panics costs the brief only its own part:
// the brief is the one the other features make.
func TestAPanickingHookLeavesTheBriefToTheOthers(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("testdata", "visionaries-brief.md"))
	if err != nil {
		t.Fatal(err)
	}
	for name, broken := range map[string]*features.Coordinator{
		"For":   {For: func(agents.Agent) bool { panic("a bad hook") }},
		"Brief": {For: func(agents.Agent) bool { return true }, Brief: func(features.Reader, *tasks.Task) []features.Section { panic("a bad hook") }},
	} {
		t.Run(name, func(t *testing.T) {
			brief, repo := visionariesBriefWith(t, features.Feature{Name: "votes", Coordinator: broken}, ideas.Feature())
			if got := strings.ReplaceAll(brief, repo, "<repo>"); got != string(want) {
				t.Errorf("the brief changed:\n got %q\nwant %q", got, want)
			}
		})
	}
}

// visionariesBrief is the brief of editor, the Visionaries coordinator, on
// a task in a repo where one idea was dismissed and one accepted, and an
// open one is left out.
func visionariesBrief(t *testing.T) (string, string) {
	t.Helper()
	return visionariesBriefWith(t, ideas.Feature())
}

// visionariesBriefWith is visionariesBrief, made with list's features.
func visionariesBriefWith(t *testing.T, list ...features.Feature) (string, string) {
	t.Helper()
	s, state, repo := withTeams(t, true)
	mustCreateTask(t, s, events.Proposal{Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Think up features", "repo": repo, "coordinator": "editor", "teamChosen": true}})
	suggested := func(title string) string {
		return mustCreate(t, s, events.Proposal{Actor: agentry, Type: ideas.IdeaSuggested, Data: map[string]any{"title": title,
			"problem": "p", "proposal": "f", "size": "S", "task": "task-1", "repo": repo}})
	}
	dismissed := suggested("Dark mode")
	if _, rejection, err := s.Propose(events.Proposal{Stream: dismissed, Actor: you, Type: ideas.IdeaDismissed,
		Data: map[string]any{"note": "The OS does it"}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if got, err := ideas.Accept(s, state, suggested("Search"), ideas.Choice{}); err != nil || !got.Accepted {
		t.Fatalf("accepting: %+v %v", got, err)
	}
	suggested("Still open")
	held, err := state.Task("task-1")
	if err != nil {
		t.Fatal(err)
	}
	task := *held // the state's own stays as the log made it
	task.Worktree = &tasks.Worktree{Repo: repo, Worker: "local", Path: repo + "/task-1", Branch: "agentry/task-1", Base: "abc123"}
	editor, _ := state.Roster().Get("editor")
	return coordinatorBrief(&task, editor, state.Roster(), family{}, nil, nil, state, list), repo
}
