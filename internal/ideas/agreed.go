package ideas

import (
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
)

// Agreed says whether every viewpoint of team, as the roster spells it,
// backs the idea and none doubts it. The viewpoints are the team's agents
// that don't coordinate, read from the roster, so a team with other
// viewpoints works. A viewpoint that didn't weigh in leaves it short, and
// no such team, or one with no viewpoints, agrees on nothing.
func Agreed(idea *Idea, roster agents.Roster, team string) bool {
	viewpoints := roster.Assignees(team)
	if len(viewpoints) == 0 || len(idea.DoubtedBy) > 0 {
		return false
	}
	for _, viewpoint := range viewpoints {
		backs := func(name string) bool { return strings.EqualFold(strings.TrimSpace(name), viewpoint.Name) }
		if !slices.ContainsFunc(idea.BackedBy, backs) {
			return false
		}
	}
	return true
}

// Repeats says why Agentry mustn't accept the idea for you, "" when it
// may: it isn't open, or list, the ideas about its repo, has one under the
// same title that you dismissed or that was accepted already. made says
// whether a task was made from an idea, accepted or not.
func Repeats(idea *Idea, list []*Idea, made func(id string) bool) string {
	if idea.State != Open {
		return "it was " + string(idea.State) + " already"
	}
	for _, other := range list {
		if other.ID == idea.ID || other.Repo != idea.Repo || !sameTitle(other.Title, idea.Title) {
			continue
		}
		switch {
		case other.State == Dismissed:
			return "you dismissed " + other.ID + ", under the same title"
		case other.State == Accepted || made(other.ID):
			return other.ID + ", under the same title, is a task already"
		}
	}
	return ""
}

func sameTitle(a, b string) bool {
	return strings.EqualFold(strings.TrimSpace(a), strings.TrimSpace(b))
}
