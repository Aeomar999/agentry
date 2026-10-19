package ideas

import (
	"log/slog"
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/store"
)

// Load builds one idea from the log; nil, nil when there is no such idea.
func Load(log *store.Store, id string) (*Idea, error) {
	stream, err := log.Stream(id)
	if err != nil {
		return nil, err
	}
	return FromEvents(stream), nil
}

// List returns the ideas about a repo, newest first, so a session's ideas
// stay together; repo "" lists every repo's.
func List(log *store.Store, repo string) ([]*Idea, error) {
	ids, err := log.IdeaIDs()
	if err != nil {
		return nil, err
	}
	var list []*Idea
	for _, id := range slices.Backward(ids) {
		idea, err := Load(log, id)
		if err != nil {
			// As the task list skips a broken task: one idea that can't be
			// read must not empty the Ideas page.
			slog.Error("ideas: an idea can't be loaded, so the list leaves it out", "idea", id, "err", err)
			continue
		}
		if idea != nil && (repo == "" || idea.Repo == repo) {
			list = append(list, idea)
		}
	}
	return list, nil
}

// Dismissals are the ideas in list you dismissed, in the same order. A
// Visionaries brief names them, with your note, so they aren't suggested again.
func Dismissals(list []*Idea) []*Idea {
	var dismissed []*Idea
	for _, idea := range list {
		if idea.State == Dismissed {
			dismissed = append(dismissed, idea)
		}
	}
	return dismissed
}
