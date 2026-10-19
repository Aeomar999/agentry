package ideas

import (
	"slices"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/settings"
)

// VisionariesTeam is the team whose coordinators suggest ideas.
const VisionariesTeam = "Visionaries"

// Name is the feature's name, which a team template lists to make its
// coordinators suggest ideas.
const Name = "ideas"

// Suggests says whether the coordinator's team was made from a template
// that brings ideas, whatever the team is called. An agent from before
// templates were recorded has its template from its team's name (the
// roster sees to that), so an old Visionaries team still suggests them.
func Suggests(coordinator agents.Agent) bool {
	template, found := agents.TemplateWithID(coordinator.Template)
	return found && template.Lists(Name)
}

// MinBackers is how many viewpoints must back an idea nobody doubted before
// Agentry accepts it under settings.NobodyDoubted, so an idea one agent
// floated alone doesn't slip through.
const MinBackers = 2

// NotAutoAccepted says why Agentry wouldn't accept the idea for you under
// rule, "" when it would: it isn't open, it repeats another idea in list
// (Duplicate), or the Visionaries of the roster don't agree on it as rule
// asks (Disagreed). The loop, the rules and the Ideas page all ask it.
func NotAutoAccepted(idea *Idea, list []*Idea, roster agents.Roster, rule string) string {
	if idea.State != Open {
		return "it was " + string(idea.State) + " already"
	}
	if original := Duplicate(idea, list); original != nil {
		return duplicateText(original)
	}
	return Disagreed(idea, roster, VisionariesTeam, rule)
}

// LeftForYou is NotAutoAccepted, and when that would accept the idea, the
// open task in board it looks like (OpenTaskLike): Agentry never makes
// another of an open task, as only you may choose to. The loop and the
// Ideas page ask it.
func LeftForYou(board Tasks, idea *Idea, list []*Idea, roster agents.Roster, rule string) string {
	if why := NotAutoAccepted(idea, list, roster, rule); why != "" {
		return why
	}
	if like := OpenTaskLike(board, idea); like != nil {
		return "looks like the open task " + like.ID
	}
	return ""
}

// Disagreed says how the viewpoints of team fall short of agreeing on the
// idea under rule, "" when they agree. The viewpoints are the team's agents
// that don't coordinate, read from the roster, so a team with other
// viewpoints works; a name counts in any case and once. Any doubt is
// enough to disagree. EveryoneBacked then needs every viewpoint to back
// it, so one that didn't weigh in leaves it short; NobodyDoubted needs
// MinBackers of them. No such team, or one with no viewpoints, agrees on
// nothing, and a rule Agentry doesn't know is the strict one.
func Disagreed(idea *Idea, roster agents.Roster, team, rule string) string {
	if !slices.ContainsFunc(roster, isViewpointOf(team)) {
		return "the " + team + " team has no viewpoints"
	}
	if doubters := trimmed(idea.DoubtedBy); len(doubters) > 0 {
		return "doubted by " + strings.Join(doubters, ", ")
	}
	backers, missing := backing(idea, roster, team)
	if rule != settings.NobodyDoubted {
		if len(missing) > 0 {
			return "not backed by " + strings.Join(missing, ", ")
		}
		return ""
	}
	switch {
	case len(backers) >= MinBackers:
		return ""
	case len(backers) == 0:
		return "no backers"
	case len(backers) == 1:
		return "only one backer"
	}
	return "only " + strconv.Itoa(len(backers)) + " backers"
}

// Backers are the viewpoints of team, by their roster names, that back
// the idea.
func Backers(idea *Idea, roster agents.Roster, team string) []string {
	backers, _ := backing(idea, roster, team)
	return backers
}

// backing splits the viewpoints of team into those that back the idea and
// those that don't.
func backing(idea *Idea, roster agents.Roster, team string) (backers, missing []string) {
	for _, viewpoint := range roster {
		if !isViewpointOf(team)(viewpoint) {
			continue
		}
		backs := func(name string) bool { return strings.EqualFold(strings.TrimSpace(name), viewpoint.Name) }
		if slices.ContainsFunc(idea.BackedBy, backs) {
			backers = append(backers, viewpoint.Name)
		} else {
			missing = append(missing, viewpoint.Name)
		}
	}
	return backers, missing
}

// isViewpointOf: the agent is one of team's, in any case, and doesn't
// coordinate it.
func isViewpointOf(team string) func(agents.Agent) bool {
	return func(a agents.Agent) bool { return !a.Coordinates && strings.EqualFold(a.TeamName(), team) }
}

// trimmed are the names, trimmed, without the blank ones.
func trimmed(names []string) []string {
	var kept []string
	for _, name := range names {
		if name = strings.TrimSpace(name); name != "" {
			kept = append(kept, name)
		}
	}
	return kept
}
