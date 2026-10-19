package coordinator

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// An epic on the real loop: the tech lead splits it into three stories, one
// for Writing and one that builds on the first; each story's summary comes
// back, and the epic is done only once they all are.
func TestAnEpicRunsItsStoriesAndFinishesWithThem(t *testing.T) {
	s, repo := twoTeams(t)
	const id = "task-1"
	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": "Ship the TODO tool", "repo": repo, "coordinator": "tech-lead", "teamChosen": true, "kind": "epic", "kindChosen": true,
		"budget": 900_000, "maxTurns": 9}})

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
	// They wait in your Backlog; you start them.
	for _, story := range epic.Stories {
		if task := load(t, s, story); task.Parked != "backlog" || len(task.Turns) > 0 {
			t.Errorf("%s should wait in the Backlog: parked %q, %d turns", story, task.Parked, len(task.Turns))
		}
		if limits := load(t, s, story).Limits(); limits != epic.Limits() {
			t.Errorf("%s takes its epic's limits %+v: %+v", story, epic.Limits(), limits)
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
	s, repo := twoTeams(t)
	const id = "task-1"
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
	s.Propose(events.Proposal{Stream: other, Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": "Build the app", "repo": repo, "coordinator": "tech-lead", "teamChosen": true}})
	task = waitForTurn(t, s, other, "turn-1", "tech-lead")
	if brief := readBrief(t, task, "turn-1"); !strings.Contains(brief, "If this task is too big") {
		t.Errorf("a task left to the coordinator may be split:\n%s", brief)
	}
	endTurn(t, s, other, "turn-1", "tech-lead", `{"action": "split", "stories": [{"title": "The backend", "brief": "An API."}]}`)
	task = waitUntil(t, s, other, "the split", func(task *tasks.Task) bool { return task.Kind == tasks.Epic && len(task.Stories) == 1 })
	start(t, s, task.Stories[0])
	story := waitForTurn(t, s, task.Stories[0], "turn-1", "tech-lead")
	endTurn(t, s, story.ID, "turn-1", "tech-lead", `{"action": "split", "stories": [{"title": "a", "brief": "b"}]}`)
	story = waitForTurn(t, s, story.ID, "turn-2", "tech-lead")
	if brief := readBrief(t, story, "turn-2"); !strings.Contains(brief, "a story can't be split") {
		t.Errorf("a story of an epic isn't split:\n%s", brief)
	}
}

// An epic whose stories are all done asks you once whether to close it. If
// you leave it open, it waits quietly, with no more questions, until you add
// a story, which wakes it, or close it.
func TestAnEpicStaysOpenWithEveryStoryDone(t *testing.T) {
	s, repo := twoTeams(t)
	const id = "task-1"
	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": "Ship the TODO tool", "repo": repo, "coordinator": "tech-lead", "teamChosen": true, "kind": "epic", "kindChosen": true}})
	waitForTurn(t, s, id, "turn-1", "tech-lead")
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "split", "stories": [{"title": "Add the TODO store", "brief": "A store."}]}`)
	first := waitUntil(t, s, id, "the split", func(task *tasks.Task) bool { return len(task.Stories) == 1 }).Stories[0]
	start(t, s, first)
	waitForTurn(t, s, first, "turn-1", "tech-lead")
	endTurn(t, s, first, "turn-1", "tech-lead", `{"action": "done", "summary": "The store is in."}`)
	waitForTurn(t, s, id, "turn-2", "tech-lead")
	endTurn(t, s, id, "turn-2", "tech-lead", `{"action": "done", "summary": "Shipped the store."}`)

	// Asked once; you leave it open.
	waitUntil(t, s, id, "asking to close", func(task *tasks.Task) bool { return task.Question != "" && task.Closing != "" })
	if _, rejection, err := s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskAnswered,
		Data: map[string]any{"answer": "Leave it open, there may be more."}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	epic := waitForTurn(t, s, id, "turn-3", "tech-lead")
	if brief := readBrief(t, epic, "turn-3"); !strings.Contains(brief, "If the user says to leave it open, \"wait\" and don't ask again") {
		t.Errorf("the brief should say to wait once the user leaves it open:\n%s", brief)
	}
	endTurn(t, s, id, "turn-3", "tech-lead", `{"action": "wait"}`)
	idle := func(task *tasks.Task) bool {
		return task.Handoff == nil && task.OpenTurn() == nil && task.PendingDecision == ""
	}
	epic = waitUntil(t, s, id, "waiting", idle)
	if epic.IsDone || epic.Question != "" || epic.Status() != tasks.Working || epic.Closing != "Shipped the store." || epic.IssuesInARow != 0 {
		t.Errorf("the epic should wait, open and quiet: done %v, question %q, status %s, closing %q, issues %d",
			epic.IsDone, epic.Question, epic.Status(), epic.Closing, epic.IssuesInARow)
	}

	// Nothing more happens: the loop looks at it again, and leaves it.
	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskLabeled, Data: map[string]any{"labels": []string{"later"}}})
	time.Sleep(300 * time.Millisecond)
	if epic = load(t, s, id); len(epic.Turns) != 3 || epic.Question != "" || asked(t, s, id) != 1 {
		t.Errorf("the epic should stay quiet: %d turns, question %q, asked %d times", len(epic.Turns), epic.Question, asked(t, s, id))
	}

	// A new story wakes it once it is done, and it runs as before.
	const second = "task-3"
	s.Propose(events.Proposal{Stream: second, Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": "Add the TODO command", "repo": repo, "coordinator": "tech-lead", "teamChosen": true, "kindChosen": true}})
	for _, p := range []events.Proposal{
		{Stream: second, Actor: you, Type: events.EpicJoined, Data: map[string]any{"epic": id}},
		{Stream: id, Actor: you, Type: events.StoryAdded, Data: map[string]any{"story": second, "title": "Add the TODO command"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	if epic = load(t, s, id); epic.Closing != "" {
		t.Errorf("an epic with a story going isn't ready to close: %q", epic.Closing)
	}
	waitForTurn(t, s, second, "turn-1", "tech-lead")
	endTurn(t, s, second, "turn-1", "tech-lead", `{"action": "done", "summary": "The command is in."}`)
	epic = waitForTurn(t, s, id, "turn-4", "tech-lead")
	if brief := readBrief(t, epic, "turn-4"); !strings.Contains(brief, second+" is done: The command is in.") {
		t.Errorf("the epic should hear the new story is done:\n%s", brief)
	}
	endTurn(t, s, id, "turn-4", "tech-lead", `{"action": "done", "summary": "Shipped the store and the command."}`)
	waitUntil(t, s, id, "asking again", func(task *tasks.Task) bool { return task.Closing == "Shipped the store and the command." })
	if n := asked(t, s, id); n != 2 {
		t.Errorf("asked %d times; once per time its stories were all done is 2", n)
	}
}

// asked is how many times the task asked you something.
func asked(t *testing.T, s *store.Store, id string) int {
	n := 0
	for _, e := range mustStream(t, s, id) {
		if e.Type == events.TaskNeedsYou {
			n++
		}
	}
	return n
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

// A story can build on one of the epic's stories already there, by its id,
// and on an earlier story of the same split, by its number: each made
// story's after names the right task. An id has no number, which once
// indexed the split's stories at -1 and stopped the server.
func TestASplitStoryBuildsOnAnExistingStoryOrAnEarlierOne(t *testing.T) {
	s, state, epic := splitting(t, `{"action": "split", "stories": [
		{"title": "Add the command", "brief": "On the store.", "after": ["task-1"]},
		{"title": "Document it", "brief": "One page.", "after": [1, "task-1"]}]}`)
	task, roster, _ := (&Loop{store: s, state: state}).load(epic)
	move := Next(task, roster)
	if move.Kind != ApplyDecision {
		t.Fatalf("the split should be applied: %s %s", move.Kind, move.Reason)
	}
	(&Loop{store: s, state: state}).apply(task, roster, move.Decision)

	if stories := load(t, s, epic).Stories; !slices.Equal(stories, []string{"task-1", "task-3", "task-4"}) {
		t.Fatalf("stories %v", stories)
	}
	if after := load(t, s, "task-3").After; !slices.Equal(after, []string{"task-1"}) {
		t.Errorf("the command builds on task-1: after %v", after)
	}
	if after := load(t, s, "task-4").After; !slices.Equal(after, []string{"task-3", "task-1"}) {
		t.Errorf("the docs build on the command and task-1: after %v", after)
	}
}

// A number that isn't an earlier story of the split (none, one not made
// yet, the story itself) makes no story: the coordinator hears why, and
// decides again.
func TestASplitStoryAfterAStoryNotBeforeItIsRefused(t *testing.T) {
	for _, after := range []string{`[0]`, `["0"]`, `[-1]`, `[2]`, `[3]`} {
		s, state, epic := splitting(t, `{"action": "split", "stories": [
			{"title": "Add the store", "brief": "A store."},
			{"title": "Add the command", "brief": "On the store.", "after": `+after+`}]}`)
		newLoop(s, state, t.TempDir()).act(epic)

		task := load(t, s, epic)
		if task.IssuesInARow != 1 || task.PendingDecision != "" || len(task.Stories) != 1 {
			t.Errorf("after %s: %d issues, pending %q, stories %v", after, task.IssuesInARow, task.PendingDecision, task.Stories)
		}
		if reason := lastIssue(t, s, epic); !strings.Contains(reason, "story 2 builds on story") {
			t.Errorf("after %s: the issue should say which story is wrong: %q", after, reason)
		}
		if ids := state.TaskIDs(); len(ids) != 2 {
			t.Errorf("after %s: no story should be made: tasks %v", after, ids)
		}
	}
}

// A decision that panics while the loop acts on it raises an issue on its
// task instead of stopping the server, and the loop goes on to the next
// task. Here the store's check breaks on the story it is asked to make.
func TestADecisionThatPanicsRaisesAnIssueAndTheLoopGoesOn(t *testing.T) {
	broken := func(p events.Proposal, state rules.State, catalogue *events.Catalogue) *events.Rejection {
		if p.Type == events.TaskCreated && p.Text("title") == "Break the check" {
			panic("the check broke")
		}
		return rules.Check(p, state, catalogue)
	}
	s, state, epic := splittingWith(t, broken, `{"action": "split", "stories": [{"title": "Break the check", "brief": "Boom."}]}`)
	loop := newLoop(s, state, t.TempDir())
	loop.act(epic)
	loop.act("task-1")

	if reason := lastIssue(t, s, epic); !strings.Contains(reason, `"split" decision: the check broke`) {
		t.Errorf("the issue should name the decision and the panic: %q", reason)
	}
	if task := load(t, s, epic); task.IssuesInARow != 1 || len(task.Stories) != 1 {
		t.Errorf("%d issues, stories %v", task.IssuesInARow, task.Stories)
	}
	if question := load(t, s, "task-1").Question; !strings.Contains(question, "worktree") {
		t.Errorf("the loop should go on to task-1, whose repo isn't there: question %q", question)
	}
}

// splitting is a store with task-2, an epic with one story, task-1, whose
// coordinator's turn ended with report.
func splitting(t *testing.T, report string) (*store.Store, *current.State, string) {
	return splittingWith(t, rules.Check, report)
}

// splittingWith is splitting with the rules the store checks proposals by.
func splittingWith(t *testing.T, check func(events.Proposal, rules.State, *events.Catalogue) *events.Rejection, report string) (*store.Store, *current.State, string) {
	t.Helper()
	catalogue := events.Core()
	state := current.New(catalogue)
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, err := store.Open(path, catalogue, state, func(p events.Proposal) *events.Rejection { return check(p, state, catalogue) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	t.Cleanup(func() { // cleanups run last first: before the close
		storetest.CheckEqual(t, s, state)
		storetest.CheckSnapshots(t, path, catalogue, s.LastPosition())
	})
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	const epic = "task-2"
	propose(t, s, events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Add the store", "repo": "/r", "coordinator": "tech-lead"}})
	propose(t, s, events.Proposal{Stream: epic, Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Ship the tool", "repo": "/r", "coordinator": "tech-lead",
		"kind": "epic", "kindChosen": true}})
	propose(t, s, events.Proposal{Stream: "task-1", Actor: you, Type: events.EpicJoined, Data: map[string]any{"epic": epic}})
	propose(t, s, events.Proposal{Stream: epic, Actor: you, Type: events.StoryAdded, Data: map[string]any{"story": "task-1", "title": "Add the store"}})
	propose(t, s, events.Proposal{Stream: epic, Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}})
	propose(t, s, events.Proposal{Stream: epic, Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}})
	propose(t, s, events.Proposal{Stream: epic, Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "report": report}})
	return s, state, epic
}

// lastIssue is the reason of the last issue raised on the task.
func lastIssue(t *testing.T, s *store.Store, id string) string {
	reason := ""
	for _, e := range mustStream(t, s, id) {
		if e.Type == events.IssueRaised {
			reason = e.Text("reason")
		}
	}
	return reason
}
