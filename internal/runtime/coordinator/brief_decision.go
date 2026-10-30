package coordinator

import (
	"fmt"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// yourDecision is what the coordinator's brief ends with: the actions Agentry
// refused, what finishing waits on, and the decision to make.
func yourDecision(b *strings.Builder, task *tasks.Task, roster agents.Roster, hooks []extensions.Extension) {
	if refused := refusedActions(task); refused != "" {
		section(b, "Actions Agentry refused", refused)
	}
	if task.WrapUp != nil {
		section(b, "Wrap up", wrapUpText(task))
		section(b, "Your decision", WrapUpFormat)
		return
	}
	checks(b, task, roster)
	decision := DecisionFormat
	for _, hook := range hooks {
		if hook.Coordinator.Decision != "" {
			decision += "\n" + hook.Coordinator.Decision
		}
	}
	section(b, "Your decision", decision)
}

// wrapUpText tells the coordinator the user moved the task themselves, and
// what its one last turn may do.
func wrapUpText(task *tasks.Task) string {
	moved := "The user moved this task to Done themselves: it is finished as it is, and it stays done."
	if task.WrapUp.To != "done" {
		moved = "The user moved this task to " + tasks.PlaceName(task.WrapUp.To) + " themselves: it waits there until they move it on."
	}
	return moved + " Any turn in progress was stopped; its work so far is in the worktree.\n\n" +
		"This is your last turn on the task until they move it again: wrap it up. You may write its summary for the user, " +
		"commit or tidy the work in progress on its branch (never push), and note what was left undone and where the work is. " +
		"Start no new work: don't hand the task to an agent, ask, split, move it or pass it to another team, and don't reopen " +
		"it. Agentry keeps your summary and refuses any other decision."
}

// checks tells the coordinator what finishing the task waits on: its
// checkers' approval of the last change (unchecked).
func checks(b *strings.Builder, task *tasks.Task, roster agents.Roster) {
	rule := "Before you finish, the work must be checked. Since the last turn that changed it, every agent of your team " +
		"that checks work and has checked it must approve it again, in the verdict its report ends with. When a check asks for " +
		"changes, hand the findings to the agent that builds it; once they are made, hand it back for another check. Agentry " +
		"refuses \"done\" until then."
	if why := unchecked(task, roster); why != "" {
		section(b, "Before you finish", rule+"\n\nNow: "+why+".")
	} else if lead, _ := roster.Get(task.Coordinator); hasCheckers(roster, lead.TeamName()) {
		section(b, "Before you finish", rule+"\n\nNow: the work as it is has been checked, or nothing has changed it yet.")
	}
}

func hasCheckers(roster agents.Roster, team string) bool {
	for _, agent := range roster.Assignees(team) {
		if agent.Column == agents.Reviews {
			return true
		}
	}
	return false
}

func refusedActions(task *tasks.Task) string {
	var refused strings.Builder
	for _, action := range task.Actions {
		if action.Decision == tasks.Denied {
			fmt.Fprintf(&refused, "- %s (%s, %s): `%s`: %s\n", action.Agent, action.Turn, action.Tool, tasks.Shorten(action.Command, 200), action.Reason)
		}
	}
	return refused.String()
}
