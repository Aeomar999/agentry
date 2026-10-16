package coordinator

import (
	"context"
	"errors"
	"log/slog"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/turns"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

// Loop reads the log and, for every task an event touched, does the one move
// Next says. Its own proposals are events too, so each one wakes it again:
// the task moves step by step, and a restart picks up where it was.
type Loop struct {
	store        *store.Store
	runner       *turns.Runner
	worktreeRoot string
}

var (
	agentry = events.Actor{Kind: events.System, ID: "coordinator-loop"}
	worker  = events.Actor{Kind: events.Worker, ID: "local"}
)

func NewLoop(s *store.Store, runner *turns.Runner, worktreeRoot string) *Loop {
	return &Loop{store: s, runner: runner, worktreeRoot: worktreeRoot}
}

func (l *Loop) Run(ctx context.Context) error {
	reader := l.store.NewReader(0)
	defer reader.Close()
	for {
		batch, err := reader.Next(ctx)
		if err != nil {
			return err
		}
		for _, id := range touchedTasks(batch) {
			l.act(id)
		}
	}
}

// touchedTasks are the tasks a batch changed, each once, in order.
func touchedTasks(batch []events.Event) []string {
	seen := map[string]bool{}
	var ids []string
	for _, e := range batch {
		if !events.IsTaskStream(e.Stream) || e.Type == events.ProposalRejected || seen[e.Stream] {
			continue
		}
		seen[e.Stream] = true
		ids = append(ids, e.Stream)
	}
	return ids
}

func (l *Loop) act(id string) {
	task, roster, err := l.load(id)
	if err != nil || task == nil {
		slog.Error("coordinator loop: reading the task", "task", id, "err", err)
		return
	}
	if task.OpenTurn() == nil {
		l.runner.Stop(id) // the turn is over: end its session
	}

	switch move := Next(task, roster); move.Kind {
	case PrepareWorktree:
		l.prepareWorktree(task)
	case StartTurn:
		l.startTurn(task, roster, move)
	case ApplyDecision:
		l.apply(task, move.Decision)
	case RaiseIssue:
		l.propose(task.ID, agentry, events.IssueRaised, map[string]any{"reason": move.Reason})
	case AskYou:
		l.propose(task.ID, agentry, events.TaskNeedsYou, map[string]any{"question": move.Reason})
	}
}

func (l *Loop) prepareWorktree(task *tasks.Task) {
	port, err := worktrees.LeasePort(l.portsInUse())
	if err == nil {
		var w worktrees.Worktree
		if w, err = worktrees.Create(task.Repo, l.worktreeRoot, task.ID, port); err == nil {
			l.propose(task.ID, worker, events.WorktreeReady, map[string]any{
				"repo": w.Repo, "path": w.Path, "branch": w.Branch, "base": w.Base, "port": w.Port})
			return
		}
	}
	l.propose(task.ID, agentry, events.TaskNeedsYou, map[string]any{
		"question": "Agentry couldn't set up the task's worktree: " + strings.TrimRight(err.Error(), ". ") + ". Fix it and answer, and it tries again."})
}

func (l *Loop) startTurn(task *tasks.Task, roster agents.Roster, move Move) {
	brief := workerBrief(task, move.Agent, move.Handoff)
	if move.Agent.Coordinates {
		brief = coordinatorBrief(task, move.Agent, roster)
	}
	if err := l.runner.Start(task, move.Agent, brief); err != nil {
		slog.Error("coordinator loop: starting a turn", "task", task.ID, "agent", move.Agent.Name, "err", err)
	}
}

// apply turns the coordinator's decision into its event, written as the coordinator.
func (l *Loop) apply(task *tasks.Task, decision Decision) {
	coordinator := events.Actor{Kind: events.Agent, ID: task.Coordinator}
	if title := strings.TrimSpace(decision.Title); title != "" && title != task.Title {
		l.propose(task.ID, coordinator, events.TaskRenamed, map[string]any{"title": title})
	}
	switch decision.Action {
	case Assign:
		l.propose(task.ID, coordinator, events.TaskAssigned, map[string]any{"agent": decision.Agent, "instructions": decision.Instructions})
	case Ask:
		l.propose(task.ID, coordinator, events.TaskNeedsYou, map[string]any{"question": decision.Question})
	case Finish:
		if l.propose(task.ID, coordinator, events.TaskDone, map[string]any{"summary": decision.Summary}) {
			l.removeWorktree(task)
		}
	}
}

// removeWorktree cleans up a finished task, unless its branch isn't in dev
// yet: then the worktree is kept, so no work is lost (proof 06).
func (l *Loop) removeWorktree(task *tasks.Task) {
	w := worktrees.Worktree{Repo: task.Worktree.Repo, Path: task.Worktree.Path, Branch: task.Worktree.Branch}
	if err := w.Remove(); err != nil && !errors.Is(err, worktrees.ErrNotMerged) {
		slog.Warn("coordinator loop: removing the worktree", "task", task.ID, "err", err)
	}
}

func (l *Loop) portsInUse() []int {
	ids, _ := l.store.TaskIDs()
	var ports []int
	for _, id := range ids {
		if task, _, err := l.load(id); err == nil && task != nil && !task.IsDone && task.Worktree != nil {
			ports = append(ports, task.Worktree.Port)
		}
	}
	return ports
}

func (l *Loop) load(id string) (*tasks.Task, agents.Roster, error) {
	stream, err := l.store.Stream(id)
	if err != nil {
		return nil, nil, err
	}
	agentEvents, err := l.store.Stream(events.AgentsStream)
	if err != nil {
		return nil, nil, err
	}
	return tasks.FromEvents(stream), agents.FromEvents(agentEvents), nil
}

// propose reports whether the event was accepted. A refusal is already in
// the log as proposal.rejected; it is logged here too, for whoever runs the server.
func (l *Loop) propose(task string, actor events.Actor, kind events.Type, data map[string]any) bool {
	_, rejection, err := l.store.Propose(events.Proposal{Stream: task, Actor: actor, Type: kind, Data: data})
	if err != nil || rejection != nil {
		slog.Warn("coordinator loop: proposal refused", "task", task, "type", kind, "rejection", rejection, "err", err)
		return false
	}
	return true
}
