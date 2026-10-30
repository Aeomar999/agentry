package coordinator

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/platform/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/platform/sessions/sessionstest"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/turns"
	watchdogs "gitlab.com/dev.chempa/agentry/internal/watchdog"
)

// A resumed session that never takes its prompt is retried once, by the
// same agent with the same instructions, in a fresh session with the whole
// brief. If that fails too, the coordinator hears of it as of any issue.
func TestAFailedResumeIsRetriedFreshOnce(t *testing.T) {
	sessionstest.Isolate(t)
	bin, up := t.TempDir(), t.TempDir()
	// Its session marks which turn it is, so the test knows that turn's
	// session is up. One asked to resume exits soon, as claude does with a
	// session it doesn't have, and turn-5's crashes. Soon, not at once: the
	// runner is still waiting for the session's host, and the watchdog
	// stopping it then would be a second issue (its start grace sees to it).
	os.WriteFile(filepath.Join(bin, "claude"), []byte("#!/bin/sh\ntouch "+up+"/$"+hookcall.EnvTurn+
		"\ncase \"$*\" in *--resume*) echo 'No conversation found'; sleep 0.3; exit 1;; esac"+
		"\n[ \"$"+hookcall.EnvTurn+"\" = turn-5 ] && sleep 0.3 && exit 1\n"+sessionstest.Wait()+"\n"), 0o755)
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	repo := gitRepo(t)
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	const id = "task-1"
	loop := newLoopWith(s, state, t.TempDir(), turns.Config{})
	run(t, loop, state)
	dog := watchdogs.New(s, state, watchdogs.DefaultLimits, loop.runner.IsRunning, loop.runner.Stop, func(string, string) error { return nil })
	upIn := func(turn string) { // as the start grace makes sure
		t.Helper()
		for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(20 * time.Millisecond) {
			if _, err := os.Stat(filepath.Join(up, turn)); err == nil {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("%s's session didn't start", turn)
			}
		}
	}
	look := func(turn string) { // once the turn's session has come and gone
		t.Helper()
		upIn(turn)
		for deadline := time.Now().Add(10 * time.Second); loop.runner.IsRunning(id); time.Sleep(20 * time.Millisecond) {
			if time.Now().After(deadline) {
				t.Fatalf("%s's session didn't exit", turn)
			}
		}
		dog.Check(time.Now().Add(time.Minute))
	}
	propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Remove the TODOs", "repo": repo, "coordinator": "tech-lead"}})

	waitForTurn(t, s, id, "turn-1", "tech-lead")
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Remove every TODO."}`)
	waitForTurn(t, s, id, "turn-2", "engineer")
	propose(t, s, events.Proposal{Stream: id, Actor: worker, Type: events.SessionStarted, Correlation: "turn-2",
		Data: map[string]any{"engineSession": "s-1"}})
	endTurn(t, s, id, "turn-2", "engineer", "Removed 3.")
	waitForTurn(t, s, id, "turn-3", "tech-lead")
	const instructions = "Remove the one in src/b.go."
	endTurn(t, s, id, "turn-3", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "`+instructions+`"}`)
	if task := waitForTurn(t, s, id, "turn-4", "engineer"); task.Turn("turn-4").FromSession != "s-1" {
		t.Fatalf("turn-4 should go on with s-1: %+v", task.Turn("turn-4"))
	}

	look("turn-4")
	task := waitForTurn(t, s, id, "turn-5", "engineer")
	if issue := task.Turn("turn-4").Issue; !strings.Contains(issue, "resumed session didn't start") {
		t.Errorf("turn-4's issue: %q", issue)
	}
	if retry := task.Turn("turn-5"); retry.FromSession != "" || retry.FreshBecause != "its resume failed" {
		t.Errorf("the retry is fresh because its resume failed: from %q, fresh because %q", retry.FromSession, retry.FreshBecause)
	}
	if retry, failed := task.Turn("turn-5"), task.Turn("turn-4"); retry.Worker != failed.Worker {
		t.Errorf("the retry runs on turn-4's machine %q, not %q", failed.Worker, retry.Worker)
	}
	brief := readBrief(t, task, "turn-5")
	if !strings.Contains(brief, "## The task") || !strings.Contains(brief, "## Your instructions from the coordinator\n\n"+instructions) {
		t.Errorf("the retry has the whole brief, with turn-4's instructions:\n%s", brief)
	}

	look("turn-5")
	task = waitForTurn(t, s, id, "turn-6", "tech-lead")
	if issue := task.Turn("turn-5").Issue; !strings.Contains(issue, "process exited in turn-5") {
		t.Errorf("the fresh retry's failure is an ordinary issue: %q", issue)
	}
}
