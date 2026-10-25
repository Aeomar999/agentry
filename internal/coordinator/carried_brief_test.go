package coordinator

import (
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// newStory is task-3, a story of epic-1 just handed to the engineer: the
// first turn on it of an engineer whose session is on task-1 (fixRound).
func newStory() history {
	return history{}.
		add(you, events.TaskCreated, "", map[string]any{"title": "Lint for TODOs", "description": "CI fails on a new TODO.",
			"repo": "/r", "coordinator": "tech-lead"}).
		add(worker, events.WorktreeReady, "", map[string]any{"path": "/w/task-3", "branch": "agentry/task-3", "port": 7903}).
		turn("turn-1", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Add the lint."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Add the lint."})
}

// What the sibling's session knows that this task's brief must not carry.
var siblingsTrail = []string{"Remove the TODOs", "Every TODO in src/ goes.", "Removed 3.", "One is left in src/b.go.", "/w/task-1", "agentry/task-1"}

func TestABriefFromASiblingsSessionSaysItIsANewTask(t *testing.T) {
	roster := defaultRoster()
	task := newStory().task()
	task.ID = "task-3"
	engineer := agentNamed(t, roster, "engineer")

	brief := resumedWorkerBrief(task, engineer, roster, "Add the lint.", 0, "task-1", nil)
	for _, want := range []string{"# Agentry brief: engineer, task-3, turn-2, going on from task-1", "your session from task-1",
		"this is a new task, task-3", "may differ on this branch, so read them again before you edit them",
		"## The task\n\nTitle: Lint for TODOs\n\nCI fails on a new TODO.",
		"## What has happened on this task so far", "1. you → tech-lead: ",
		"## Your instructions from the coordinator\n\nAdd the lint.", "worktree, /w/task-3, on branch agentry/task-3",
		".agentry/turn-2-plan.md", "update your notes in .agentry/knowledge.md", "## When you finish"} {
		if !strings.Contains(brief, want) {
			t.Errorf("the brief from task-1's session is missing %q:\n%s", want, brief)
		}
	}
	for _, left := range append(siblingsTrail, "goes on from your last turn on this task", "## What has happened since your last turn", engineer.Prompt) {
		if strings.Contains(brief, left) {
			t.Errorf("the brief from task-1's session should leave out %q:\n%s", left, brief)
		}
	}
	if n := strings.Count(brief, "Add the lint."); n != 1 {
		t.Errorf("the instructions are in the brief %d times, want once:\n%s", n, brief)
	}
}

func TestACoordinatorBriefFromASiblingsSessionKeepsTheLiveState(t *testing.T) {
	roster := defaultRoster()
	task := newStory().task()
	task.ID, task.Epic = "task-3", "epic-1"
	epic := &tasks.Task{ID: "epic-1", Title: "Tidy the code", Description: "No TODOs anywhere.", Coordinator: "tech-lead"}

	brief := resumedCoordinatorBrief(task, agentNamed(t, roster, "tech-lead"), roster, 0, "task-1",
		family{epic: epic, stories: []*tasks.Task{task}}, twoMachines, nil, nil, nil)
	for _, want := range []string{"# Agentry brief: tech-lead, task-3, turn-2, going on from task-1", "this is a new task, task-3",
		"## The task\n\nTitle: Lint for TODOs", "## What has happened on this task so far", "1. you → tech-lead: ",
		"tech-lead → engineer: Add the lint.", "worktree, /w/task-3, on branch agentry/task-3", "## Machines",
		"## This task is a story of an epic", "## Your decision"} {
		if !strings.Contains(brief, want) {
			t.Errorf("the coordinator brief from task-1's session is missing %q:\n%s", want, brief)
		}
	}
	for _, left := range append(siblingsTrail, "goes on from your last turn on this task") {
		if strings.Contains(brief, left) {
			t.Errorf("the coordinator brief from task-1's session should leave out %q:\n%s", left, brief)
		}
	}
}

// A session of the task itself gets the brief story 2 made, word for word.
func TestASameTaskResumedBriefIsUnchanged(t *testing.T) {
	roster := defaultRoster()
	task := fixRound().add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove the one in src/b.go."}).task()
	engineer := agentNamed(t, roster, "engineer")
	seen := task.LastTurnOf("engineer").Seen
	if own, unnamed := resumedWorkerBrief(task, engineer, roster, "Remove the one in src/b.go.", seen, task.ID, nil),
		resumedWorkerBrief(task, engineer, roster, "Remove the one in src/b.go.", seen, "", nil); own != unnamed {
		t.Errorf("from the task itself:\n%s\nwant as with no story named:\n%s", own, unnamed)
	}
}
