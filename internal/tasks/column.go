package tasks

import (
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/agents"
)

// UndoesYourColumn says why handing the task to agent would undo the column
// you moved it to, or "": moved to Review, it goes to an agent that checks
// work, if its team has one; moved from Review back to In progress, to one
// that doesn't.
func (t *Task) UndoesYourColumn(agent string, roster agents.Roster) string {
	to, _ := roster.Get(agent)
	checks := to.Column == agents.Reviews
	switch {
	case t.Column == "review" && !checks && hasAChecker(roster, t.Coordinator):
		return "the user moved this task to Review; hand it to an agent that checks work"
	case t.Column == "in-progress" && t.SentBack && checks:
		return "the user moved this task back to In progress; hand it to an agent that builds it"
	}
	return ""
}

// hasAChecker says whether the coordinator's team has an agent that checks work.
func hasAChecker(roster agents.Roster, coordinator string) bool {
	lead, _ := roster.Get(coordinator)
	return slices.ContainsFunc(roster.Assignees(lead.TeamName()), func(a agents.Agent) bool { return a.Column == agents.Reviews })
}
