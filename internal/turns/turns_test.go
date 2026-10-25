package turns

import (
	"cmp"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/engines"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/sessions/sessionstest"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// fakeClaude puts a `claude` on PATH that writes its arguments and
// environment to a file, then waits like a real session, in sessions of
// the test's own.
func fakeClaude(t *testing.T) (record string) {
	t.Helper()
	sessionstest.Isolate(t)
	bin := t.TempDir()
	record = filepath.Join(bin, "record.txt")
	// Written whole, then moved into place: the test reads it the moment it exists.
	script := "#!/bin/sh\n{ printf '%s\\n' \"$@\"; env | grep -E '^(AGENTRY_|PORT=)' | sort; } > " + record + ".part && mv " +
		record + ".part " + record + "\n" + sessionstest.Wait() + "\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	return record
}

func taskWithWorktree(t *testing.T, s *store.Store, id string) *tasks.Task {
	t.Helper()
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	you := events.Actor{Kind: events.Human, ID: "you"}
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	worktree := t.TempDir()
	for _, p := range []events.Proposal{
		{Stream: id, Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Remove the TODOs", "repo": "/r", "coordinator": "tech-lead"}},
		{Stream: id, Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": worktree, "branch": "agentry/" + id, "port": 7901}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	stream, _ := s.Stream(id)
	return tasks.FromEvents(stream)
}

func TestStartRunsTheAgentOnAOneLinePrompt(t *testing.T) {
	record := fakeClaude(t)
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	task := taskWithWorktree(t, s, "task-1")
	defer sessions.Stop(sessions.SocketFor(task.ID))
	coordinator, _ := agents.FromEvents(mustStream(t, s, events.AgentsStream)).Get("tech-lead")

	runner := NewRunner(s, state, here(), Config{HookToken: "secret"})
	if err := runner.Start(task, coordinator, "# Brief\nDecide who works next.", runner.SessionToResume(task, coordinator)); err != nil {
		t.Fatal(err)
	}

	brief, _ := os.ReadFile(filepath.Join(task.Worktree.Path, ".agentry", "turn-1.md"))
	if string(brief) != "# Brief\nDecide who works next." {
		t.Errorf("brief file: %q", brief)
	}
	if _, err := os.Stat(filepath.Join(task.Worktree.Path, ".agentry", "claude-settings.json")); err != nil {
		t.Errorf("hook settings: %v", err)
	}
	updated := tasks.FromEvents(mustStream(t, s, task.ID))
	prompt := "Read .agentry/turn-1.md and do what it says."
	if turn := updated.OpenTurn(); turn == nil || turn.Agent != "tech-lead" || turn.Hash != events.Hash(prompt) {
		t.Fatalf("open turn: %+v", turn)
	}
	for _, e := range mustStream(t, s, task.ID) {
		if _, given := e.Data["seen"]; e.Type == events.TurnStarted && (!given || e.Number("seen") != len(task.Trail)) {
			t.Errorf("turn.started says its brief saw %v steps of the trail, want %d", e.Data["seen"], len(task.Trail))
		}
	}

	var launched string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && launched == ""; time.Sleep(50 * time.Millisecond) {
		content, _ := os.ReadFile(record)
		launched = string(content)
	}
	for _, want := range []string{"--permission-mode\nbypassPermissions\n", prompt + "\n",
		"AGENTRY_AGENT=tech-lead", "AGENTRY_TASK=task-1", "AGENTRY_TURN=turn-1", "AGENTRY_HOOK_TOKEN=secret",
		"AGENTRY_HOOK_DEADLINE=29m50s", "AGENTRY_SERVER=http://127.0.0.1:7800", "AGENTRY_WORKTREE=" + task.Worktree.Path, "PORT=7901"} {
		if !strings.Contains(launched, want) {
			t.Errorf("launch is missing %q:\n%s", want, launched)
		}
	}
}

// The hook gives up on an ask before the engine's own timeout kills it, so
// the server holds the action for you and the gate fails closed.
func TestTheHookGivesUpBeforeTheEngineKillsIt(t *testing.T) {
	task := &tasks.Task{ID: "task-1", Worktree: &tasks.Worktree{Port: 7901}}
	env := (&Runner{}).environment(task, "turn-1", agents.Agent{Name: "engineer", Engine: "claude"})
	deadline, err := time.ParseDuration(env[hookcall.EnvDeadline])
	if killed := engines.GateTimeoutSeconds * time.Second; err != nil || deadline <= 0 || deadline >= killed {
		t.Errorf("the hook's deadline %q should be under the engine's timeout, %s (%v)", env[hookcall.EnvDeadline], killed, err)
	}
}

func TestAnEngineWithNoAdapterIsRefused(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	task := taskWithWorktree(t, s, "task-1")
	err := NewRunner(s, state, here(), Config{}).Start(task, agents.Agent{Name: "x", Engine: "gpt-cli"}, "brief", Session{Fresh: "first turn"})
	if err == nil || !strings.Contains(err.Error(), "no adapter") {
		t.Errorf("got %v", err)
	}
}

// here is a fleet of this machine only.
func here() *machines.Fleet {
	return machines.NewFleet(&machines.Local{Label: workers.Local, ServerURL: "http://127.0.0.1:7800", HookCommand: "/bin/agentry hook"})
}

func mustStream(t *testing.T, s *store.Store, name string) []events.Event {
	t.Helper()
	stream, err := s.Stream(name)
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

func TestTypeLineSendsALineToTheSession(t *testing.T) {
	sessionstest.Isolate(t)
	bin := t.TempDir()
	typed := filepath.Join(bin, "typed.txt")
	// Reads one line from its terminal, as an engine waiting at its prompt.
	script := "#!/bin/sh\nread line; printf '%s' \"$line\" > " + typed + ".part && mv " + typed + ".part " + typed + "\n" + sessionstest.Wait() + "\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	task := taskWithWorktree(t, s, "task-1")
	defer sessions.Stop(sessions.SocketFor(task.ID))
	coordinator, _ := agents.FromEvents(mustStream(t, s, events.AgentsStream)).Get("tech-lead")
	runner := NewRunner(s, state, here(), Config{})
	if err := runner.Start(task, coordinator, "brief", runner.SessionToResume(task, coordinator)); err != nil {
		t.Fatal(err)
	}

	if err := runner.TypeLine(task.ID, "Carry on."); err != nil {
		t.Fatal(err)
	}
	var line string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && line == ""; time.Sleep(50 * time.Millisecond) {
		content, _ := os.ReadFile(typed)
		line = string(content)
	}
	if line != "Carry on." {
		t.Errorf("the session read %q", line)
	}
}

func TestTypeLineIntoNoSessionIsAnError(t *testing.T) {
	sessionstest.Isolate(t)
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	task := taskWithWorktree(t, s, "task-1")
	if err := NewRunner(s, state, here(), Config{}).TypeLine(task.ID, "Carry on."); err == nil {
		t.Error("typing into a session that isn't running should fail")
	}
}

// The agent's next turn on the task goes on with its last session; a first
// one, or any with resume off, starts fresh and says why.
func TestATurnGoesOnWithTheAgentsLastSession(t *testing.T) {
	for _, c := range []struct {
		name    string
		config  Config
		resumes bool
		fresh   string
	}{
		{"resume on", Config{}, true, ""},
		{"resume off", Config{NoResume: true}, false, ResumeOff},
	} {
		t.Run(c.name, func(t *testing.T) {
			record := fakeClaude(t)
			s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
			task := taskWithWorktree(t, s, "task-1")
			defer sessions.Stop(sessions.SocketFor(task.ID))
			coordinator, _ := agents.FromEvents(mustStream(t, s, events.AgentsStream)).Get("tech-lead")
			runner := NewRunner(s, state, here(), c.config)

			if err := runner.Start(task, coordinator, "brief", runner.SessionToResume(task, coordinator)); err != nil {
				t.Fatal(err)
			}
			if first := launched(t, record); strings.Contains(first, "--resume") {
				t.Errorf("a first turn resumes nothing:\n%s", first)
			}
			first := tasks.FromEvents(mustStream(t, s, task.ID)).Turn("turn-1")
			if want := cmp.Or(c.fresh, "first turn"); first.FromSession != "" || first.FreshBecause != want {
				t.Errorf("first turn: from %q, fresh because %q; want fresh because %q", first.FromSession, first.FreshBecause, want)
			}
			endTurn(t, s, "turn-1", "tech-lead", "s-1")

			ended := tasks.FromEvents(mustStream(t, s, task.ID))
			if err := runner.Start(ended, coordinator, "brief", runner.SessionToResume(ended, coordinator)); err != nil {
				t.Fatal(err)
			}
			second := tasks.FromEvents(mustStream(t, s, task.ID)).Turn("turn-2")
			if resumed := strings.Contains(launched(t, record), "--resume\ns-1\n"); resumed != c.resumes {
				t.Errorf("second turn launched with --resume s-1: %v, want %v", resumed, c.resumes)
			}
			if c.resumes && (second.FromSession != "s-1" || second.FreshBecause != "") {
				t.Errorf("second turn: from %q, fresh because %q; want from s-1", second.FromSession, second.FreshBecause)
			}
			if !c.resumes && (second.FromSession != "" || second.FreshBecause != c.fresh) {
				t.Errorf("second turn: from %q, fresh because %q; want fresh because %q", second.FromSession, second.FreshBecause, c.fresh)
			}
		})
	}
}

// A resumed session carries how much of the trail its last brief saw, so
// the brief tells it only what came after; a fresh one needs none of it.
func TestAResumedSessionKnowsHowMuchOfTheTrailItSaw(t *testing.T) {
	coordinator := agents.Agent{Name: "tech-lead", Engine: "claude"}
	last := &tasks.Turn{Agent: "tech-lead", Ended: true, Worker: workers.Local, Engine: "claude", EngineSession: "s-1", Seen: 3}
	task := &tasks.Task{ID: "task-1", Worktree: &tasks.Worktree{Worker: workers.Local}, Turns: []*tasks.Turn{last}}
	_, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	if got := NewRunner(nil, state, here(), Config{}).SessionToResume(task, coordinator); got != (Session{Resume: "s-1", From: "task-1", Seen: 3}) {
		t.Errorf("resumed session %+v, want it to go on with s-1 having seen 3 steps", got)
	}
	if got := NewRunner(nil, nil, here(), Config{NoResume: true}).SessionToResume(task, coordinator); got != (Session{Fresh: ResumeOff}) {
		t.Errorf("fresh session %+v, want only why it starts fresh", got)
	}
}

// One turn runs per task at a time: the log refuses a turn.started while
// one is open, or for a turn id already taken (rules noOpenTurn and
// isTheNextTurn), so no two turns go on with one session.
func TestTwoTurnsOfATaskNeverResumeTheSameSessionAtOnce(t *testing.T) {
	record := fakeClaude(t)
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	task := taskWithWorktree(t, s, "task-1")
	defer sessions.Stop(sessions.SocketFor(task.ID))
	coordinator, _ := agents.FromEvents(mustStream(t, s, events.AgentsStream)).Get("tech-lead")
	runner := NewRunner(s, state, here(), Config{})
	if err := runner.Start(task, coordinator, "brief", runner.SessionToResume(task, coordinator)); err != nil {
		t.Fatal(err)
	}
	launched(t, record)
	endTurn(t, s, "turn-1", "tech-lead", "s-1")

	// Both starts see the task as it was after turn 1, so both would resume s-1.
	ended := tasks.FromEvents(mustStream(t, s, task.ID))
	if err := runner.Start(ended, coordinator, "brief", runner.SessionToResume(ended, coordinator)); err != nil {
		t.Fatal(err)
	}
	if err := runner.Start(ended, coordinator, "brief", runner.SessionToResume(ended, coordinator)); err == nil {
		t.Error("a second start while a turn is open should be refused")
	}
	// The refused start raises an issue, which cuts turn-2 short too; what
	// counts here is that only one turn ever started from s-1.
	from := 0
	for _, turn := range tasks.FromEvents(mustStream(t, s, task.ID)).Turns {
		if turn.FromSession == "s-1" {
			from++
		}
	}
	if from != 1 {
		t.Errorf("%d turns went on with s-1, want 1", from)
	}
}

// launched waits for the fake claude's record of its launch and takes it,
// so the next launch writes a new one.
func launched(t *testing.T, record string) string {
	t.Helper()
	var content []byte
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && len(content) == 0; time.Sleep(50 * time.Millisecond) {
		content, _ = os.ReadFile(record)
	}
	if len(content) == 0 {
		t.Fatal("the engine didn't launch")
	}
	os.Remove(record)
	return string(content)
}

// endTurn logs the turn's session and its end, as its hooks would.
func endTurn(t *testing.T, s *store.Store, turn, agent, session string) {
	t.Helper()
	worker := events.Actor{Kind: events.Worker, ID: workers.Local}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: worker, Type: events.SessionStarted, Correlation: turn, Data: map[string]any{"engineSession": session}},
		{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: turn, Data: map[string]any{"agent": agent, "report": "done"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	sessions.Stop(sessions.SocketFor("task-1"))
}
