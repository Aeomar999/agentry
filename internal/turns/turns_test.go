package turns

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
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
	task := taskWithWorktree(t, s, "task-turns-1")
	defer sessions.Stop(sessions.SocketFor(task.ID))
	coordinator, _ := agents.FromEvents(mustStream(t, s, events.AgentsStream)).Get("tech-lead")

	runner := NewRunner(s, state, here(), Config{HookToken: "secret"})
	if err := runner.Start(task, coordinator, "# Brief\nDecide who works next."); err != nil {
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

	var launched string
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline) && launched == ""; time.Sleep(50 * time.Millisecond) {
		content, _ := os.ReadFile(record)
		launched = string(content)
	}
	for _, want := range []string{"--permission-mode\nbypassPermissions\n", prompt + "\n",
		"AGENTRY_AGENT=tech-lead", "AGENTRY_TASK=task-turns-1", "AGENTRY_TURN=turn-1", "AGENTRY_HOOK_TOKEN=secret",
		"AGENTRY_HOOK_DEADLINE=29m50s", "AGENTRY_SERVER=http://127.0.0.1:7800", "AGENTRY_WORKTREE=" + task.Worktree.Path, "PORT=7901"} {
		if !strings.Contains(launched, want) {
			t.Errorf("launch is missing %q:\n%s", want, launched)
		}
	}
}

func TestAnEngineWithNoAdapterIsRefused(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	task := taskWithWorktree(t, s, "task-turns-2")
	err := NewRunner(s, state, here(), Config{}).Start(task, agents.Agent{Name: "x", Engine: "gpt-cli"}, "brief")
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
	task := taskWithWorktree(t, s, "task-turns-3")
	defer sessions.Stop(sessions.SocketFor(task.ID))
	coordinator, _ := agents.FromEvents(mustStream(t, s, events.AgentsStream)).Get("tech-lead")
	runner := NewRunner(s, state, here(), Config{})
	if err := runner.Start(task, coordinator, "brief"); err != nil {
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
	task := taskWithWorktree(t, s, "task-turns-4")
	if err := NewRunner(s, state, here(), Config{}).TypeLine(task.ID, "Carry on."); err == nil {
		t.Error("typing into a session that isn't running should fail")
	}
}
