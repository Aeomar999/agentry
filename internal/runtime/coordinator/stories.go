package coordinator

import (
	"errors"
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// storyDone gives a finished story's summary to its epic, if the epic
// hasn't heard it yet. A closed epic hears nothing more; reopened, it
// hears its finished stories then.
func (l *Loop) storyDone(story *tasks.Task) {
	epic, _, err := l.load(story.Epic)
	if err != nil || epic == nil || epic.IsDone || !slices.Contains(epic.Stories, story.ID) || slices.Contains(epic.StoriesDone, story.ID) {
		return
	}
	l.propose(epic.ID, agentry, events.StoryDone, map[string]any{"story": story.ID, "summary": story.Summary,
		"coordinator": story.Coordinator})
}

// wakeStories hears from the epic's finished stories and starts any that
// can start now.
func (l *Loop) wakeStories(epic *tasks.Task) {
	for _, id := range openStories(epic) {
		story, _, err := l.load(id)
		switch {
		case err != nil || story == nil:
		case story.IsDone:
			l.storyDone(story)
		case story.Worktree == nil:
			l.act(id)
		}
	}
}

// blocked is a story whose stories before it aren't done: its worktree
// would start from a dev without their work. An archived story is done,
// and an erased one blocks nothing: nothing of it is left to wait for.
func (l *Loop) blocked(story *tasks.Task) bool {
	for _, id := range story.After {
		before, _, err := l.load(id)
		if errors.Is(err, current.ErrArchived) || errors.Is(err, current.ErrErased) {
			continue
		}
		if err != nil || before == nil || !before.IsDone {
			return true
		}
	}
	return false
}

// answer gives a finished task's summary to the task that asked for it, if
// that one still waits for it. An archived asker never does, so it is not
// unarchived for the answer: only a done task is archived, and a done task
// waits on none. Moved to Done, it stops waiting; its coordinator finishes
// it only from a turn, which a waiting task doesn't get (Next); and the
// rules refuse task.request_answered to a done task.
func (l *Loop) answer(child *tasks.Task) {
	parent, _, err := l.load(child.Parent)
	if err != nil || parent == nil || parent.WaitingOn != child.ID {
		return
	}
	l.propose(parent.ID, agentry, events.RequestAnswered, map[string]any{"child": child.ID, "summary": child.Summary,
		"coordinator": child.Coordinator})
}
