package tasks

import (
	"cmp"
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// FromEvents builds a task by applying its accepted events in order.
// It returns nil if the events don't include task.created, and panics with
// Replay's error for a stream it can't replay. Tests and callers that want
// a bad stream to fail loudly use it; code that must survive one, such as
// startup, uses Replay.
func FromEvents(stream []events.Event) *Task {
	t, err := Replay(stream)
	if err != nil {
		panic(err)
	}
	return t
}

// Replay is FromEvents with the error for a stream whose apply panics: the
// stream, position and event it stopped at. Apply is meant to replay every
// event the rules ever accepted, so the error is a bug, but one task's
// stream must not take down the reads of every other.
//
// An archived task folds to nil: at its task.archived the task so far is
// set aside, and Archived gives its summary. A task.unarchived takes back
// the task set aside, rebuilt from the restored rows before the marker in
// the same stream (docs/design/log-growth.md, "Reopening an archived
// task"); with none set aside the stream can't be replayed. An erased task
// folds to nil too, whatever its events before the erasure, and Erased
// gives the little it keeps.
func Replay(stream []events.Event) (t *Task, err error) {
	if _, erased := Erased(stream); erased {
		return nil, nil
	}
	var e events.Event
	defer func() {
		if recovered := recover(); recovered != nil {
			t, err = nil, fmt.Errorf("applying %s to %s at %d: %v", e.Type, e.Stream, e.Position, recovered)
		}
	}()
	var aside *Task
	for _, e = range stream {
		switch e.Type {
		case events.TaskCreated:
			t = &Task{ID: e.Stream, CreatedAt: e.At}
		case events.TaskArchived:
			t, aside = nil, t
			continue
		case events.TaskUnarchived:
			if aside == nil {
				panic("no task before its marker to take back")
			}
			t, aside = aside, nil
		}
		if t != nil {
			t.Apply(e)
		}
	}
	return t, nil
}

// Apply changes the task in place by one accepted event of its stream. A
// state loaded from a snapshot takes the events after it this way. A
// redaction changes nothing, not even when the task was last active: what
// it changed is in the rows before it, which the task is folded from again.
func (t *Task) Apply(e events.Event) {
	if e.Type == events.EventRedacted {
		return
	}
	t.LastActiveAt = e.At
	if turn := t.Turn(e.Correlation); turn != nil {
		turn.LastEventAt = e.At
	}
	if action := t.Action(e.Correlation); action != nil {
		if turn := t.Turn(action.Turn); turn != nil {
			turn.LastEventAt = e.At
		}
	}

	t.applyFiles(e)
	if t.orphaned(e) {
		return
	}
	switch e.Type {
	case events.TaskUnarchived, events.TaskCreated, events.TaskDirected, events.TaskBudgeted,
		events.TaskTurnsLimited, events.AgentRenamedInTask, events.CoordinatorMoved, events.TaskRanked,
		events.TaskLabeled, events.TaskRenamed:
		t.applyTask(e)
	case events.TaskHeld, events.TaskWrappedUp, events.TaskReopened:
		t.applyBoard(e)
	case events.RepoChanged, events.WorktreeReady, events.WorktreeMoved:
		t.applyWorktree(e)
	case events.TaskTransferred, events.TaskRequested, events.TaskSplit, events.EpicWaits, events.StoryDone,
		events.EpicJoined, events.EpicLeft, events.StoryAdded, events.StoryRemoved, events.FollowUpsSkipped,
		events.RequestAnswered:
		t.applyEpics(e)
	case events.TaskAssigned, events.TaskNeedsYou, events.TaskAnswered, events.TaskDone:
		t.applyHandoffs(e)
	case events.TurnStarted, events.PromptAcknowledged, events.SessionStarted, events.PlanUpdated,
		events.AgentWaiting, events.TurnEnded, events.TurnLimited, events.TurnResumed, events.IssueRaised,
		events.TurnStopped:
		t.applyTurns(e)
	case events.ActionRequested, events.ActionAsked, events.ActionApproved, events.ActionDenied,
		events.ActionHeld, events.ActionResult:
		t.applyActions(e)
	}
}

// orphaned is an event for a turn that isn't open or an action the task
// doesn't have. The rules refuse one now, but a log written under earlier
// rules can hold it (task.held didn't stop the turn then, so its actions
// went on), and replaying the log must not fail on it: it changes nothing.
func (t *Task) orphaned(e events.Event) bool {
	switch e.Type {
	case events.ActionRequested:
		return t.OpenTurn() == nil
	case events.ActionAsked, events.ActionApproved, events.ActionDenied, events.ActionHeld, events.ActionResult:
		return t.Action(e.Correlation) == nil
	case events.PromptAcknowledged, events.SessionStarted, events.PlanUpdated, events.AgentWaiting, events.TurnEnded, events.TurnLimited, events.TurnResumed, events.TurnStopped:
		return t.Turn(e.Correlation) == nil
	}
	return false
}

// idle is a task with nothing in hand: no turn, no decision to apply and
// no one about to start.
func (t *Task) idle() bool {
	return t.OpenTurn() == nil && t.PendingDecision == "" && t.Handoff == nil
}

// step adds a line to the trail. A step to an agent is also the task's
// pending handoff, and any step settles a pending decision.
func (t *Task) step(e events.Event, from, to, text string) {
	step := Step{At: e.At, From: from, To: to, Text: text}
	t.Trail = append(t.Trail, step)
	t.PendingDecision, t.Handoff = "", nil
	if to != You {
		t.Handoff = &step
	}
}

// reply adds the coordinator's answer to what you told it, which any
// decision may carry, to the trail before the decision itself.
func (t *Task) reply(e events.Event) {
	if text := e.Text("toYou"); text != "" {
		t.Told = Step{At: e.At, From: e.Actor.ID, To: You, Text: text}
		t.Trail = append(t.Trail, t.Told)
	}
}

// MadeFromOf is the stream a task.created says made the task, "" for none.
// Older logs and binaries name it only as "idea", so that stays the fallback.
func MadeFromOf(p events.Proposal) string {
	return cmp.Or(p.Text("from"), p.Text("idea"))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
