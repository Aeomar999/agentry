package ideas

import "slices"

// Source is every idea, as kept from the log (current.State).
type Source interface {
	Ideas() []*Idea                // oldest first, broken ones left out
	Idea(id string) (*Idea, error) // nil, nil when there is no such idea
}

// List returns the ideas about a repo, newest first, so a session's ideas
// stay together; repo "" lists every repo's. A broken idea is left out, as
// the task list skips a broken task, so one idea that can't be read doesn't
// empty the Ideas page.
func List(source Source, repo string) []*Idea {
	var list []*Idea
	for _, idea := range slices.Backward(source.Ideas()) {
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
