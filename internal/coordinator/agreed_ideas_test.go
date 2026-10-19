package coordinator

import (
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
// viewpoint agreed on, following up task-epic; one doubted; one a viewpoint
// didn't weigh in on; and two agreed on under the titles of an idea you
// dismissed and one you accepted before. It waits for the done and answers
// the repo's ideas by title.
func agreedDone(t *testing.T, s *store.Store, state *current.State, repo string, change map[string]any) map[string]*ideas.Idea {
	t.Helper()
	if len(change) > 0 {
		mustPropose(t, s, events.Proposal{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged, Data: change})
	}
	mustPropose(t, s, events.Proposal{Stream: "task-epic", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Improve it",
		"description": "Improve it.\n\nHard limits:\n- No deploys.\n- No secrets.\n\nThanks.", "repo": repo, "coordinator": "tech-lead",
		"kind": "epic", "parked": "backlog"}})
	mustPropose(t, s, events.Proposal{Stream: "task-earlier", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Earlier ideas",
		"repo": repo, "coordinator": "editor", "parked": "backlog"}})
	for title, by := range map[string]events.Type{"Dark mode": events.IdeaDismissed, "Export": events.IdeaAccepted} {
		created := mustCreate(t, s, events.Proposal{Actor: agentry, Type: events.IdeaSuggested, Data: map[string]any{"title": title,
			"problem": "p", "proposal": "f", "size": "S", "task": "task-earlier", "repo": repo}})
		if by == events.IdeaDismissed {
			mustPropose(t, s, events.Proposal{Stream: created, Actor: you, Type: by, Data: map[string]any{"note": "No"}})
		} else if got, err := ideas.Accept(s, state, created, ideas.Choice{}); err != nil || !got.Accepted {
			t.Fatalf("accepting %s: %+v %v", title, got, err)
		}
	}

	const id = "task-vision-agreed"
	startTask(t, s, id, repo, "editor")
	endTurn(t, s, id, "turn-1", "editor", `{"action": "done", "summary": "Five ideas.", "ideas": [
		{"title": "Search the board", "problem": "Scrolling", "proposal": "A search box", "size": "M", "epic": "task-epic", "backedBy": `+everyViewpoint+`},
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
	created, rejection, err := s.Create(events.IdeaPrefix, p)
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
	if agreed.State != ideas.Accepted || !agreed.AutoAccepted || agreed.Note != AutoAccepted || agreed.Became == "" {
		t.Fatalf("the agreed idea: %s, auto %v, note %q, became %q", agreed.State, agreed.AutoAccepted, agreed.Note, agreed.Became)
	}
	leftOpen(t, byTitle)
	stream := mustStream(t, s, agreed.Became)
	if stream[0].Actor != agentry || stream[0].Text("parked") != "backlog" {
		t.Errorf("made by %v, parked %q: want Agentry's, in the Backlog first", stream[0].Actor, stream[0].Text("parked"))
	}
	task := waitUntil(t, s, agreed.Became, "started", func(task *tasks.Task) bool { return task.Parked == "" })
	if task.Repo != repo || task.Idea != agreed.ID || task.Coordinator != "tech-lead" ||
		!strings.Contains(task.Description, "Limits from task-epic (copied from the epic):\n\nHard limits:\n- No deploys.\n- No secrets.") {
		t.Errorf("task: repo %q idea %q coordinator %q\n%s", task.Repo, task.Idea, task.Coordinator, task.Description)
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
	// task-epic, task-earlier, the one Export became and the session
	if got := len(state.TaskIDs()); got != before+4 {
		t.Errorf("%d tasks were made, want 4", got-before)
	}
}
