package rules

import (
	"errors"
	"path/filepath"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

var (
	you         = events.Actor{Kind: events.Human, ID: "you"}
	worker      = events.Actor{Kind: events.Worker, ID: "local"}
	policy      = events.Actor{Kind: events.Policy, ID: "rules"}
	agentry     = events.Actor{Kind: events.System, ID: "watchdog"}
	coordinator = events.Actor{Kind: events.Agent, ID: "tech-lead"}
	engineer    = events.Actor{Kind: events.Agent, ID: "engineer"}
)

// step is one proposal and whether it should be accepted.
type step struct {
	actor       events.Actor
	kind        events.Type
	correlation string
	data        map[string]any
	accepted    bool
}

// openSeeded is a store with the rules and the five default agents.
func openSeeded(t *testing.T) *store.Store {
	t.Helper()
	state := current.New()
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), state, func(p events.Proposal) *events.Rejection { return Check(p, state) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, p := range agents.SeedProposals(nil) {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatalf("seeding %s: %v %v", p.Text("name"), rejection, err)
		}
	}
	return s
}

func run(t *testing.T, steps []step) *tasks.Task {
	t.Helper()
	s := openSeeded(t)
	for i, st := range steps {
		p := events.Proposal{Stream: "task-1", Actor: st.actor, Type: st.kind, Correlation: st.correlation, Data: st.data}
		_, rejection, err := s.Propose(p)
		if err != nil {
			t.Fatal(err)
		}
		if (rejection == nil) != st.accepted {
			t.Errorf("step %d %s by %s: accepted=%v (%v), want %v", i+1, st.kind, st.actor, rejection == nil, rejection, st.accepted)
		}
	}
	stream, _ := s.Stream("task-1")
	return tasks.FromEvents(stream)
}

var (
	created  = step{you, events.TaskCreated, "", map[string]any{"title": "Remove the TODOs", "repo": "/src/app", "coordinator": "tech-lead"}, true}
	worktree = step{worker, events.WorktreeReady, "", map[string]any{"path": "/w/task-1", "branch": "agentry/task-1", "port": 7901}, true}
)

func turnStarted(id, agent string) step {
	return step{worker, events.TurnStarted, id, map[string]any{"agent": agent, "hash": "h-" + id}, true}
}

func turnEnded(id, agent, report string) step {
	return step{worker, events.TurnEnded, id, map[string]any{"agent": agent, "report": report}, true}
}

func TestATaskGoesRoundTheCoordinator(t *testing.T) {
	task := run(t, []step{
		created,
		{you, events.TaskCreated, "", created.data, false},                                               // only once
		{worker, events.TurnStarted, "turn-1", map[string]any{"agent": "tech-lead", "hash": "h"}, false}, // no worktree yet
		worktree,
		turnStarted("turn-1", "tech-lead"),
		{worker, events.PromptAcknowledged, "turn-1", map[string]any{"hash": "wrong"}, false},
		{worker, events.PromptAcknowledged, "turn-1", map[string]any{"hash": "h-turn-1"}, true},
		{coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "remove them"}, false}, // still in its turn
		turnEnded("turn-1", "tech-lead", `{"action":"assign"}`),
		{engineer, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "x"}, false},     // only the coordinator assigns
		{coordinator, events.TaskAssigned, "", map[string]any{"agent": "tech-lead", "instructions": "x"}, false}, // not to itself
		{coordinator, events.TaskAssigned, "", map[string]any{"agent": "designer", "instructions": "x"}, false},  // no such agent
		{coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "remove them"}, true},
		{worker, events.TurnStarted, "turn-5", map[string]any{"agent": "engineer", "hash": "h"}, false}, // wrong turn number
		turnStarted("turn-2", "engineer"),
		{coordinator, events.ActionRequested, "toolu_1", map[string]any{"tool": "Bash", "command": "ls"}, false}, // not the working agent
		{engineer, events.ActionRequested, "toolu_1", map[string]any{"tool": "Bash", "command": "ls"}, true},
		{policy, events.ActionApproved, "toolu_1", map[string]any{"reason": "stays inside the worktree"}, true},
		{policy, events.ActionDenied, "toolu_1", map[string]any{"reason": "x"}, false}, // decided once
		{worker, events.ActionResult, "toolu_1", nil, true},
		turnEnded("turn-2", "engineer", "Removed 3 TODOs."),
		turnStarted("turn-3", "tech-lead"),
		turnEnded("turn-3", "tech-lead", `{"action":"done"}`),
		{coordinator, events.TaskDone, "", map[string]any{"summary": "TODOs removed"}, true},
		{agentry, events.IssueRaised, "", map[string]any{"reason": "late"}, false}, // nothing after done
	})

	if task.Status() != tasks.Done || len(task.Turns) != 3 || !task.Turns[0].Acknowledged {
		t.Errorf("status %s, %d turns, first acknowledged %v", task.Status(), len(task.Turns), task.Turns[0].Acknowledged)
	}
	want := []tasks.Step{
		{From: "you", To: "tech-lead", Text: "Remove the TODOs"},
		{From: "tech-lead", To: "engineer", Text: "remove them"},
		{From: "engineer", To: "tech-lead", Text: "Removed 3 TODOs."},
		{From: "tech-lead", To: "you", Text: "TODOs removed"},
	}
	if len(task.Trail) != len(want) {
		t.Fatalf("trail: %+v", task.Trail)
	}
	for i, w := range want {
		if got := task.Trail[i]; got.From != w.From || got.To != w.To || got.Text != w.Text {
			t.Errorf("trail %d: got %s → %s %q, want %s → %s %q", i, got.From, got.To, got.Text, w.From, w.To, w.Text)
		}
	}
}

