package coordinator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/turns"
	"gitlab.com/dev.chempa/agentry/internal/workers"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

// Loop reads the log and, for every task an event touched, does the one move
// Next says. Its own proposals are events too, so each one wakes it again:
// the task moves step by step, and a restart picks up where it was.
type Loop struct {
	store  *store.Store
	runner *turns.Runner
	fleet  *machines.Fleet
}

var agentry = events.Actor{Kind: events.System, ID: "coordinator-loop"}

func NewLoop(s *store.Store, runner *turns.Runner, fleet *machines.Fleet) *Loop {
	return &Loop{store: s, runner: runner, fleet: fleet}
}

// gitTimeout is how long a machine has to make, move or remove a worktree:
// a worker's first copy of a repo can take a while.
const gitTimeout = 10 * time.Minute

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
		l.askYou(task, move.Reason)
	}
}

// prepareWorktree makes the task's worktree here, where its repo is. If an
// agent that runs on a worker takes the task, the work moves there.
func (l *Loop) prepareWorktree(task *tasks.Task) {
	here, _ := l.fleet.Get(workers.Local)
	w, err := l.prepare(here, task, "")
	if err != nil {
		l.askYou(task, "Agentry couldn't set up the task's worktree: "+sentence(err)+" Fix it and answer, and it tries again.")
		return
	}
	l.propose(task.ID, workerActor(here), events.WorktreeReady, map[string]any{
		"repo": w.Repo, "worker": here.Name(), "path": w.Path, "branch": w.Branch, "base": w.Base, "port": w.Port})
}

func (l *Loop) prepare(m machines.Machine, task *tasks.Task, base string) (worktrees.Worktree, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	origin, _ := worktrees.Origin(task.Repo)
	return m.Prepare(ctx, machines.WorktreeRequest{Task: task.ID, Repo: task.Repo, Base: base,
		Ports: l.portsInUse(m.Name()), Origin: origin, RepoKey: worktrees.RepoKey(task.Repo)})
}

// startTurn starts the agent's turn on a machine it may run on, moving the
// work there first if it is somewhere else.
func (l *Loop) startTurn(task *tasks.Task, roster agents.Roster, move Move) {
	ctx, cancel := context.WithTimeout(context.Background(), machines.OfflineGrace)
	defer cancel()
	target, err := l.fleet.WaitFor(ctx, move.Agent, task.Worktree.Worker)
	if err != nil {
		l.askYou(task, sentence(err)+" Bring one of its workers online, or change where it runs, and answer to carry on.")
		return
	}
	if target.Name() != task.Worktree.Worker {
		l.moveWork(task, target, move.Agent) // when it has moved, the loop comes back here
		return
	}
	brief := workerBrief(task, move.Agent, move.Handoff)
	if move.Agent.Coordinates {
		brief = coordinatorBrief(task, move.Agent, roster)
	}
	if err := l.runner.Start(task, move.Agent, brief); err != nil {
		slog.Error("coordinator loop: starting a turn", "task", task.ID, "agent", move.Agent.Name, "err", err)
	}
}

// moveWork takes the task's work from the machine it is on to target: the
// first commits what was left and hands the branch to the hub, then target
// makes its worktree from it.
func (l *Loop) moveWork(task *tasks.Task, target machines.Machine, agent agents.Agent) {
	from, online := l.fleet.Get(task.Worktree.Worker)
	if !online {
		l.askYou(task, "The task's work is on "+task.Worktree.Worker+", which is offline, and "+agent.Name+
			" works on "+target.Name()+". Bring "+task.Worktree.Worker+" online and answer to carry on.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	message := fmt.Sprintf("Agentry: work left uncommitted on %s, moving to %s for %s", from.Name(), target.Name(), agent.Name)
	if err := from.HandOff(ctx, l.worktreeOf(task), message); err != nil {
		l.askYou(task, "Agentry couldn't move the work off "+from.Name()+": "+sentence(err)+" Fix it and answer, and it tries again.")
		return
	}
	w, err := l.prepare(target, task, task.Worktree.Base)
	if err != nil {
		l.askYou(task, "Agentry couldn't set the work up on "+target.Name()+": "+sentence(err)+
			" Its branch is safe in "+task.Repo+". Fix it and answer, and it tries again.")
		return
	}
	l.propose(task.ID, workerActor(target), events.WorktreeMoved, map[string]any{
		"worker": target.Name(), "from": from.Name(), "path": w.Path, "port": w.Port})
}

func (l *Loop) worktreeOf(task *tasks.Task) worktrees.Worktree {
	return worktrees.Worktree{Repo: task.Worktree.Repo, Path: task.Worktree.Path, Branch: task.Worktree.Branch,
		Base: task.Worktree.Base, Port: task.Worktree.Port}
}

func (l *Loop) askYou(task *tasks.Task, question string) {
	l.propose(task.ID, agentry, events.TaskNeedsYou, map[string]any{"question": question})
}

func workerActor(m machines.Machine) events.Actor {
	return events.Actor{Kind: events.Worker, ID: m.Name()}
}

// sentence is an error as the end of a sentence.
func sentence(err error) string { return strings.TrimRight(err.Error(), ". ") + "." }

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
		l.propose(task.ID, coordinator, events.TaskNeedsYou, map[string]any{"question": decision.Question,
			"options": decision.Options, "page": decision.Page})
	case Finish:
		if l.propose(task.ID, coordinator, events.TaskDone, map[string]any{"summary": decision.Summary}) {
			l.removeWorktree(task)
		}
	}
}

// removeWorktree cleans up a finished task, unless its branch isn't in dev
// yet: then the worktree is kept, so no work is lost (proof 06). On a
// worker, the hub's copy of the branch goes too once the worker's is merged.
func (l *Loop) removeWorktree(task *tasks.Task) {
	m := l.fleet.Of(task.Worktree.Worker)
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	err := m.Remove(ctx, l.worktreeOf(task))
	if err == nil && !machines.IsLocal(m.Name()) {
		err = worktrees.DeleteBranch(task.Worktree.Repo, task.Worktree.Branch)
	}
	if err != nil && !errors.Is(err, worktrees.ErrNotMerged) {
		slog.Warn("coordinator loop: removing the worktree", "task", task.ID, "err", err)
	}
}

// portsInUse are the ports of unfinished tasks whose work is on the machine.
func (l *Loop) portsInUse(machine string) []int {
	ids, _ := l.store.TaskIDs()
	var ports []int
	for _, id := range ids {
		task, _, err := l.load(id)
		if err == nil && task != nil && !task.IsDone && task.Worktree != nil && task.Worktree.Worker == machine {
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
