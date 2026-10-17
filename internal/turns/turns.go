// Package turns starts and stops agent turns. A turn is one agent working
// once: a fresh engine session in the task's worktree, started from a brief.
package turns

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/engines"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

type Config struct {
	HookToken string        // lets a session's hooks post to the server
	Theme     func() string // the page's theme now; nil leaves the engine's own
}

// Runner starts turns on the machine the task's work is on.
type Runner struct {
	store  *store.Store
	fleet  *machines.Fleet
	config Config
}

func NewRunner(s *store.Store, fleet *machines.Fleet, config Config) *Runner {
	return &Runner{store: s, fleet: fleet, config: config}
}

// startTimeout is how long a machine has to set a turn up and start it.
const startTimeout = 2 * time.Minute

// Start begins the task's next turn: the agent works from brief. If it
// can't, it raises an issue, so the coordinator hears about it.
//
// The brief goes in a file and the prompt is one line pointing at it, never
// the brief itself: a pasted prompt gets wrapped and cut at 16 KB, and a
// leading "!" runs as a shell command past the gate (proof 02). The line's
// hash, echoed back by UserPromptSubmit, proves the agent got it.
func (r *Runner) Start(task *tasks.Task, agent agents.Agent, brief string) error {
	err := r.start(task, agent, brief)
	if err != nil {
		r.raiseIssue(task.ID, "the "+agent.Name+" turn didn't start: "+err.Error())
	}
	return err
}

func (r *Runner) start(task *tasks.Task, agent agents.Agent, brief string) error {
	if _, ok := engines.Get(agent.Engine); !ok {
		return fmt.Errorf("%s uses engine %q, which has no adapter", agent.Name, agent.Engine)
	}
	machine := r.fleet.Of(task.Worktree.Worker)
	id := task.NextTurnID()
	turn := machines.Turn{Task: task.ID, ID: id, Worktree: task.Worktree.Path, Engine: agent.Engine, Model: agent.Model,
		Brief: brief, BriefFile: filepath.Join(machines.StateDir, id+".md"), Knowledge: agent.Knowledge,
		Env: r.environment(task, id, agent)}
	plan, planFrom := task.PlanToContinue(agent.Name) // its unfinished plan, to continue or replace
	turn.Plan = plan
	turn.Prompt = "Read " + turn.BriefFile + " and do what it says."
	if r.config.Theme != nil {
		turn.Theme = r.config.Theme()
	}

	ctx, cancel := context.WithTimeout(context.Background(), startTimeout)
	defer cancel()
	if err := machine.PrepareTurn(ctx, turn); err != nil {
		return err
	}
	// Recorded before the session starts, so its first hook call finds the turn open.
	_, rejection, err := r.store.Propose(events.Proposal{Stream: task.ID, Actor: workerOf(machine.Name()), Type: events.TurnStarted,
		Correlation: id, Data: map[string]any{"agent": agent.Name, "engine": agent.Engine, "hash": events.Hash(turn.Prompt),
			"worker": machine.Name(), "planFrom": planFrom}})
	if err != nil {
		return err
	}
	if rejection != nil {
		return rejection
	}
	return machine.StartTurn(ctx, turn)
}

// Stop ends the task's session, wherever it runs. The turn itself ends by its own event.
func (r *Runner) Stop(task string) { r.MachineOf(task).Stop(task) }

// IsRunning says whether the task's agent process is alive, wherever it runs.
func (r *Runner) IsRunning(task string) bool { return r.MachineOf(task).IsRunning(task) }

// MachineOf is the machine the task's work is on.
func (r *Runner) MachineOf(id string) machines.Machine {
	stream, err := r.store.Stream(id)
	if task := tasks.FromEvents(stream); err == nil && task != nil && task.Worktree != nil {
		return r.fleet.Of(task.Worktree.Worker)
	}
	return r.fleet.Of(workers.Local)
}

func workerOf(machine string) events.Actor { return events.Actor{Kind: events.Worker, ID: machine} }

func (r *Runner) environment(task *tasks.Task, turn string, agent agents.Agent) map[string]string {
	deadline := time.Duration(engines.GateTimeoutSeconds-10) * time.Second
	return map[string]string{
		hookcall.EnvToken:    r.config.HookToken,
		hookcall.EnvTask:     task.ID,
		hookcall.EnvTurn:     turn,
		hookcall.EnvAgent:    agent.Name,
		hookcall.EnvEngine:   agent.Engine,
		hookcall.EnvDeadline: deadline.String(),
		"PORT":               strconv.Itoa(task.Worktree.Port),
	}
}

func (r *Runner) raiseIssue(task, reason string) {
	r.store.Propose(events.Proposal{Stream: task, Actor: events.Actor{Kind: events.System, ID: "turns"}, Type: events.IssueRaised,
		Data: map[string]any{"reason": reason}})
}
