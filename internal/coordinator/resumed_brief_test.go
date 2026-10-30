package coordinator

import (
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/turns"
)

// fixRound is a task in its fix round: the engineer worked, the reviewer
// asked for a change, and the coordinator hands it back to the engineer.
// The engineer's denied push happened in its first turn.
func fixRound() history {
	return history{}.
		add(you, events.TaskCreated, "", map[string]any{"title": "Remove the TODOs", "description": "Every TODO in src/ goes.",
			"repo": "/r", "coordinator": "tech-lead"}).
		add(worker, events.WorktreeReady, "", map[string]any{"path": "/w/task-1", "branch": "agentry/task-1", "port": 7901}).
		turn("turn-1", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Remove them."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove them."}).
		add(worker, events.TurnStarted, "turn-2", map[string]any{"agent": "engineer", "hash": "h"}).
		add(worker, events.ActionRequested, "push", map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"}).
		add(you, events.ActionDenied, "push", map[string]any{"reason": "agents don't push"}).
		add(worker, events.TurnEnded, "turn-2", map[string]any{"agent": "engineer", "report": "Removed 3."}).
		turn("turn-3", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Review it."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "reviewer", "instructions": "Review it."}).
		turn("turn-4", "reviewer", "One is left in src/b.go.\n\nVerdict: changes requested").
		turn("turn-5", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Remove the one in src/b.go."}`)
}

func agentNamed(t *testing.T, roster agents.Roster, name string) agents.Agent {
	t.Helper()
	agent, ok := roster.Get(name)
	if !ok {
		t.Fatalf("no agent %s", name)
	}
	return agent
}

func TestAResumedWorkerBriefIsWhatHappenedSinceAndItsInstructions(t *testing.T) {
	roster := defaultRoster()
	const instructions = "Remove the one in src/b.go."
	task := fixRound().add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": instructions}).task()
	engineer := agentNamed(t, roster, "engineer")

	brief := resumedWorkerBrief(task, engineer, roster, instructions, task.LastTurnOf("engineer").Seen, "", nil)
	for _, want := range []string{"# Agentry brief: engineer, task-1, turn-6, going on", "goes on from your last turn",
		"## What has happened since your last turn", "One is left in src/b.go.", "3. engineer → tech-lead: Removed 3.",
		"## Your instructions from the coordinator\n\n" + instructions, "worktree, /w/task-1, on branch agentry/task-1",
		"## Shared files", ".agentry/turn-6-plan.md", "update your notes in .agentry/knowledge.md", "## When you finish"} {
		if !strings.Contains(brief, want) {
			t.Errorf("the resumed brief is missing %q:\n%s", want, brief)
		}
	}
	for _, left := range []string{"## The task", "Every TODO in src/ goes.", "you → tech-lead", "Remove them.",
		engineer.Prompt, "## What you know from earlier tasks"} {
		if strings.Contains(brief, left) {
			t.Errorf("the resumed brief should leave out %q:\n%s", left, brief)
		}
	}
	if n := strings.Count(brief, instructions); n != 1 {
		t.Errorf("the new instructions are in the brief %d times, want once, not again on the trail:\n%s", n, brief)
	}
}

func TestAResumedReviewerBriefStillEndsWithTheVerdict(t *testing.T) {
	roster := defaultRoster()
	task := fixRound().
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove the one in src/b.go."}).
		turn("turn-6", "engineer", "Removed it.").
		turn("turn-7", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Review the fix."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "reviewer", "instructions": "Review the fix."}).task()

	brief := resumedWorkerBrief(task, agentNamed(t, roster, "reviewer"), roster, "Review the fix.", task.LastTurnOf("reviewer").Seen, "", nil)
	if !strings.HasSuffix(strings.TrimSpace(brief), strings.TrimSpace(VerdictFormat)) {
		t.Errorf("a reviewer's resumed brief ends with the verdict format:\n%s", brief)
	}
	if !strings.Contains(brief, "engineer → tech-lead: Removed it.") || strings.Contains(brief, "Removed 3.") {
		t.Errorf("the reviewer hears only what happened since its last turn:\n%s", brief)
	}
}

func TestAResumedCoordinatorBriefKeepsTheDecisionChecksAndRefusals(t *testing.T) {
	roster := defaultRoster()
	task := fixRound().
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove the one in src/b.go."}).
		turn("turn-6", "engineer", "Removed it.").task()
	lead := agentNamed(t, roster, "tech-lead")

	brief := resumedCoordinatorBrief(task, lead, roster, task.LastTurnOf("tech-lead").Seen, "", family{}, twoMachines, nil, nil, nil)
	for _, want := range []string{"# Agentry brief: tech-lead, task-1, turn-7, going on", "engineer → tech-lead: Removed it.",
		"6. tech-lead → engineer: Remove the one in src/b.go.", "## Machines\n\n- local (host here", "- tower (host tower, linux), offline\n",
		"## Actions Agentry refused", "git push origin HEAD:dev`: agents don't push", "## Before you finish",
		"## Your decision\n\n" + strings.TrimSpace(DecisionFormat), "worktree, /w/task-1", ".agentry/turn-7-plan.md",
		"Decide what happens next."} {
		if !strings.Contains(brief, want) {
			t.Errorf("the resumed coordinator brief is missing %q:\n%s", want, brief)
		}
	}
	for _, left := range []string{"## The task", "## The agents you can hand work to", "## Other teams",
		"One is left in src/b.go.", lead.Prompt, "## If this task is too big"} {
		if strings.Contains(brief, left) {
			t.Errorf("the resumed coordinator brief should leave out %q:\n%s", left, brief)
		}
	}
}

// twoMachines is this machine and a worker gone offline, so a coordinator
// has machines to choose from.
var twoMachines = []machine{{name: "local", host: "here", os: "this machine, where Agentry runs", online: true},
	{name: "tower", host: "tower", os: "linux"}}

// A story started from the Backlog leaves no step on the trail, so a
// resumed coordinator hears of the epic's stories as they are now.
func TestAResumedCoordinatorBriefHasTheEpicsStoriesNow(t *testing.T) {
	roster := defaultRoster()
	task := fixRound().turn("turn-6", "engineer", "Removed it.").task()
	task.Epic = "epic-1"
	epic := &tasks.Task{ID: "epic-1", Title: "Tidy the code", Description: "No TODOs anywhere.", Coordinator: "tech-lead"}
	later := &tasks.Task{ID: "task-2", Title: "Lint for TODOs", After: []string{"task-1"}}

	brief := resumedCoordinatorBrief(task, agentNamed(t, roster, "tech-lead"), roster, task.LastTurnOf("tech-lead").Seen, "",
		family{epic: epic, stories: []*tasks.Task{task, later}}, nil, nil, nil, nil)
	for _, want := range []string{"## This task is a story of an epic", "The epic, epic-1: Tidy the code",
		"Later stories build on this one: task-2 (Lint for TODOs)"} {
		if !strings.Contains(brief, want) {
			t.Errorf("the resumed coordinator brief is missing %q:\n%s", want, brief)
		}
	}

	epic.Kind, epic.Worktree = tasks.Epic, task.Worktree
	brief = resumedCoordinatorBrief(epic, agentNamed(t, roster, "tech-lead"), roster, 0, "",
		family{stories: []*tasks.Task{task, later}}, nil, nil, nil, nil)
	if !strings.Contains(brief, "## This task is an epic") || !strings.Contains(brief, "- task-2 (") {
		t.Errorf("the resumed brief of an epic has its stories now:\n%s", brief)
	}
}

// How to split a task too big is in the coordinator's first brief, while
// it hasn't chosen the task's kind; a resumed brief doesn't say it again.
func TestAResumedCoordinatorBriefDoesNotOfferTheSplitAgain(t *testing.T) {
	roster := defaultRoster()
	task := fixRound().turn("turn-6", "engineer", "Removed it.").task()
	lead := agentNamed(t, roster, "tech-lead")
	const split = "## If this task is too big"
	if whole := coordinatorBrief(task, lead, roster, family{}, nil, nil, nil, nil); !strings.Contains(whole, split) {
		t.Fatalf("the whole brief of a task whose kind isn't chosen offers the split:\n%s", whole)
	}
	if resumed := resumedCoordinatorBrief(task, lead, roster, task.LastTurnOf("tech-lead").Seen, "", family{}, nil, nil, nil, nil); strings.Contains(resumed, split) {
		t.Errorf("the resumed brief offers the split again:\n%s", resumed)
	}
}

// A resumed brief is much shorter than the whole one for the same turn.
func TestAResumedBriefIsShorterThanTheWholeOne(t *testing.T) {
	roster := defaultRoster()
	const instructions = "Remove the one in src/b.go."
	task := fixRound().add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": instructions}).task()
	engineer := agentNamed(t, roster, "engineer")
	whole := workerBrief(task, engineer, roster, instructions, nil)
	resumed := resumedWorkerBrief(task, engineer, roster, instructions, task.LastTurnOf("engineer").Seen, "", nil)
	t.Logf("engineer's brief for turn-6: whole %d bytes, resumed %d bytes", len(whole), len(resumed))
	if len(resumed) >= len(whole) {
		t.Errorf("resumed %d bytes, whole %d", len(resumed), len(whole))
	}
}

// The loop asks the runner how the turn starts before it makes the brief:
// a turn that goes on with the agent's session gets the resumed brief, and
// one that starts fresh the whole one.
func TestTheLoopBriefsATurnAsItStarts(t *testing.T) {
	for _, c := range []struct {
		name    string
		config  turns.Config
		resumed bool
	}{
		{"resume on", turns.Config{}, true},
		{"resume off", turns.Config{NoResume: true}, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			fakeClaudeOnPath(t)
			repo := gitRepo(t)
			s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
			for _, p := range agents.SeedProposals(nil) {
				s.Propose(p)
			}
			const id = "task-1"
			run(t, newLoopWith(s, state, t.TempDir(), c.config), state)
			propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
				Data: map[string]any{"title": "Remove the TODOs", "repo": repo, "coordinator": "tech-lead"}})

			waitForTurn(t, s, id, "turn-1", "tech-lead")
			propose(t, s, events.Proposal{Stream: id, Actor: worker, Type: events.SessionStarted, Correlation: "turn-1",
				Data: map[string]any{"engineSession": "s-1"}})
			endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Remove every TODO."}`)
			waitForTurn(t, s, id, "turn-2", "engineer")
			endTurn(t, s, id, "turn-2", "engineer", "Removed 3.")
			task := waitForTurn(t, s, id, "turn-3", "tech-lead")

			brief := readBrief(t, task, "turn-3")
			if resumed := strings.Contains(brief, ", going on\n"); resumed != c.resumed {
				t.Errorf("turn-3's brief is the resumed one: %v, want %v:\n%s", resumed, c.resumed, brief)
			}
			if whole := strings.Contains(brief, "## The task"); whole == c.resumed {
				t.Errorf("turn-3's brief has the task: %v, want %v:\n%s", whole, !c.resumed, brief)
			}
			if from := task.Turn("turn-3").FromSession; (from == "s-1") != c.resumed {
				t.Errorf("turn-3 went on with %q; resumed %v", from, c.resumed)
			}
			if !strings.Contains(brief, "engineer → tech-lead: Removed 3.") {
				t.Errorf("either brief has the engineer's report:\n%s", brief)
			}
		})
	}
}
