package coordinator

import (
	"cmp"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/settings"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// The test Visionaries are Writing's agents, so the viewpoints are these.
const everyViewpoint = `["researcher", "writer", "fact-checker"]`

// agreedDone is a Visionaries done with five ideas about repo: one every
// viewpoint agreed on, following up the epic task-1; one doubted; one a viewpoint
// didn't weigh in on; and two agreed on under the titles of an idea you
// dismissed and one you accepted before. It waits for the done and answers
// the repo's ideas by title.
func agreedDone(t *testing.T, s *store.Store, state *current.State, repo string, change map[string]any) map[string]*ideas.Idea {
	t.Helper()
	if len(change) > 0 {
		mustPropose(t, s, events.Proposal{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged, Data: change})
	}
	mustPropose(t, s, events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Improve it",
		"description": "Improve it.\n\nHard limits:\n- No deploys.\n- No secrets.\n\nThanks.", "repo": repo, "coordinator": "tech-lead",
		"kind": "epic", "parked": "backlog"}})
	mustPropose(t, s, events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Earlier ideas",
		"repo": repo, "coordinator": "editor", "parked": "backlog"}})
	for title, by := range map[string]events.Type{"Dark mode": ideas.IdeaDismissed, "Export": ideas.IdeaAccepted} {
		created := mustCreate(t, s, events.Proposal{Actor: agentry, Type: ideas.IdeaSuggested, Data: map[string]any{"title": title,
			"problem": "p", "proposal": "f", "size": "S", "task": "task-2", "repo": repo}})
		if by == ideas.IdeaDismissed {
			mustPropose(t, s, events.Proposal{Stream: created, Actor: you, Type: by, Data: map[string]any{"note": "No"}})
		} else if got, err := ideas.Accept(s, state, created, ideas.Choice{}); err != nil || !got.Accepted {
			t.Fatalf("accepting %s: %+v %v", title, got, err)
		}
	}

	id := state.NextTask()
	startTask(t, s, id, repo, "editor")
	endTurn(t, s, id, "turn-1", "editor", `{"action": "done", "summary": "Five ideas.", "ideas": [
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M", "epic": "task-1", "backedBy": `+everyViewpoint+`},
		{"title": "Doubted", "problem": "p", "proposal": "f", "size": "S", "backedBy": `+everyViewpoint+`, "doubtedBy": ["writer"]},
		{"title": "Half", "problem": "p", "proposal": "f", "size": "S", "backedBy": ["researcher", "writer"]},
		{"title": "dark mode", "problem": "p", "proposal": "f", "size": "S", "backedBy": `+everyViewpoint+`},
		{"title": "Export", "problem": "p", "proposal": "f", "size": "S", "backedBy": `+everyViewpoint+`}]}`)
	waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	list := ideas.List(state, repo)
	byTitle := map[string]*ideas.Idea{}
	for _, idea := range list {
		if idea.Task == id {
			byTitle[idea.Title] = idea
		}
	}
	if len(byTitle) != 5 {
		t.Fatalf("want the five ideas kept: %v", byTitle)
	}
	return byTitle
}

func mustCreate(t *testing.T, s *store.Store, p events.Proposal) string {
	t.Helper()
	created, rejection, err := s.Create(ideas.Prefix, p)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	return created.Stream
}

// leftOpen checks the ideas but the agreed one stay open for you.
func leftOpen(t *testing.T, byTitle map[string]*ideas.Idea) {
	t.Helper()
	for _, title := range []string{"Doubted", "Half", "dark mode", "Export"} {
		if idea := byTitle[title]; idea.State != ideas.Open {
			t.Errorf("%q should be left open for you: %s", title, idea.State)
		}
	}
}

func TestAnIdeaEveryVisionaryAgreedOnIsAcceptedAndStarted(t *testing.T) {
	s, state, repo := withTeams(t, true)
	runLoop(t, s, state)
	byTitle := agreedDone(t, s, state, repo, map[string]any{settings.AutoAcceptIdeas: true, settings.AutoStartIdeas: true})

	agreed := byTitle["Search the board"]
	if agreed.State != ideas.Accepted || !agreed.AutoAccepted || agreed.Note != "auto-accepted: all Visionaries agreed" || agreed.Became == "" {
		t.Fatalf("the agreed idea: %s, auto %v, note %q, became %q", agreed.State, agreed.AutoAccepted, agreed.Note, agreed.Became)
	}
	leftOpen(t, byTitle)
	stream := mustStream(t, s, agreed.Became)
	if stream[0].Actor != agentry || stream[0].Text("parked") != "backlog" {
		t.Errorf("made by %v, parked %q: want Agentry's, in the Backlog first", stream[0].Actor, stream[0].Text("parked"))
	}
	task := waitUntil(t, s, agreed.Became, "started", func(task *tasks.Task) bool { return task.Parked == "" })
	if task.Repo != repo || task.MadeFrom != agreed.ID || task.Coordinator != "tech-lead" ||
		!strings.Contains(task.Description, "Limits from task-1 (copied from the epic):\n\nHard limits:\n- No deploys.\n- No secrets.") {
		t.Errorf("task: repo %q made from %q coordinator %q\n%s", task.Repo, task.MadeFrom, task.Coordinator, task.Description)
	}
	started := false
	for _, e := range mustStream(t, s, agreed.Became) {
		started = started || (e.Type == events.TaskHeld && e.Actor == agentry && e.Text("column") == "in-progress")
	}
	if !started {
		t.Error("Agentry should start it as your Start does: task.held to In progress")
	}
}

