package rules

import (
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// A rule returns "" when the proposal may happen now, or the reason it may not.
type rule func(t *tasks.Task, p events.Proposal) string

// taskRules has one entry per task event type, except task.created, which
// Check handles. Each lists the conditions that must all hold.
var taskRules = map[events.Type]rule{
	events.WorktreeReady: all(noWorktreeYet),

	events.TaskAssigned: all(byTheCoordinator, noOpenTurn, noQuestionOpen, notToTheCoordinator),
	events.TurnStarted:  all(noOpenTurn, noQuestionOpen, worktreeReady, isTheNextTurn),

	events.PromptAcknowledged: all(isTheOpenTurn, notYetAcknowledged, hashMatches),
	events.SessionStarted:     all(isTheOpenTurn),

	events.ActionRequested: all(byTheWorkingAgent, isNewAction),
	events.ActionAsked:     all(actionUndecided, notYetAsked),
	events.ActionApproved:  all(actionUndecided),
	events.ActionDenied:    all(actionUndecided),
	events.ActionResult:    all(actionApproved, noResultYet),

	events.AgentWaiting: all(isTheOpenTurn),
	events.TurnEnded:    all(isTheOpenTurn, sameAgentAsTheTurn),
	events.IssueRaised:  all(),

	events.TaskNeedsYou: all(byTheCoordinatorOrAgentry, noOpenTurn, noQuestionOpen),
	events.TaskAnswered: all(questionOpen),
	events.TaskDone:     all(byTheCoordinator, noOpenTurn),
}

func all(rules ...rule) rule {
	return func(t *tasks.Task, p events.Proposal) string {
		for _, r := range rules {
			if reason := r(t, p); reason != "" {
				return reason
			}
		}
		return ""
	}
}

// ---- who ----------------------------------------------------------------------

func byTheCoordinator(t *tasks.Task, p events.Proposal) string {
	if p.Actor.ID != t.Coordinator {
		return p.Actor.ID + " is not this task's coordinator (" + t.Coordinator + ")"
	}
	return ""
}

func byTheCoordinatorOrAgentry(t *tasks.Task, p events.Proposal) string {
	if p.Actor.Kind == events.System {
		return ""
	}
	return byTheCoordinator(t, p)
}

func byTheWorkingAgent(t *tasks.Task, p events.Proposal) string {
	if t.OpenTurn() == nil {
		return "no agent is working on the task"
	}
	if working := t.OpenTurn().Agent; p.Actor.ID != working {
		return p.Actor.ID + " is not the agent working on the task (" + working + ")"
	}
	return ""
}

func notToTheCoordinator(t *tasks.Task, p events.Proposal) string {
	if p.Text("agent") == t.Coordinator {
		return "the coordinator can't assign the task to itself"
	}
	return ""
}

// ---- turns --------------------------------------------------------------------

func noOpenTurn(t *tasks.Task, _ events.Proposal) string {
	if turn := t.OpenTurn(); turn != nil {
		return turn.Agent + " is still working (" + turn.ID + ")"
	}
	return ""
}

func isTheNextTurn(t *tasks.Task, p events.Proposal) string {
	if p.Correlation != t.NextTurnID() {
		return "the next turn is " + t.NextTurnID() + ", not " + p.Correlation
	}
	return ""
}

func isTheOpenTurn(t *tasks.Task, p events.Proposal) string {
	if turn := t.OpenTurn(); turn == nil || turn.ID != p.Correlation {
		return p.Correlation + " is not the turn in progress"
	}
	return ""
}

func sameAgentAsTheTurn(t *tasks.Task, p events.Proposal) string {
	if agent := t.OpenTurn().Agent; p.Text("agent") != agent {
		return p.Correlation + " belongs to " + agent + ", not " + p.Text("agent")
	}
	return ""
}

func notYetAcknowledged(t *tasks.Task, p events.Proposal) string {
	if t.Turn(p.Correlation).Acknowledged {
		return "the prompt of " + p.Correlation + " was already acknowledged"
	}
	return ""
}

func hashMatches(t *tasks.Task, p events.Proposal) string {
	if p.Text("hash") != t.Turn(p.Correlation).Hash {
		return "the agent received different text than was sent (hash mismatch)"
	}
	return ""
}

func worktreeReady(t *tasks.Task, _ events.Proposal) string {
	if t.Worktree == nil {
		return "the task's worktree isn't ready"
	}
	return ""
}

func noWorktreeYet(t *tasks.Task, _ events.Proposal) string {
	if t.Worktree != nil {
		return "the task already has a worktree"
	}
	return ""
}

// ---- actions ------------------------------------------------------------------

func isNewAction(t *tasks.Task, p events.Proposal) string {
	if t.Action(p.Correlation) != nil {
		return "action " + p.Correlation + " was already requested"
	}
	return ""
}

func actionUndecided(t *tasks.Task, p events.Proposal) string {
	action := t.Action(p.Correlation)
	switch {
	case action == nil:
		return "action " + p.Correlation + " doesn't exist"
	case action.Decision != tasks.Undecided:
		return "action " + p.Correlation + " was already " + string(action.Decision)
	}
	return ""
}

func notYetAsked(t *tasks.Task, p events.Proposal) string {
	if t.Action(p.Correlation).Asked {
		return "action " + p.Correlation + " was already asked about"
	}
	return ""
}

func actionApproved(t *tasks.Task, p events.Proposal) string {
	action := t.Action(p.Correlation)
	switch {
	case action == nil:
		return "action " + p.Correlation + " doesn't exist"
	case action.Decision != tasks.Approved:
		return "action " + p.Correlation + " was never approved"
	}
	return ""
}

func noResultYet(t *tasks.Task, p events.Proposal) string {
	if t.Action(p.Correlation).HasResult {
		return "action " + p.Correlation + " already has a result"
	}
	return ""
}

// ---- questions ----------------------------------------------------------------

func noQuestionOpen(t *tasks.Task, _ events.Proposal) string {
	if t.Question != "" {
		return "the task is waiting for your answer"
	}
	return ""
}

func questionOpen(t *tasks.Task, _ events.Proposal) string {
	if t.Question == "" {
		return "the task isn't waiting for an answer"
	}
	return ""
}
