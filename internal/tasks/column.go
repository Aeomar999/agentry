package tasks

import (
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/agents"
)

// UndoesYourColumn says why handing the task to agent would undo the column
// you moved it to, or "": moved to Review, it goes to an agent that checks
// work, if its team has one; moved to Shipping, to one that ships work, if
// its team still has one; moved from Review or Shipping back to In
// progress, to one that builds it.
func (t *Task) UndoesYourColumn(agent string, roster agents.Roster) string {
	to, _ := roster.Get(agent)
	switch {
	case t.Column == "review" && to.Column != agents.Reviews && TeamHas(roster, t.Coordinator, agents.Reviews):
		return "the user moved this task to Review; hand it to an agent that checks work"
	case t.Column == "shipping" && to.Column != agents.Ships && TeamHas(roster, t.Coordinator, agents.Ships):
		return "the user moved this task to Shipping; hand it to an agent that ships work"
	case t.Column == "in-progress" && t.SentBack && to.Column != agents.Builds:
		return "the user moved this task back to In progress; hand it to an agent that builds it"
	}
	return ""
}

// TeamHas says whether the coordinator's team has an agent whose work counts
// as column.
func TeamHas(roster agents.Roster, coordinator, column string) bool {
	lead, _ := roster.Get(coordinator)
	return slices.ContainsFunc(roster.Assignees(lead.TeamName()), func(a agents.Agent) bool { return a.Column == column })
}