func TestAnAgreedIdeaWaitsInTheBacklogUnlessStartingIsOn(t *testing.T) {
	s, state, repo := withTeams(t, true)
	runLoop(t, s, state)
	byTitle := agreedDone(t, s, state, repo, map[string]any{settings.AutoAcceptIdeas: true})

	agreed := byTitle["Search the board"]
	if agreed.State != ideas.Accepted || !agreed.AutoAccepted {
		t.Fatalf("the agreed idea: %s, auto %v", agreed.State, agreed.AutoAccepted)
	}
	leftOpen(t, byTitle)
	if task := load(t, s, agreed.Became); task.Parked != "backlog" {
		t.Errorf("starting is off: the task should wait in the Backlog, parked %q", task.Parked)
	}
}

func TestStartingAloneAcceptsNothing(t *testing.T) {
	s, state, repo := withTeams(t, true)
	runLoop(t, s, state)
	byTitle := agreedDone(t, s, state, repo, map[string]any{settings.AutoStartIdeas: true})
	for title, idea := range byTitle {
		if idea.State != ideas.Open {
			t.Errorf("%q: accepting is off, so it stays open: %s", title, idea.State)
		}
	}
}

// The default: nothing is accepted for you, and no task is made.
func TestWithTheSettingOffEveryIdeaStaysOpen(t *testing.T) {
	s, state, repo := withTeams(t, true)
	runLoop(t, s, state)
	before := len(state.TaskIDs())
	byTitle := agreedDone(t, s, state, repo, nil)
	for title, idea := range byTitle {
		if idea.State != ideas.Open {
			t.Errorf("%q: %s", title, idea.State)
		}
	}
	// the epic, the earlier task, the one Export became and the session
	if got := len(state.TaskIDs()); got != before+4 {
		t.Errorf("%d tasks were made, want 4", got-before)
	}
}

// Agentry's accept gives the task the server's limits, unlimited ones too,
// or, for an idea that follows up an epic, the epic's.
func TestAnAgreedIdeaTakesTheServersLimitsOrItsEpics(t *testing.T) {
	for _, defaults := range []tasks.Limits{{}, {Budget: 5_000_000, MaxTurns: 24}} {
		t.Run(fmt.Sprintf("%+v", defaults), func(t *testing.T) {
			s, state, repo := withTeams(t, true)
			loop := newLoop(s, state, t.TempDir())
			loop.Defaults = defaults
			run(t, loop, state)
			mustPropose(t, s, events.Proposal{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged,
				Data: map[string]any{settings.AutoAcceptIdeas: true}})
			mustPropose(t, s, events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Improve it",
				"repo": repo, "coordinator": "tech-lead", "kind": "epic", "parked": "backlog", "budget": 800, "maxTurns": 6}})

			const id = "task-2"
			startTask(t, s, id, repo, "editor")
			endTurn(t, s, id, "turn-1", "editor", `{"action": "done", "summary": "Two ideas.", "ideas": [
				{"title": "Alone", "problem": "p", "proposal": "f", "size": "S", "backedBy": `+everyViewpoint+`},
				{"title": "Follow", "problem": "p", "proposal": "f", "size": "S", "epic": "task-1", "backedBy": `+everyViewpoint+`}]}`)
			waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
			list := ideas.List(state, repo)
			if len(list) != 2 {
				t.Fatalf("the ideas: %v", list)
			}
			want := map[string]tasks.Limits{"Alone": defaults, "Follow": {Budget: 800, MaxTurns: 6}}
			for _, idea := range list {
				if idea.Became == "" {
					t.Fatalf("%q wasn't accepted: %s", idea.Title, idea.State)
				}
				if limits := load(t, s, idea.Became).Limits(); limits != want[idea.Title] {
					t.Errorf("%q: limits %+v, want %+v", idea.Title, limits, want[idea.Title])
				}
			}
		})
	}
}

