package coordinator

import (
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// An epic on the real loop: the tech lead splits it into three stories, one
// for Writing and one that builds on the first; each story's summary comes
// back, and the epic is done only once they all are.
func TestAnEpicRunsItsStoriesAndFinishesWithThem(t *testing.T) {
	s, cancel, repo := twoTeams(t)
	defer cancel()
	const id = "task-1"
	defer sessions.Stop(sessions.SocketFor(id))
	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": "Ship the TODO tool", "repo": repo, "coordinator": "tech-lead", "teamChosen": true, "kind": "epic", "kindChosen": true}})

	epic := waitForTurn(t, s, id, "turn-1", "tech-lead")
	if brief := readBrief(t, epic, "turn-1"); !strings.Contains(brief, "it has none yet. Before anything else, split it") {
		t.Errorf("the epic's brief should ask for stories:\n%s", brief)
	}
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "split", "stories": [
		{"title": "Add the TODO store", "brief": "A store for TODOs, with tests."},
		{"title": "Write the user guide", "brief": "One page on using the tool.", "team": "writing"},
		{"title": "Add the TODO command", "brief": "A command on top of the store.", "after": [1]}]}`)
	epic = waitUntil(t, s, id, "the split", func(task *tasks.Task) bool { return len(task.Stories) == 3 })
	store1, guide, command := epic.Stories[0], epic.Stories[1], epic.Stories[2]
	for _, story := range epic.Stories {
		defer sessions.Stop(sessions.SocketFor(story))
	}
	// They wait in your Backlog; you start them.
	for _, story := range epic.Stories {
		if task := load(t, s, story); task.Parked != "backlog" || len(task.Turns) > 0 {
			t.Errorf("%s should wait in the Backlog: parked %q, %d turns", story, task.Parked, len(task.Turns))
		}
		start(t, s, story)
	}
	waitForTurn(t, s, store1, "turn-1", "tech-lead")
	child := waitForTurn(t, s, guide, "turn-1", "editor")
	if child.Epic != id || !strings.Contains(child.Description, "A story of "+id) {
		t.Errorf("the guide's story: epic %q, description %q", child.Epic, child.Description)
	}
	if brief := readBrief(t, child, "turn-1"); !strings.Contains(brief, "This task is a story of an epic") {
		t.Errorf("the story's brief should say whose story it is:\n%s", brief)
	}
	if task := load(t, s, command); task.Worktree != nil || len(task.Turns) > 0 {
		t.Errorf("the command builds on the store, so it waits: %+v", task.Turns)
	}

	// The store is done: its summary comes back, and the command starts.
	endTurn(t, s, store1, "turn-1", "tech-lead", `{"action": "done", "summary": "The store is in."}`)
	epic = waitForTurn(t, s, id, "turn-2", "tech-lead")
	if brief := readBrief(t, epic, "turn-2"); !strings.Contains(brief, store1+" is done: The store is in.") {
		t.Errorf("the epic should hear the store is done:\n%s", brief)
	}
	waitForTurn(t, s, command, "turn-1", "tech-lead")

	// While the epic's coordinator decides, the guide finishes: it hears that next.
	endTurn(t, s, guide, "turn-1", "editor", `{"action": "done", "summary": "The guide is in docs/guide.md."}`)
	waitUntil(t, s, id, "the guide's news", func(task *tasks.Task) bool { return len(task.StoriesDone) == 2 })
	endTurn(t, s, id, "turn-2", "tech-lead", `{"action": "wait"}`)
	epic = waitForTurn(t, s, id, "turn-3", "tech-lead")
	if brief := readBrief(t, epic, "turn-3"); !strings.Contains(brief, guide+" is done: The guide is in docs/guide.md.") {
		t.Errorf("the epic should hear the guide is done:\n%s", brief)
	}

	// Not done while a story isn't.
	endTurn(t, s, id, "turn-3", "tech-lead", `{"action": "done", "summary": "Shipped."}`)
	epic = waitForTurn(t, s, id, "turn-4", "tech-lead")
	if brief := readBrief(t, epic, "turn-4"); !strings.Contains(brief, "the epic can't be done while its stories aren't: "+command+" still going") {
		t.Errorf("the epic should hear why it isn't done:\n%s", brief)
	}
	endTurn(t, s, id, "turn-4", "tech-lead", `{"action": "wait"}`)
	waitUntil(t, s, id, "waiting", func(task *tasks.Task) bool {
		return task.Handoff == nil && task.OpenTurn() == nil && task.PendingDecision == ""
	})

	endTurn(t, s, command, "turn-1", "tech-lead", `{"action": "done", "summary": "The command is in."}`)
	waitForTurn(t, s, id, "turn-5", "tech-lead")
	endTurn(t, s, id, "turn-5", "tech-lead", `{"action": "done", "summary": "Shipped: the store, the guide and the command."}`)

	// You close an epic: it asks first.
	epic = waitUntil(t, s, id, "asking to close", func(task *tasks.Task) bool { return task.Closing != "" })
	if epic.IsDone || !strings.Contains(epic.Question, "Close it?") || epic.Asked.Options[0] != CloseEpic {
		t.Errorf("the epic should ask you to close it: done %v, question %q", epic.IsDone, epic.Question)
	}
	if _, rejection, _ := s.Propose(events.Proposal{Stream: id, Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"}, Type: events.TaskDone,
		Data: map[string]any{"summary": "x"}}); rejection == nil {
		t.Error("the coordinator closed the epic itself")
	}
	if _, rejection, err := s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskDone,
		Data: map[string]any{"summary": epic.Closing}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	epic = waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if epic.Summary != "Shipped: the store, the guide and the command." || epic.Question != "" {
		t.Errorf("closed: summary %q, question %q", epic.Summary, epic.Question)
	}
}

func TestAStoryCantBeSplitAndAnEpicIsntAssigned(t *testing.T) {
	s, cancel, repo := twoTeams(t)
	defer cancel()
	const id = "task-1"
	defer sessions.Stop(sessions.SocketFor(id))
	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": "Fix the typo", "repo": repo, "coordinator": "tech-lead", "teamChosen": true, "kind": "", "kindChosen": true}})
	task := waitForTurn(t, s, id, "turn-1", "tech-lead")
	if brief := readBrief(t, task, "turn-1"); strings.Contains(brief, "If this task is too big") {
		t.Errorf("a story you chose isn't offered a split:\n%s", brief)
	}
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "split", "stories": [{"title": "a", "brief": "b"}]}`)
	task = waitForTurn(t, s, id, "turn-2", "tech-lead")
	if brief := readBrief(t, task, "turn-2"); !strings.Contains(brief, "the user made this task a story, so it isn't split") {
		t.Errorf("the tech lead should hear why:\n%s", brief)
	}

	// Left to the coordinator, a task can become an epic; then it isn't assigned.
	const other = "task-2"
	defer sessions.Stop(sessions.SocketFor(other))
	s.Propose(events.Proposal{Stream: other, Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": "Build the app", "repo": repo, "coordinator": "tech-lead", "teamChosen": true}})
	task = waitForTurn(t, s, other, "turn-1", "tech-lead")
	if brief := readBrief(t, task, "turn-1"); !strings.Contains(brief, "If this task is too big") {
		t.Errorf("a task left to the coordinator may be split:\n%s", brief)
	}
	endTurn(t, s, other, "turn-1", "tech-lead", `{"action": "split", "stories": [{"title": "The backend", "brief": "An API."}]}`)
	task = waitUntil(t, s, other, "the split", func(task *tasks.Task) bool { return task.Kind == tasks.Epic && len(task.Stories) == 1 })
	defer sessions.Stop(sessions.SocketFor(task.Stories[0]))
	start(t, s, task.Stories[0])
	story := waitForTurn(t, s, task.Stories[0], "turn-1", "tech-lead")
	endTurn(t, s, story.ID, "turn-1", "tech-lead", `{"action": "split", "stories": [{"title": "a", "brief": "b"}]}`)
	story = waitForTurn(t, s, story.ID, "turn-2", "tech-lead")
	if brief := readBrief(t, story, "turn-2"); !strings.Contains(brief, "a story can't be split") {
		t.Errorf("a story of an epic isn't split:\n%s", brief)
	}
}

// start takes the story out of the Backlog, as moving it on the board does.
func start(t *testing.T, s *store.Store, id string) {
	t.Helper()
	if _, rejection, err := s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskHeld, Data: map[string]any{"place": ""}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
}

func load(t *testing.T, s *store.Store, id string) *tasks.Task {
	t.Helper()
	return tasks.FromEvents(mustStream(t, s, id))
}
