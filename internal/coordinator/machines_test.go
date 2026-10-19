package coordinator

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// The coordinator chooses where an agent works: a machine it may run on,
// by name. One it may not use, one that isn't there, or one that is
// offline comes back to the coordinator, with why, to choose again.
func TestTheCoordinatorChoosesTheMachine(t *testing.T) {
	fakeClaudeOnPath(t)
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	code, invited := workers.Invite(you, time.Now())
	_, joined := workers.Join("laptop", code, workers.Machine{Host: "laptop.lan", OS: "darwin", Arch: "arm64"})
	s.Propose(invited)
	s.Propose(joined)
	tester, _ := agents.FromEvents(mustStream(t, s, events.AgentsStream)).Get("tester")
	tester.Workers = []string{"laptop", "local"}
	if _, rejection, _ := s.Propose(tester.Proposal(you, true)); rejection != nil {
		t.Fatal(rejection)
	}
	const id = "task-machine-1"
	defer sessions.Stop(sessions.SocketFor(id))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go newLoop(s, state, t.TempDir()).Run(ctx)

	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Fix it", "repo": gitRepo(t), "coordinator": "tech-lead"}})
	task := waitForTurn(t, s, id, "turn-1", "tech-lead")
	brief := readBrief(t, task, "turn-1")
	for _, want := range []string{"## Machines", "- laptop (host laptop.lan, darwin/arm64), offline; may run tester", "The task's work is on local now"} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief is missing %q:\n%s", want, brief)
		}
	}

	refusals := []struct{ decision, why string }{
		{`{"action": "assign", "agent": "engineer", "instructions": "Go.", "machine": "laptop"}`, "engineer may not run on laptop; it runs on local"},
		{`{"action": "assign", "agent": "engineer", "instructions": "Go.", "machine": "build-box"}`, "there is no machine called build-box; the machines are local, laptop"},
		{`{"action": "assign", "agent": "tester", "instructions": "Go.", "machine": "laptop.lan"}`, "the turn couldn't start where you chose: laptop is offline"},
	}
	for i, r := range refusals {
		turn, next := fmt.Sprintf("turn-%d", i+1), fmt.Sprintf("turn-%d", i+2)
		endTurn(t, s, id, turn, "tech-lead", r.decision)
		task = waitForTurn(t, s, id, next, "tech-lead")
		if brief := readBrief(t, task, next); !strings.Contains(brief, r.why) {
			t.Errorf("after %s the tech lead should hear %q:\n%s", r.decision, r.why, brief)
		}
	}

	// A machine it may use: the turn runs there, named by the host's name too.
	endTurn(t, s, id, "turn-4", "tech-lead", `{"action": "assign", "agent": "tester", "instructions": "Test it.", "machine": "LOCAL"}`)
	task = waitForTurn(t, s, id, "turn-5", "tester")
	if open := task.OpenTurn(); open.Worker != "local" || task.Trail[len(task.Trail)-1].Machine != "local" {
		t.Errorf("the tester should run on local: %+v", open)
	}
}