// Under nobodyDoubted an idea two viewpoints backed with no doubt is
// accepted, with a note that says so, while a doubted one, one with a
// single backer and every duplicate stay open: one under the title of an
// idea you chose, one that says what an accepted idea said under a new
// title, and one the Visionaries said repeats an idea.
func TestUnderNobodyDoubtedADuplicateIsNeverAccepted(t *testing.T) {
	s, state, repo := withTeams(t, true)
	runLoop(t, s, state)
	byTitle := agreedDone(t, s, state, repo, map[string]any{settings.AutoAcceptIdeas: true, settings.AutoAcceptRule: settings.NobodyDoubted})
	for title, note := range map[string]string{"Search the board": "auto-accepted: nobody doubted it, 3 backed it",
		"Half": "auto-accepted: nobody doubted it, 2 backed it"} {
		if idea := byTitle[title]; idea.State != ideas.Accepted || !idea.AutoAccepted || idea.Note != note {
			t.Errorf("%q: %s, auto %v, note %q; want accepted with %q", title, idea.State, idea.AutoAccepted, idea.Note, note)
		}
	}
	for _, title := range []string{"Doubted", "dark mode", "Export"} {
		if idea := byTitle[title]; idea.State != ideas.Open {
			t.Errorf("%q should be left open for you: %s", title, idea.State)
		}
	}

	const problem = "Finding the right task on a busy board takes a lot of scrolling through every column"
	const proposal = "Add a search box above the board that filters the cards by title as you type"
	accepted := mustCreate(t, s, events.Proposal{Actor: agentry, Type: ideas.IdeaSuggested, Data: map[string]any{"title": "Board search",
		"problem": problem, "proposal": proposal, "size": "M", "task": "task-2", "repo": repo}})
	if got, err := ideas.Accept(s, state, accepted, ideas.Choice{}); err != nil || !got.Accepted {
		t.Fatalf("accepting the original: %+v %v", got, err)
	}
	id := state.NextTask()
	startTask(t, s, id, repo, "editor")
	endTurn(t, s, id, "turn-1", "editor", `{"action": "done", "summary": "Three ideas.", "ideas": [
		{"title": "Find cards fast", "problem": "`+problem+`", "proposal": "A search box above the board that filters cards by their title", "size": "M",
			"backedBy": `+everyViewpoint+`},
		{"title": "Something new", "problem": "p", "proposal": "f", "size": "S", "backedBy": `+everyViewpoint+`, "repeats": "`+accepted+`"},
		{"title": "Alone", "problem": "p", "proposal": "f", "size": "S", "backedBy": ["writer"]}]}`)
	waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	kept := 0
	for _, idea := range ideas.List(state, repo) {
		if idea.Task == id {
			kept++
			if idea.State != ideas.Open {
				t.Errorf("%q should be left open for you: %s", idea.Title, idea.State)
			}
		}
	}
	if kept != 3 {
		t.Errorf("want the three ideas kept, got %d", kept)
	}
	// The loop leaves a duplicate alone, not tries it for the rules to refuse.
	all, err := s.After(0, 10000)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range all {
		if e.Type == events.ProposalRejected {
			t.Errorf("Agentry tried something the rules refused, in %s: %s", e.Stream, e.Text("reason"))
		}
	}
}

// A Visionaries done with ideas, with accepting and starting them on,
// writes the events it wrote before the ideas hook: the same types, actors
// and data. testdata/visionaries-done-events.jsonl was written by the code
// from before the hook, with the repo's path as <repo>.
func TestAVisionariesDoneWritesWhatItDidBeforeTheHook(t *testing.T) {
	got, repo := visionariesDoneEvents(t)
	want, err := os.ReadFile(filepath.Join("testdata", "visionaries-done-events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if got = strings.ReplaceAll(got, repo, "<repo>"); got != string(want) {
		t.Errorf("the events changed:\n got %s\nwant %s", got, want)
	}
}

// visionariesDoneEvents are agreedDone's events, as JSON lines: the done,
// every event of its five ideas, and Agentry making and starting the agreed
// idea's task.
func visionariesDoneEvents(t *testing.T) (string, string) {
	t.Helper()
	s, state, repo := withTeams(t, true)
	runLoop(t, s, state)
	byTitle := agreedDone(t, s, state, repo, map[string]any{settings.AutoAcceptIdeas: true, settings.AutoStartIdeas: true})
	agreed := byTitle["Search the board"]
	waitUntil(t, s, agreed.Became, "started", func(task *tasks.Task) bool { return task.Parked == "" })
	var b strings.Builder
	add := func(e events.Event) {
		line, err := json.Marshal(e.Proposal)
		if err != nil {
			t.Fatal(err)
		}
		b.Write(append(line, '\n'))
	}
	for _, e := range mustStream(t, s, agreed.Task) {
		if e.Type == events.TaskDone {
			add(e)
		}
	}
	var kept []*ideas.Idea
	for _, idea := range byTitle {
		kept = append(kept, idea)
	}
	slices.SortFunc(kept, func(a, b *ideas.Idea) int {
		return cmp.Compare(mustStream(t, s, a.ID)[0].Position, mustStream(t, s, b.ID)[0].Position)
	})
	for _, idea := range kept {
		for _, e := range mustStream(t, s, idea.ID) {
			add(e)
		}
	}
	for _, e := range mustStream(t, s, agreed.Became) {
		if e.Actor == agentry && (e.Type == events.TaskCreated || e.Type == events.TaskHeld) {
			add(e)
		}
	}
	return b.String(), repo
}
