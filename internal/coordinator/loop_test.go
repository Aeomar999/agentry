package coordinator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/turns"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// The whole loop on a real store, tmux and git, with a fake `claude` that
// just waits. The test plays the agents: it ends each turn with a report,
// the way the Stop hook would.
func TestTheLoopTakesATaskRoundTheCoordinator(t *testing.T) {
	fakeClaudeOnPath(t)
	repo := gitRepo(t)
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), rules.Check)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	const id = "task-loop-1"
	defer sessions.Stop(sessions.SocketFor(id))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	loop := newLoop(s, t.TempDir())
	go loop.Run(ctx)

	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Remove the TODOs", "repo": repo, "coordinator": "tech-lead"}})

	task := waitForTurn(t, s, id, "turn-1", "tech-lead")
	if task.Worktree == nil || !strings.HasSuffix(task.Worktree.Path, id) || task.Worktree.Port == 0 {
		t.Fatalf("worktree: %+v", task.Worktree)
	}
	brief := readBrief(t, task, "turn-1")
	for _, want := range []string{"You are the tech lead", "- engineer: ", "1. you → tech-lead: Remove the TODOs", `"action": "assign"`} {
		if !strings.Contains(brief, want) {
			t.Errorf("the coordinator's brief is missing %q", want)
		}
	}

	// The coordinator renames the task as it decides.
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Remove every TODO in src/.", "title": "Remove the TODO comments in src/"}`)
	task = waitForTurn(t, s, id, "turn-2", "engineer")
	if task.Title != "Remove the TODO comments in src/" || task.FirstTitle != "Remove the TODOs" {
		t.Errorf("title %q, first title %q", task.Title, task.FirstTitle)
	}
	if brief := readBrief(t, task, "turn-2"); !strings.Contains(brief, "You are the engineer") || !strings.Contains(brief, "Remove every TODO in src/.") {
		t.Errorf("the engineer's brief:\n%s", brief)
	}

	commitIn(t, task.Worktree.Path) // the engineer's work, not yet in dev
	endTurn(t, s, id, "turn-2", "engineer", "Removed 3 TODOs and committed.")
	task = waitForTurn(t, s, id, "turn-3", "tech-lead")
	if brief := readBrief(t, task, "turn-3"); !strings.Contains(brief, "engineer → tech-lead: Removed 3 TODOs and committed.") {
		t.Errorf("the engineer's report should be in the coordinator's brief:\n%s", brief)
	}

	// Unreviewed work can't be done: the tech lead hears why.
	endTurn(t, s, id, "turn-3", "tech-lead", `{"action": "done", "summary": "TODOs removed."}`)
	task = waitForTurn(t, s, id, "turn-4", "tech-lead")
	if brief := readBrief(t, task, "turn-4"); !strings.Contains(brief, "no one has checked it since it last changed (turn-2, by engineer)") {
		t.Errorf("the tech lead should hear the work needs checking:\n%s", brief)
	}

	// The reviewer wants a change; once it is made, it reviews again.
	endTurn(t, s, id, "turn-4", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Review it."}`)
	task = waitForTurn(t, s, id, "turn-5", "reviewer")
	if brief := readBrief(t, task, "turn-5"); !strings.Contains(brief, "Verdict: changes needed") {
		t.Errorf("the reviewer should be told how to give its verdict:\n%s", brief)
	}
	endTurn(t, s, id, "turn-5", "reviewer", "One TODO is left in src/b.go.\n\n**Verdict: changes needed**")
	waitForTurn(t, s, id, "turn-6", "tech-lead")
	endTurn(t, s, id, "turn-6", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Remove the TODO in src/b.go."}`)
	waitForTurn(t, s, id, "turn-7", "engineer")
	endTurn(t, s, id, "turn-7", "engineer", "Removed it.")
	waitForTurn(t, s, id, "turn-8", "tech-lead")
	endTurn(t, s, id, "turn-8", "tech-lead", `{"action": "done", "summary": "TODOs removed."}`)
	task = waitForTurn(t, s, id, "turn-9", "tech-lead")
	if brief := readBrief(t, task, "turn-9"); !strings.Contains(brief, "reviewer checked it in turn-5, and it changed after that (turn-7, by engineer)") {
		t.Errorf("a change after a review needs another review:\n%s", brief)
	}
	endTurn(t, s, id, "turn-9", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Review the fix."}`)
	waitForTurn(t, s, id, "turn-10", "reviewer")
	endTurn(t, s, id, "turn-10", "reviewer", "All TODOs are gone.\n\nVerdict: approved")

	// The deployer ships it without changing it, so the review still holds.
	waitForTurn(t, s, id, "turn-11", "tech-lead")
	endTurn(t, s, id, "turn-11", "tech-lead", `{"action": "assign", "agent": "deployer", "instructions": "Merge it."}`)
	waitForTurn(t, s, id, "turn-12", "deployer")
	endTurn(t, s, id, "turn-12", "deployer", "Merged.")
	waitForTurn(t, s, id, "turn-13", "tech-lead")
	endTurn(t, s, id, "turn-13", "tech-lead", `{"action": "done", "summary": "TODOs removed."}`)
	task = waitUntil(t, s, id, "done", func(task *tasks.Task) bool { return task.IsDone })
	if task.Summary != "TODOs removed." || sessions.IsRunning(sessions.SocketFor(id)) {
		t.Errorf("summary %q, session still running %v", task.Summary, sessions.IsRunning(sessions.SocketFor(id)))
	}
	if _, err := os.Stat(task.Worktree.Path); err != nil {
		t.Errorf("the branch isn't merged, so the worktree must be kept: %v", err)
	}
}

// An agent that runs only on a worker that is offline doesn't start: the
// task asks you, saying why, instead of waiting where nobody sees it.
func TestAnAgentWhoseWorkersAreOfflineAsksYou(t *testing.T) {
	fakeClaudeOnPath(t)
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), rules.Check)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	code, invited := workers.Invite(you, time.Now())
	_, joined := workers.Join("laptop", code, workers.Machine{})
	s.Propose(invited)
	s.Propose(joined)
	engineer, _ := agents.FromEvents(mustStream(t, s, events.AgentsStream)).Get("engineer")
	engineer.Workers = []string{"laptop"}
	if _, rejection, _ := s.Propose(engineer.Proposal(you, true)); rejection != nil {
		t.Fatal(rejection)
	}
	const id = "task-loop-2"
	defer sessions.Stop(sessions.SocketFor(id))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go newLoop(s, t.TempDir()).Run(ctx)

	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Remove the TODOs", "repo": gitRepo(t), "coordinator": "tech-lead"}})
	waitForTurn(t, s, id, "turn-1", "tech-lead")
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Go."}`)
	task := waitUntil(t, s, id, "a question", func(task *tasks.Task) bool { return task.Question != "" })
	if !strings.Contains(task.Question, "engineer can't run anywhere now: laptop is offline") || len(task.Turns) != 1 {
		t.Errorf("question %q after %d turns", task.Question, len(task.Turns))
	}
}

