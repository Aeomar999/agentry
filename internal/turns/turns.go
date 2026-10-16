// Package turns starts and stops agent turns. A turn is one agent working
// once: a fresh engine session in the task's worktree, started from a brief.
package turns

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/engines"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// StateDir is where Agentry keeps a task's files inside its worktree. Git
// ignores it (package worktrees).
const StateDir = ".agentry"

type Config struct {
	HookCommand string // runs `agentry hook`, e.g. "/usr/local/bin/agentry hook"
	ServerURL   string // where the hook posts its calls
	HookToken   string
}

type Runner struct {
	store  *store.Store
	config Config
}

var worker = events.Actor{Kind: events.Worker, ID: "local"}

func NewRunner(s *store.Store, config Config) *Runner {
	return &Runner{store: s, config: config}
}

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
	engine, ok := engines.Get(agent.Engine)
	if !ok {
		return fmt.Errorf("%s uses engine %q, which has no adapter", agent.Name, agent.Engine)
	}
	turn := task.NextTurnID()
	briefFile := filepath.Join(StateDir, turn+".md")
	if err := writeFile(filepath.Join(task.Worktree.Path, briefFile), brief); err != nil {
		return err
	}
	prompt := "Read " + briefFile + " and do what it says."

	session := engines.Session{Worktree: task.Worktree.Path, StateDir: filepath.Join(task.Worktree.Path, StateDir),
		HookCommand: r.config.HookCommand, Model: agent.Model}
	if err := engine.Prepare(session); err != nil {
		return fmt.Errorf("preparing %s's hooks: %w", agent.Engine, err)
	}

	// Recorded before the session starts, so its first hook call finds the turn open.
	_, rejection, err := r.store.Propose(events.Proposal{Stream: task.ID, Actor: worker, Type: events.TurnStarted,
		Correlation: turn, Data: map[string]any{"agent": agent.Name, "engine": agent.Engine, "hash": events.Hash(prompt)}})
	if err != nil {
		return err
	}
	if rejection != nil {
		return rejection
	}

	return sessions.Start(sessions.Spec{Socket: sessions.SocketFor(task.ID), Dir: task.Worktree.Path,
		Env: r.environment(task, turn, agent), Command: engine.Launch(session, prompt)})
}

// Stop ends the task's session. The turn itself ends by its own event.
func (r *Runner) Stop(task string) { sessions.Stop(sessions.SocketFor(task)) }

func (r *Runner) environment(task *tasks.Task, turn string, agent agents.Agent) map[string]string {
	deadline := time.Duration(engines.GateTimeoutSeconds-10) * time.Second
	return map[string]string{
		hookcall.EnvServer:   r.config.ServerURL,
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
	r.store.Propose(events.Proposal{Stream: task, Actor: worker, Type: events.IssueRaised,
		Data: map[string]any{"reason": reason}})
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
