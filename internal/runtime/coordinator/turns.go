package coordinator

import (
	"context"
	"log/slog"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/platform/workspace"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
	"gitlab.com/dev.chempa/agentry/internal/runtime/turns"
)

// endWrapUp ends the coordinator's last turn after you moved the task, with
// its summary, or why it has none. A task you finished is cleaned up then,
// as one its coordinator finished is.
func (l *Loop) endWrapUp(task *tasks.Task, move Move) {
	by, data := agentry, map[string]any{"reason": move.Reason}
	if move.Reason == "" {
		by = events.Actor{Kind: events.Agent, ID: task.Coordinator}
		data = map[string]any{"summary": move.Decision.Summary, "toYou": move.Decision.ToYou}
	}
	if l.propose(task.ID, by, events.TaskWrappedUp, data) && task.IsDone && task.Worktree != nil {
		l.removeWorktree(task)
	}
}

// startTurn starts the agent's turn on a machine it may run on, moving the
// work there first if it is somewhere else.
func (l *Loop) startTurn(task *tasks.Task, roster agents.Roster, move Move) {
	ctx, cancel := context.WithTimeout(context.Background(), machines.OfflineGrace)
	defer cancel()
	if move.Machine != "" {
		// The coordinator chose it: if it can't be had, the coordinator
		// hears why and chooses again.
		target, err := l.fleet.WaitOn(ctx, move.Agent, move.Machine)
		if err != nil {
			l.propose(task.ID, agentry, events.IssueRaised, map[string]any{"reason": "the turn couldn't start where you chose: " + sentence(err)})
			return
		}
		l.startOn(task, roster, move, target)
		return
	}
	target, err := l.fleet.WaitFor(ctx, move.Agent, task.Worktree.Worker)
	if err != nil {
		l.askYou(task, sentence(err)+" Bring one of its workers online, or change where it runs, and answer to carry on.")
		return
	}
	l.startOn(task, roster, move, target)
}

// startOn starts the turn on target, moving the work there first if it is
// somewhere else.
func (l *Loop) startOn(task *tasks.Task, roster agents.Roster, move Move, target machines.Machine) {
	if last := task.LastEndedTurn(); last != nil && !task.FilesCollected(last.ID) {
		l.readyForTurn(task, task.Worktree.Worker) // read the last turn's files back where it ran, before the work moves
		return
	}
	if target.Name() != task.Worktree.Worker {
		l.moveWork(task, target, move.Agent) // when it has moved, the loop comes back here
		return
	}
	if !l.readyForTurn(task, target.Name()) {
		return // the task's files are on their way: the loop comes back here
	}
	session := l.runner.SessionToResume(task, move.Agent)
	if err := l.runner.Start(task, move.Agent, l.briefFor(task, roster, move, session), session); err != nil {
		slog.Error("coordinator loop: starting a turn", "task", task.ID, "agent", move.Agent.Name, "err", err)
	}
}

// briefFor is the brief for the agent's turn in session: what has changed
// since for one that goes on with its last session; for a fresh one, the
// whole of it, or the handoff for an agent that checks the work.
func (l *Loop) briefFor(task *tasks.Task, roster agents.Roster, move Move, session turns.Session) string {
	shared, _ := l.filesFor(task)
	agent := move.Agent
	if session.Resumes() {
		if agent.Coordinates {
			return resumedCoordinatorBrief(task, agent, roster, session.Seen, session.From, l.familyOf(task), l.machines(), shared, l.state, l.Extensions)
		}
		return resumedWorkerBrief(task, agent, roster, move.Handoff, session.Seen, session.From, shared)
	}
	if agent.Coordinates {
		return coordinatorBrief(task, agent, roster, l.familyOf(task), l.machines(), shared, l.state, l.Extensions)
	}
	if agent.Column == agents.Reviews {
		return handoffBrief(task, agent, roster, move.Handoff, shared, l.commitsToReview(task))
	}
	return workerBrief(task, agent, roster, move.Handoff, shared)
}

// commitsTimeout is how long the machine the work is on has to list the
// task branch's commits for a handoff: a brief shouldn't wait on it.
const commitsTimeout = 10 * time.Second

// commitsToReview is the task branch's commits, newest first, for a
// checking agent's handoff; none, with a warning, when the machine the
// work is on can't list them in time. The brief says so and gives the
// commands instead.
func (l *Loop) commitsToReview(task *tasks.Task) []workspace.Commit {
	ctx, cancel := context.WithTimeout(context.Background(), commitsTimeout)
	defer cancel()
	commits, err := l.fleet.Of(task.Worktree.Worker).Commits(ctx, workspace.Workspace{Dir: task.Worktree.Path, Base: task.Worktree.Base})
	if err != nil {
		slog.Warn("coordinator loop: listing the commits to review", "task", task.ID, "err", err)
		return nil
	}
	return commits
}

func (l *Loop) askYou(task *tasks.Task, question string) { l.askAt(task, question, "") }

// askAt asks you, saying which limit stopped the task, if one did.
func (l *Loop) askAt(task *tasks.Task, question, limit string) {
	data := map[string]any{"question": question}
	if limit != "" {
		data["limit"] = limit
	}
	l.propose(task.ID, agentry, events.TaskNeedsYou, data)
}
