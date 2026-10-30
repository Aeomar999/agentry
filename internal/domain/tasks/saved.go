package tasks

import "maps"

// Saved is a task in a form that can be written out, as JSON, and read back
// whole: the task, and beside it what Task keeps unexported, which JSON
// would leave out.
type Saved struct {
	Task         Task // its unexported fields left zero: they are the ones below
	WrappedUp    int
	Collected    map[string]bool
	StoryNews    bool
	ApprovalNews bool
}

// Save is the task's saved form. It is made from a Clone, so it shares
// nothing with the task.
func (t *Task) Save() Saved {
	c := t.Clone()
	saved := Saved{WrappedUp: c.wrappedUp, Collected: c.collected, StoryNews: c.storyNews, ApprovalNews: c.approvalNews}
	c.wrappedUp, c.collected, c.storyNews, c.approvalNews = 0, nil, false, false
	c.actionAt = nil // derived from Actions, so not saved: Restore builds it again
	saved.Task = *c
	return saved
}

// Restore is the task s was saved from. It shares nothing with s.
func Restore(s Saved) *Task {
	t := s.Task.Clone()
	t.wrappedUp, t.collected, t.storyNews, t.approvalNews = s.WrappedUp, maps.Clone(s.Collected), s.StoryNews, s.ApprovalNews
	t.actionAt = indexActions(t.Actions)
	return t
}
