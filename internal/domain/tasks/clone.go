package tasks

import (
	"maps"
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// Next is the task after e, built on a copy: t itself is left as it was, so
// whoever still holds it keeps reading what it said. FromEvents applies in
// place instead, and stays the reference Next must agree with.
func Next(t *Task, e events.Event) *Task {
	if e.Type == events.TaskCreated {
		t = &Task{ID: e.Stream, CreatedAt: e.At}
	} else if t == nil {
		return nil
	} else {
		t = t.Clone()
	}
	t.Apply(e)
	return t
}

// Clone is a deep copy: everything Apply may write through, every slice it
// may append to and every turn and action it changes in place, is the
// copy's own. Turn, Action, Step, SharedFile, Fetch, Worktree and WrapUp
// hold only values, so copying the struct copies them whole.
func (t *Task) Clone() *Task {
	c := *t
	c.Labels = slices.Clone(t.Labels)
	c.After = slices.Clone(t.After)
	c.Stories = slices.Clone(t.Stories)
	c.StoriesDone = slices.Clone(t.StoriesDone)
	c.Trail = slices.Clone(t.Trail)
	c.Files = maps.Clone(t.Files)
	c.collected = maps.Clone(t.collected)
	c.Worktree = clonePointer(t.Worktree)
	c.Fetching = clonePointer(t.Fetching)
	c.Handoff = clonePointer(t.Handoff)
	c.WrapUp = clonePointer(t.WrapUp)
	c.Asked.Options = slices.Clone(t.Asked.Options)
	c.Asked.Fetch = clonePointer(t.Asked.Fetch)
	c.Turns = cloneEach(t.Turns)
	c.Actions = cloneEach(t.Actions)
	c.actionAt = maps.Clone(t.actionAt) // positions, so they hold in the copy's Actions too
	return &c
}

func clonePointer[T any](p *T) *T {
	if p == nil {
		return nil
	}
	c := *p
	return &c
}

func cloneEach[T any](list []*T) []*T {
	if list == nil {
		return nil
	}
	copies := make([]*T, len(list))
	for i, p := range list {
		copies[i] = clonePointer(p)
	}
	return copies
}