func TestOnlyAskedActionsAndQuestionsNeedYou(t *testing.T) {
	steps := []step{
		created, worktree, turnStarted("turn-1", "tech-lead"), turnEnded("turn-1", "tech-lead", "assign"),
		{coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"}, true},
		turnStarted("turn-2", "engineer"),
		{engineer, events.ActionRequested, "toolu_1", map[string]any{"tool": "Bash", "command": "git push"}, true},
	}
	if task := run(t, steps); task.Status() != tasks.Working {
		t.Errorf("a requested action alone should not need you (proof 05), got %s", task.Status())
	}

	steps = append(steps, step{policy, events.ActionAsked, "toolu_1", map[string]any{"reason": "may reach outside the worktree"}, true})
	if task := run(t, steps); task.Status() != tasks.NeedsYou || len(task.WaitingForYou()) != 1 {
		t.Errorf("an asked action should need you, got %s", task.Status())
	}

	steps = append(steps,
		step{you, events.ActionApproved, "toolu_1", map[string]any{"reason": "you approved"}, true},
		step{agentry, events.IssueRaised, "", map[string]any{"reason": "the agent's process exited"}, true},
		step{agentry, events.TaskNeedsYou, "", map[string]any{"question": "too many turns"}, true},
		step{coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "again"}, false},
		step{you, events.TaskAnswered, "", map[string]any{"answer": "carry on"}, true},
		step{you, events.TaskAnswered, "", map[string]any{"answer": "twice"}, false},
	)
	task := run(t, steps)
	if task.Status() != tasks.Working || task.Turns[1].Issue != "the agent's process exited" || task.OpenTurn() != nil {
		t.Errorf("status %s, issue %q, open turn %v", task.Status(), task.Turns[1].Issue, task.OpenTurn())
	}
}

func TestEveryTaskEventHasARule(t *testing.T) {
	for _, kind := range []events.Type{events.WorktreeReady, events.TaskAssigned, events.TurnStarted,
		events.PromptAcknowledged, events.SessionStarted, events.ActionRequested, events.ActionAsked,
		events.ActionApproved, events.ActionDenied, events.ActionResult, events.AgentWaiting, events.TurnEnded,
		events.IssueRaised, events.TaskNeedsYou, events.TaskAnswered, events.TaskDone} {
		if taskRules[kind] == nil {
			t.Errorf("%s has no rule", kind)
		}
	}
}

