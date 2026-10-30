package ideas

import (
	"slices"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// Source is every idea, as kept from the log (current.State), read through
// the state's generic reads of an extension's streams.
type Source interface {
	Owned(stream string) (any, error) // nil, nil when the stream holds nothing
	OwnedIDs(prefix string) []string  // in log order, broken ones too
}

// Get is an idea's state: nil, nil when there is no such idea, and the error
// when its stream is broken.
func Get(source Source, id string) (*Idea, error) {
	held, err := source.Owned(id)
	if err != nil {
		return nil, err
	}
	idea, _ := held.(*Idea)
	return idea, nil
}

// All is every idea, oldest first, in a new slice the caller may sort. A
// broken idea is left out, as the task list leaves out a broken task.
func All(source Source) []*Idea {
	var all []*Idea
	for _, id := range source.OwnedIDs(Prefix) {
		if idea, err := Get(source, id); err == nil && idea != nil {
			all = append(all, idea)
		}
	}
	return all
}

// List returns the ideas about a repo, newest first, so a session's ideas
// stay together; repo "" lists every repo's. A broken idea is left out, as
// the task list skips a broken task, so one idea that can't be read doesn't
// empty the Ideas page.
func List(source Source, repo string) []*Idea {
	var list []*Idea
	for _, idea := range slices.Backward(All(source)) {
		if repo == "" || idea.Repo == repo {
			list = append(list, idea)
		}
	}
	return list
}

// InState are the ideas in list that are in state, in the same order. A
// Visionaries brief names those you accepted and dismissed, so they aren't
// suggested again.
func InState(list []*Idea, state State) []*Idea {
	var in []*Idea
	for _, idea := range list {
		if idea.State == state {
			in = append(in, idea)
		}
	}
	return in
}

// newestSessionFirst sorts list by when the task each idea came from was
// made, newest first, keeping list's order within a task.
func newestSessionFirst(read extensions.Reader, list []*Idea) {
	started := map[string]time.Time{}
	for _, idea := range list {
		if _, seen := started[idea.Task]; !seen {
			if task, err := read.Task(idea.Task); err == nil && task != nil {
				started[idea.Task] = task.CreatedAt
			}
		}
	}
	slices.SortStableFunc(list, func(a, b *Idea) int { return started[b.Task].Compare(started[a.Task]) })
}