func mustStream(t *testing.T, s *store.Store, name string) []events.Event {
	t.Helper()
	stream, err := s.Stream(name)
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

func fakeClaudeOnPath(t *testing.T) {
	bin := t.TempDir()
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\nsleep 60\n"), 0o755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
}

func gitRepo(t *testing.T) string {
	repo := t.TempDir()
	git(t, repo, "init", "-q", "-b", "dev")
	git(t, repo, "commit", "-q", "--allow-empty", "-m", "first")
	return repo
}

func commitIn(t *testing.T, dir string) {
	git(t, dir, "commit", "-q", "--allow-empty", "-m", "fix: remove the TODOs")
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", dir, "-c", "user.name=t", "-c", "user.email=t@e"}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v %s", args, err, output)
	}
}

var worker = events.Actor{Kind: events.Worker, ID: "local"}

func endTurn(t *testing.T, s *store.Store, id, turn, agent, report string) {
	t.Helper()
	_, rejection, err := s.Propose(events.Proposal{Stream: id, Actor: worker, Type: events.TurnEnded, Correlation: turn,
		Data: map[string]any{"agent": agent, "report": report}})
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
}

func waitForTurn(t *testing.T, s *store.Store, id, turn, agent string) *tasks.Task {
	t.Helper()
	return waitUntil(t, s, id, agent+" in "+turn, func(task *tasks.Task) bool {
		open := task.OpenTurn()
		return open != nil && open.ID == turn && open.Agent == agent
	})
}

func waitUntil(t *testing.T, s *store.Store, id, what string, condition func(*tasks.Task) bool) *tasks.Task {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		stream, _ := s.Stream(id)
		if task := tasks.FromEvents(stream); task != nil && condition(task) {
			return task
		}
	}
	stream, _ := s.Stream(id)
	all, _ := s.After(0, 1000)
	var log []string
	for _, e := range all {
		if e.Stream == id {
			log = append(log, string(e.Type)+" "+e.Text("reason"))
		}
	}
	t.Fatalf("timed out waiting for %s; task: %+v\nlog:\n%s", what, tasks.FromEvents(stream), strings.Join(log, "\n"))
	return nil
}

func readBrief(t *testing.T, task *tasks.Task, turn string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(task.Worktree.Path, machines.StateDir, turn+".md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(content)
}

// newLoop is a loop with this machine only, its worktrees in root.
func newLoop(s *store.Store, root string) *Loop {
	fleet := machines.NewFleet(&machines.Local{Label: workers.Local, ServerURL: "http://x", HookCommand: "agentry hook", WorktreeRoot: root})
	return NewLoop(s, turns.NewRunner(s, fleet, turns.Config{}), fleet)
}