func TestAgentRules(t *testing.T) {
	s := openSeeded(t)
	agent := func(name, engine, prompt string, coordinates bool) agents.Agent {
		return agents.Agent{Name: name, Engine: engine, Prompt: prompt, Coordinates: coordinates}
	}
	cases := []struct {
		name     string
		proposal events.Proposal
		accepted bool
	}{
		{"a new agent", agent("designer", "codex", "Design the page.", false).Proposal(you, false), true},
		{"the same name twice", agent("designer", "claude", "Again.", false).Proposal(you, false), false},
		{"a name that can't be a file name", agent("Data Scientist", "claude", "x", false).Proposal(you, false), false},
		{"an unknown engine", agent("writer", "gpt-cli", "x", false).Proposal(you, false), false},
		{"no prompt", agent("writer", "claude", "  ", false).Proposal(you, false), false},
		{"change an agent", agent("engineer", "codex", "Only Go.", false).Proposal(you, true), true},
		{"change one that doesn't exist", agent("ghost", "claude", "x", false).Proposal(you, true), false},
		{"leave no coordinator", agent("tech-lead", "claude", "x", false).Proposal(you, true), false},
		{"a worker that doesn't exist", agents.Agent{Name: "tester", Engine: "claude", Prompt: "x", Workers: []string{"build-box"}}.Proposal(you, true), false},
		{"no workers: runs on this machine", agents.Agent{Name: "tester", Engine: "claude", Prompt: "x", Workers: nil}.Proposal(you, true), true},
		{"an agent keeps its own notes", agent("engineer", "", "", false).Learned(engineer, "Use gofmt."), true},
		{"not another's", agent("reviewer", "", "", false).Learned(engineer, "x"), false},
		{"you can edit anyone's", agent("reviewer", "", "", false).Learned(you, "Be strict."), true},
		{"remove an agent", agent("designer", "codex", "x", false).Removal(you), true},
		{"remove it twice", agent("designer", "codex", "x", false).Removal(you), false},
		{"remove the only coordinator", agent("tech-lead", "claude", "x", true).Removal(you), false},
		{"a second team, with its coordinator", agents.Agent{Name: "editor", Engine: "claude", Prompt: "Edit.", Coordinates: true, Team: "Writing"}.Proposal(you, false), true},
		{"and its writer", agents.Agent{Name: "writer", Engine: "claude", Prompt: "Write.", Team: "Writing"}.Proposal(you, false), true},
		{"its coordinator can't stop coordinating", agents.Agent{Name: "editor", Engine: "claude", Prompt: "Edit.", Team: "Writing"}.Proposal(you, true), false},
		{"or go while the team has agents", agents.Agent{Name: "editor"}.Removal(you), false},
		{"a coordinator hands work only to its own team", events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"},
			Type: events.TaskAssigned, Data: map[string]any{"agent": "writer", "instructions": "x"}}, false},
		{"an agent makes a task only for another, as a request", events.Proposal{Stream: "task-9", Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"},
			Type: events.TaskCreated, Data: map[string]any{"title": "x", "repo": "/r", "coordinator": "tech-lead"}}, false},
		{"a task for a coordinator that isn't one", events.Proposal{Stream: "task-9", Actor: you, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": "/r", "coordinator": "engineer"}}, false},
	}
	for _, c := range cases {
		_, rejection, err := s.Propose(c.proposal)
		if err != nil {
			t.Fatal(err)
		}
		if (rejection == nil) != c.accepted {
			t.Errorf("%s: accepted=%v (%v), want %v", c.name, rejection == nil, rejection, c.accepted)
		}
	}
}

// The coordinator (or you) may give the task a better title, once it has
// read it; the first title is kept.
func TestTheTaskCanBeRenamed(t *testing.T) {
	task := run(t, []step{
		created,
		{engineer, events.TaskRenamed, "", map[string]any{"title": "Something else"}, false},      // only the coordinator
		{coordinator, events.TaskRenamed, "", map[string]any{"title": "Remove the TODOs"}, false}, // the same title
		{coordinator, events.TaskRenamed, "", map[string]any{"title": "Remove the TODO comments in src/"}, true},
		{you, events.TaskRenamed, "", map[string]any{"title": "Clean up src/"}, true},
	})
	if task.Title != "Clean up src/" || task.FirstTitle != "Remove the TODOs" {
		t.Errorf("title %q, first title %q", task.Title, task.FirstTitle)
	}
}

// A budget and a limit of turns are 0 or above, 0 for no limit.
func TestBudgetsAndLimitsOfTurns(t *testing.T) {
	task := run(t, []step{
		{you, events.TaskCreated, "", map[string]any{"title": "x", "repo": "/r", "coordinator": "tech-lead", "maxTurns": -1}, false},
		{you, events.TaskCreated, "", map[string]any{"title": "x", "repo": "/r", "coordinator": "tech-lead", "budget": -1}, false},
		{you, events.TaskCreated, "", map[string]any{"title": "x", "repo": "/r", "coordinator": "tech-lead", "budget": 0, "maxTurns": 50}, true},
		{you, events.TaskTurnsLimited, "", map[string]any{"maxTurns": -3}, false},
		{coordinator, events.TaskTurnsLimited, "", map[string]any{"maxTurns": 100}, false}, // only you
		{you, events.TaskTurnsLimited, "", map[string]any{"maxTurns": 0}, true},
		{you, events.TaskBudgeted, "", map[string]any{"budget": -1}, false},
		{you, events.TaskBudgeted, "", map[string]any{"budget": 2_000_000}, true},
		{you, events.TaskBudgeted, "", map[string]any{"budget": 0}, true},
	})
	if task.MaxTurns != 0 || task.Budget != 0 {
		t.Errorf("max turns %d, budget %d", task.MaxTurns, task.Budget)
	}
}

// Directions from the board are noted; a question takes an answer instead.
func TestDirectionsWaitForNoQuestion(t *testing.T) {
	task := run(t, []step{
		created,
		{you, events.TaskDirected, "", map[string]any{"text": "Hand this task to reviewer next."}, true},
		{coordinator, events.TaskDirected, "", map[string]any{"text": "x"}, false}, // only you
		worktree,
		turnStarted("turn-1", "tech-lead"),
		turnEnded("turn-1", "tech-lead", `{"action":"ask"}`),
		{coordinator, events.TaskNeedsYou, "", map[string]any{"question": "Which branch?"}, true},
		{you, events.TaskDirected, "", map[string]any{"text": "Wrap it up."}, false},
	})
	if steps := task.Trail; steps[1].Text != "Hand this task to reviewer next." || steps[1].From != "you" {
		t.Errorf("trail: %+v", steps)
	}
}

// Your moves on the board stand: the task is done or parked at once, the
// turn in progress stops, and the coordinator only wraps up, in one turn.
func TestYourMoveToDoneStands(t *testing.T) {
	s := openSeeded(t)
	proposeAll(t, s, []step{
		created, worktree, turnStarted("turn-1", "tech-lead"), turnEnded("turn-1", "tech-lead", "assign"),
		{coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"}, true},
		turnStarted("turn-2", "engineer"),
		{you, events.TaskDone, "", map[string]any{"summary": "Moved.", "moved": true}, true}, // the engineer's turn stops
		turnStarted("turn-3", "tech-lead"),                                                   // the coordinator's wrap-up
		turnEnded("turn-3", "tech-lead", `{"action":"assign"}`),
	})
	const stays = "the user moved this task to Done; it stays done"
	refusedWith(t, s, coordinator, events.TaskAssigned, map[string]any{"agent": "engineer", "instructions": "more"}, stays)
	refusedWith(t, s, coordinator, events.TaskNeedsYou, map[string]any{"question": "Reopen it?"}, stays)
	refusedWith(t, s, coordinator, events.TaskDone, map[string]any{"summary": "again"}, stays)
	proposeAll(t, s, []step{
		{coordinator, events.TaskWrappedUp, "", map[string]any{"summary": "The parser is half done, committed."}, true},
		{coordinator, events.TaskWrappedUp, "", map[string]any{"summary": "twice"}, false},
		{worker, events.TurnStarted, "turn-4", map[string]any{"agent": "tech-lead", "hash": "h"}, false}, // nothing more runs
	})
	task := taskIn(s)
	if !task.IsDone || !task.DoneByYou || task.WrapUp != nil || !task.Turns[1].Stopped || task.Summary != "The parser is half done, committed." {
		t.Errorf("done %v by you %v, wrap-up %+v, engineer stopped %v, summary %q", task.IsDone, task.DoneByYou, task.WrapUp, task.Turns[1].Stopped, task.Summary)
	}
}

func TestYourMoveToToDoStands(t *testing.T) {
	s := openSeeded(t)
	proposeAll(t, s, []step{
		created, worktree, turnStarted("turn-1", "tech-lead"),
		{you, events.TaskHeld, "", map[string]any{"place": "todo"}, true}, // the coordinator's turn stops
		turnStarted("turn-2", "tech-lead"),                                // its wrap-up
		turnEnded("turn-2", "tech-lead", `{"action":"assign"}`),
	})
	refusedWith(t, s, coordinator, events.TaskAssigned, map[string]any{"agent": "engineer", "instructions": "go"},
		"the user moved this task to To do; it waits there until they move it on")
	proposeAll(t, s, []step{{coordinator, events.TaskWrappedUp, "", map[string]any{"summary": "Nothing started yet."}, true}})
	if task := taskIn(s); task.Status() != tasks.Held || task.WrapUp != nil || !task.Turns[0].Stopped || task.Told.Text != "Nothing started yet." {
		t.Errorf("status %s, wrap-up %+v, first turn stopped %v, told %q", task.Status(), task.WrapUp, task.Turns[0].Stopped, task.Told.Text)
	}
}

// Your move to Review or back to In progress stands until the coordinator
// hands the task on, and it hands it to an agent that fits the move.
func TestYourMoveToReviewStands(t *testing.T) {
	s := openSeeded(t)
	assign := func(agent string, checks bool) map[string]any {
		return map[string]any{"agent": agent, "instructions": "go", "checks": checks}
	}
	proposeAll(t, s, []step{
		created, worktree, turnStarted("turn-1", "tech-lead"), turnEnded("turn-1", "tech-lead", "assign"),
		{coordinator, events.TaskAssigned, "", assign("engineer", false), true},
		turnStarted("turn-2", "engineer"), turnEnded("turn-2", "engineer", "Built."),
		{you, events.TaskDirected, "", map[string]any{"text": "Have this task reviewed next.", "column": "review"}, true},
		turnStarted("turn-3", "tech-lead"), turnEnded("turn-3", "tech-lead", "assign"),
	})
	refusedWith(t, s, coordinator, events.TaskAssigned, assign("engineer", false),
		"the user moved this task to Review; hand it to an agent that checks work")
	proposeAll(t, s, []step{
		{coordinator, events.TaskAssigned, "", assign("reviewer", true), true},
		turnStarted("turn-4", "reviewer"), turnEnded("turn-4", "reviewer", "Looks fine."),
	})
	if task := taskIn(s); task.Column != "" {
		t.Errorf("the reviewer has it: column %q, want the stage's", task.Column)
	}

	proposeAll(t, s, []step{
		{you, events.TaskDirected, "", map[string]any{"text": "This needs more work.", "column": "in-progress", "sentBack": true}, true},
		turnStarted("turn-5", "tech-lead"), turnEnded("turn-5", "tech-lead", "assign"),
	})
	refusedWith(t, s, coordinator, events.TaskAssigned, assign("tester", true),
		"the user moved this task back to In progress; hand it to an agent that builds it")
	proposeAll(t, s, []step{{coordinator, events.TaskAssigned, "", assign("engineer", false), true}})

	// Started from To do rather than sent back from Review: anyone may take it.
	proposeAll(t, s, []step{
		turnStarted("turn-6", "engineer"), turnEnded("turn-6", "engineer", "Fixed."),
		{you, events.TaskHeld, "", map[string]any{"place": "todo"}, true},
		{you, events.TaskHeld, "", map[string]any{"place": "", "column": "in-progress"}, true},
		turnStarted("turn-7", "tech-lead"), turnEnded("turn-7", "tech-lead", "assign"),
		{coordinator, events.TaskAssigned, "", assign("reviewer", true), true},
	})
}

// A team with no agent that checks work can't have a task checked: its
// coordinator hands it to a builder, and the task stays in Review.
func TestReviewWithNoChecker(t *testing.T) {
	s := openSeeded(t)
	for _, name := range []string{"reviewer", "tester"} {
		if _, rejection, err := s.Propose(events.Proposal{Stream: events.AgentsStream, Actor: you, Type: events.AgentRemoved,
			Data: map[string]any{"name": name}}); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	proposeAll(t, s, []step{
		created, worktree, turnStarted("turn-1", "tech-lead"), turnEnded("turn-1", "tech-lead", "assign"),
		{you, events.TaskDirected, "", map[string]any{"text": "Have this task reviewed next.", "column": "review"}, true},
		{coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "check it", "checks": false}, true},
	})
	if task := taskIn(s); task.Column != "review" {
		t.Errorf("column %q, want it kept in review", task.Column)
	}
}

// Only your move gives a wrap-up, and only the board's Done finishes a
// story for you.
func TestNoWrapUpWithoutYourMove(t *testing.T) {
	s := openSeeded(t)
	proposeAll(t, s, []step{
		created, worktree, turnStarted("turn-1", "tech-lead"), turnEnded("turn-1", "tech-lead", "x"),
		{coordinator, events.TaskWrappedUp, "", map[string]any{"summary": "x"}, false},
	})
	refusedWith(t, s, you, events.TaskDone, map[string]any{"summary": "x"},
		"a task is finished by its coordinator; move it to Done on the board to have it finished as it is")
}

// proposeAll proposes the steps to task-1.
func proposeAll(t *testing.T, s *store.Store, steps []step) {
	t.Helper()
	for i, st := range steps {
		_, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: st.actor, Type: st.kind, Correlation: st.correlation, Data: st.data})
		if err != nil {
			t.Fatal(err)
		}
		if (rejection == nil) != st.accepted {
			t.Errorf("step %d %s by %s: accepted=%v (%v), want %v", i+1, st.kind, st.actor, rejection == nil, rejection, st.accepted)
		}
	}
}

