package coordinator

import (
	"fmt"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// The loop's safety stops: without them a coordinator that keeps sending work
// back, or an agent that keeps failing, would loop forever.
const (
	MaxTurns     = 24 // about 12 handoffs there and back, then you decide
	MaxIssuesRun = 3  // issues in a row with no turn ending normally
)

// Move is the one thing the loop should do next for a task.
type Move struct {
	Kind     MoveKind
	Agent    agents.Agent // for StartTurn
	Handoff  string       // for StartTurn: what the agent is given
	Decision Decision     // for ApplyDecision
	Reason   string       // for RaiseIssue and AskYou
}

type MoveKind string

const (
	Wait            MoveKind = "wait" // an agent is working, or you are deciding
	PrepareWorktree MoveKind = "prepare the worktree"
	StartTurn       MoveKind = "start a turn"
	ApplyDecision   MoveKind = "apply the coordinator's decision"
	RaiseIssue      MoveKind = "raise an issue"
	AskYou          MoveKind = "ask you"
)

// Next decides what should happen next for a task, from its state alone, so
// the loop can stop and restart at any point and pick up where it was.
func Next(task *tasks.Task, roster agents.Roster) Move {
	switch {
	case task.IsDone, task.Question != "", task.OpenTurn() != nil, task.Parked != "":
		return Move{Kind: Wait}
	case task.Worktree == nil:
		return Move{Kind: PrepareWorktree}
	case task.PendingDecision != "":
		return decide(task, roster)
	case task.Handoff == nil:
		return Move{Kind: Wait}
	case task.TurnsSinceAnswer >= MaxTurns:
		return Move{Kind: AskYou, Reason: fmt.Sprintf(
			"This task has had %d turns without finishing. Answer with any guidance and the coordinator carries on.", task.TurnsSinceAnswer)}
	case task.Budget > 0 && task.TokensUsed >= task.Budget && task.TurnsSinceAnswer > 0:
		// Once you answer, one more turn runs, so "wrap up" can be done.
		return Move{Kind: AskYou, Reason: fmt.Sprintf(
			"This task has used %s tokens of its %s budget. Raise the budget to go on, or tell the coordinator to wrap up.",
			Tokens(task.TokensUsed), Tokens(task.Budget))}
	case task.IssuesInARow >= MaxIssuesRun:
		return Move{Kind: AskYou, Reason: fmt.Sprintf(
			"%d issues in a row, the last: %s. Fix what's wrong and answer, and the coordinator carries on.", task.IssuesInARow, task.Handoff.Text)}
	}

	agent, exists := roster.Get(task.Handoff.To)
	if !exists && task.Handoff.To == task.Coordinator {
		return Move{Kind: AskYou, Reason: "The task's coordinator, " + task.Coordinator + ", no longer exists."}
	}
	if !exists {
		return Move{Kind: RaiseIssue, Reason: "there is no agent called " + task.Handoff.To}
	}
	return Move{Kind: StartTurn, Agent: agent, Handoff: task.Handoff.Text}
}

func decide(task *tasks.Task, roster agents.Roster) Move {
	decision, err := ParseDecision(task.PendingDecision)
	if err != nil {
		return Move{Kind: RaiseIssue, Reason: "the coordinator's reply had no usable decision: " + err.Error()}
	}
	if decision.Action == Assign {
		if _, exists := roster.Get(decision.Agent); !exists {
			return Move{Kind: RaiseIssue, Reason: "the coordinator assigned the task to " + decision.Agent + ", and there is no agent by that name"}
		}
	}
	return Move{Kind: ApplyDecision, Decision: decision}
}

// Tokens writes a count of tokens as people read it: 950, 12.4k, 3.1M.
func Tokens(n int) string {
	switch {
	case n >= 1_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e6), ".0") + "M"
	case n >= 1_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e3), ".0") + "k"
	}
	return fmt.Sprint(n)
}
