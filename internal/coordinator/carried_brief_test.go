package coordinator

import (
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
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

// sibling is task-1, the story of epic-1 whose session the engineer brings
// to the new story: fixRound, with its own trail and worktree.
func sibling() *tasks.Task {
	task := fixRound().task()
	task.ID, task.Epic = "task-1", "epic-1"
	return task
}

// siblingsWords is what the sibling's session knows that this task's brief
// must not carry: its title, description, worktree and every step of its
// trail, instructions and reports alike.
func siblingsWords(t *testing.T, sibling *tasks.Task) []string {
	t.Helper()
	words := []string{sibling.Title, sibling.Description, sibling.Worktree.Path, sibling.Worktree.Branch}
	for _, step := range sibling.Trail {
		words = append(words, step.Text)
	}
	if len(sibling.Trail) < 4 || slices.Contains(words, "") {
		t.Fatalf("the sibling has too little of its own to tell its words apart: %q", words)
	}
	return words
}

func TestABriefFromASiblingsSessionSaysItIsANewTask(t *testing.T) {
	roster := defaultRoster()
	sibling := sibling()
	task := newStory().task()
	task.ID, task.Epic = "task-3", sibling.Epic
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
	for _, left := range append(siblingsWords(t, sibling), "goes on from your last turn on this task", "## What has happened since your last turn", engineer.Prompt) {
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
	sibling := sibling()
	task := newStory().task()
	task.ID, task.Epic = "task-3", sibling.Epic
	epic := &tasks.Task{ID: sibling.Epic, Title: "Tidy the code", Description: "No TODOs anywhere.", Coordinator: "tech-lead"}

	brief := resumedCoordinatorBrief(task, agentNamed(t, roster, "tech-lead"), roster, 0, sibling.ID,
		family{epic: epic, stories: []*tasks.Task{sibling, task}}, twoMachines, nil, nil, nil)
	for _, want := range []string{"# Agentry brief: tech-lead, task-3, turn-2, going on from task-1", "this is a new task, task-3",
		"## The task\n\nTitle: Lint for TODOs", "## What has happened on this task so far", "1. you → tech-lead: ",
		"tech-lead → engineer: Add the lint.", "worktree, /w/task-3, on branch agentry/task-3", "## Machines",
		"## This task is a story of an epic", "## Your decision"} {
		if !strings.Contains(brief, want) {
			t.Errorf("the coordinator brief from task-1's session is missing %q:\n%s", want, brief)
		}
	}
	for _, left := range append(siblingsWords(t, sibling), "goes on from your last turn on this task") {
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