func refusedWith(t *testing.T, s *store.Store, actor events.Actor, kind events.Type, data map[string]any, reason string) {
	t.Helper()
	_, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Data: data})
	if err != nil || rejection == nil || rejection.Reason != reason {
		t.Errorf("%s by %s: %v %v, want refused: %s", kind, actor, rejection, err, reason)
	}
}

func taskIn(s *store.Store) *tasks.Task {
	stream, _ := s.Stream("task-1")
	return tasks.FromEvents(stream)
}

// brokenStream is a state whose one stream could not be built.
type brokenStream struct {
	*current.State
	stream string
}

func (b brokenStream) Broken(stream string) error {
	if stream == b.stream {
		return errors.New("applying it panicked")
	}
	return b.State.Broken(stream)
}

// A roster whose stream broke refuses what would be checked against it,
// as an unreadable stream did when the rules replayed the log.
func TestABrokenRosterRefusesWhatItWouldCheck(t *testing.T) {
	_, joined := workers.Join("laptop", "a-code", workers.Machine{})
	_, connected := connections.Connect("a-code", connections.Machine{})
	cases := []struct {
		stream string
		p      events.Proposal
		want   string
	}{
		{events.AgentsStream, events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: created.data}, "could not read the agents: applying it panicked"},
		{events.WorkersStream, joined, "could not read the workers: applying it panicked"},
		{events.ConnectionsStream, connected, "could not read the connections: applying it panicked"},
	}
	for _, c := range cases {
		state := current.New()
		state.Loaded(0)
		if rejection := Check(c.p, brokenStream{state, c.stream}); rejection == nil || rejection.Reason != c.want {
			t.Errorf("%s broken, %s: %v, want %q", c.stream, c.p.Type, rejection, c.want)
		}
	}
}
