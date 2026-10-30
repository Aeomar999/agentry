package coordinator

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/platform/worktrees"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
)

// gitTimeout is how long a machine has to make, move or remove a worktree:
// a worker's first copy of a repo can take a while.
const gitTimeout = 10 * time.Minute

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

func workerActor(m machines.Machine) events.Actor {
	return events.Actor{Kind: events.Worker, ID: m.Name()}
}

// sentence is an error as the end of a sentence.
func sentence(err error) string { return strings.TrimRight(err.Error(), ". ") + "." }

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

// portsInUse are the ports of tasks whose work is on the machine and not
// over: unfinished, or done by you and still wrapping up, since a reopen
// then works in the same worktree. A broken task's port counts too, read
// from its stream, since nothing says its worktree has gone. An archived
// task is done, so it is skipped like one, without reading its stream, and
// an erased one has no stream left to read.
func (l *Loop) portsInUse(machine string) []int {
	var ports []int
	for _, id := range l.state.TaskIDs() {
		task, err := l.state.Task(id)
		if errors.Is(err, current.ErrArchived) || errors.Is(err, current.ErrErased) {
			continue
		}
		if err != nil {
			if port, ok := l.lastPort(id, machine); ok {
				ports = append(ports, port)
			}
			continue
		}
		if task != nil && (!task.IsDone || task.WrapUp != nil) && task.Worktree != nil && task.Worktree.Worker == machine {
			ports = append(ports, task.Worktree.Port)
		}
	}
	return ports
}

// lastPort is the port a task's stream last gave its worktree, if that
// worktree is on the machine: the worker and port of its last
// worktree.ready or worktree.moved, read as tasks' apply reads them. It
// needs none of the task's other state, so it works on a broken task. A
// reopen or a repo move that clears the worktree is ignored, so a broken
// task may hold a stale port: one port at most, which is fine.
func (l *Loop) lastPort(id, machine string) (int, bool) {
	stream, err := l.store.Stream(id)
	if err != nil {
		slog.Warn("coordinator loop: a broken task's port can't be read", "task", id, "err", err)
		return 0, false
	}
	var worker string
	var port int
	for _, e := range stream {
		switch e.Type {
		case events.WorktreeReady:
			worker, port = cmp.Or(e.Text("worker"), workers.Local), e.Number("port")
		case events.WorktreeMoved:
			worker, port = e.Text("worker"), e.Number("port")
		}
	}
	return port, worker != "" && worker == machine
}
