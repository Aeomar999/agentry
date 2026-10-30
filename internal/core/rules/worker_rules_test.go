package rules

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

func TestWorkerRules(t *testing.T) {
	s := openSeeded(t)
	propose := func(p events.Proposal) *events.Rejection {
		t.Helper()
		_, rejection, err := s.Propose(p)
		if err != nil {
			t.Fatal(err)
		}
		return rejection
	}
	machine := workers.Machine{Host: "laptop", OS: "linux", Arch: "amd64"}
	code, invited := workers.Invite(you, time.Now())
	propose(invited)

	if _, p := workers.Join("laptop", "a-code-never-made", machine); propose(p) == nil {
		t.Error("a machine joined with a code nobody made")
	}
	if _, p := workers.Join(workers.Local, code, machine); propose(p) == nil {
		t.Error("a worker took the name of the machine Agentry runs on")
	}
	if _, p := workers.Join("laptop", code, machine); propose(p) != nil {
		t.Fatal("a machine couldn't join with a fresh code")
	}
	if _, p := workers.Join("desktop", code, machine); propose(p) == nil {
		t.Error("a second machine joined with a used code")
	}

	engineer := agents.Agent{Name: "builder", Prompt: "Build it.", Engine: "claude", Workers: []string{"laptop", workers.Local}}
	if rejection := propose(engineer.Proposal(you, false)); rejection != nil {
		t.Errorf("an agent on the worker and this machine was refused: %v", rejection)
	}
	stray := agents.Agent{Name: "stray", Prompt: "x", Engine: "claude", Workers: []string{"nowhere"}}
	if propose(stray.Proposal(you, false)) == nil {
		t.Error("an agent was given a worker that never joined")
	}
	if propose(workers.Removal(you, "laptop")) == nil {
		t.Error("a worker was removed while an agent runs on it")
	}
	engineer.Workers = nil
	propose(engineer.Proposal(you, true))
	if rejection := propose(workers.Removal(you, "laptop")); rejection != nil {
		t.Errorf("removing a worker no agent runs on: %v", rejection)
	}
}

func TestWorkMovesOnlyToAnotherMachine(t *testing.T) {
	move := func(to string) map[string]any { return map[string]any{"worker": to, "path": "/w/task-1", "port": 7901} }
	run(t, []step{
		{you, events.TaskCreated, "", map[string]any{"title": "t", "repo": "/r", "coordinator": "tech-lead"}, true},
		{worker, events.WorktreeMoved, "", move("laptop"), false}, // no worktree yet
		{worker, events.WorktreeReady, "", map[string]any{"worker": "local", "path": "/r/w", "branch": "b", "port": 7900}, true},
		{worker, events.WorktreeMoved, "", move("local"), false}, // it is there already
		{worker, events.WorktreeMoved, "", move("laptop"), true},
		{worker, events.TurnStarted, "turn-1", map[string]any{"agent": "tech-lead", "hash": "h"}, true},
		{worker, events.WorktreeMoved, "", move("local"), false}, // not while a turn runs
	})
}
