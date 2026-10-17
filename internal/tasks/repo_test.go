package tasks

import (
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

func TestATaskMovesRepoBetweenTurns(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	at := time.Now()
	log := []events.Event{
		{At: at, Proposal: events.Proposal{Stream: "task-7", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "t", "repo": "/src/agentry", "coordinator": "tech-lead"}}},
		{At: at, Proposal: events.Proposal{Stream: "task-7", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/task-7", "branch": "agentry/task-7", "port": 7907}}},
		{At: at, Proposal: events.Proposal{Stream: "task-7", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "reviewer", "hash": "h"}}},
		{At: at, Proposal: events.Proposal{Stream: "task-7", Actor: you, Type: events.RepoChanged, Data: map[string]any{"repo": "/src/design"}}},
	}
	task := FromEvents(log)
	if task.Repo != "/src/agentry" || task.Worktree == nil || task.MovingTo != "/src/design" {
		t.Errorf("while the turn runs, the work stays where it is: repo %q, moving to %q", task.Repo, task.MovingTo)
	}
	if last := task.Trail[len(task.Trail)-1]; !strings.Contains(last.Text, "/src/design (it moves when the turn in progress ends)") {
		t.Errorf("the coordinator is told: %q", last.Text)
	}

	log = append(log, events.Event{At: at, Proposal: events.Proposal{Stream: "task-7", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "reviewer", "report": "r"}}})
	if task = FromEvents(log); task.Repo != "/src/design" || task.Worktree != nil || task.MovingTo != "" {
		t.Errorf("when the turn ends, the task moves and its worktree is made again: repo %q, worktree %+v", task.Repo, task.Worktree)
	}

	// Between turns it moves at once.
	log = append(log, events.Event{At: at, Proposal: events.Proposal{Stream: "task-7", Actor: you, Type: events.RepoChanged, Data: map[string]any{"repo": "/src/other"}}})
	if task = FromEvents(log); task.Repo != "/src/other" || !strings.Contains(task.Trail[len(task.Trail)-1].Text, "repo at /src/other. ") {
		t.Errorf("between turns: repo %q, told %q", task.Repo, task.Trail[len(task.Trail)-1].Text)
	}
}
