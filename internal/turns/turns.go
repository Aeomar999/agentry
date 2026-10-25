// Package turns starts and stops agent turns. A turn is one agent working
// once in the task's worktree, started from a brief: in a fresh engine
// session, or going on with the agent's last one when the task allows it.
package turns

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
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
	NoResume  bool          // start every turn fresh, never going on with a session
}

// ResumeOff is why a turn starts fresh when Config.NoResume is set.
const ResumeOff = "resume is off"

// Runner starts turns on the machine the task's work is on.
type Runner struct {
	store  *store.Store   // the log: where turns are recorded
	state  *current.State // the tasks, for where their work is
	fleet  *machines.Fleet
	config Config

	// starting is held from a turn's last look at whether its session is
	// busy until its turn.started is in, so two turns starting at once
	// never both go on with one session in place.
	starting sync.Mutex
}

func NewRunner(s *store.Store, state *current.State, fleet *machines.Fleet, config Config) *Runner {
	return &Runner{store: s, state: state, fleet: fleet, config: config}
}

// startTimeout is how long a machine has to set a turn up and start it.
const startTimeout = 2 * time.Minute

// A Session is how a turn starts: going on with the agent's last engine
// session, here or on a sibling story, or fresh, and why. SessionToResume
// decides it, before the brief is made, so the brief and the launch always
// agree; a fork goes on from the same point as a resume, so Start may
// still turn one into the other.
type Session struct {
	Resume string // the engine session to go on with, "" for a fresh one
	From   string // the task whose turn Resume was
	Fork   bool   // go on with a copy of Resume, since another turn is busy in it
	Fresh  string // why it starts fresh, "" when it resumes
	Seen   int    // how much of this task's trail the resumed session's last brief saw, 0 for another task's
}

// Resumes says whether the turn goes on with a session.
func (s Session) Resumes() bool { return s.Resume != "" }

// SessionToResume is how the agent's next turn on the task starts, on the
// machine the task's work is on: by tasks.SessionToShare, across the
// stories of the task's epic.
func (r *Runner) SessionToResume(task *tasks.Task, agent agents.Agent) Session {
	if r.config.NoResume {
		return Session{Fresh: ResumeOff}
	}
	resume, from, fresh := task.SessionToShare(r.storiesBeside(task), agent.Name, agent.Engine, r.fleet.Of(task.Worktree.Worker).Name())
	if resume == "" {
		return Session{Fresh: fresh}
	}
	session := Session{Resume: resume, From: from.ID, Fork: tasks.SessionBusy(resume, r.state.Tasks())}
	if from == task {
		session.Seen = task.LastTurnOf(agent.Name).Seen
	}
	return session
}

// storiesBeside is every story of the task's epic, the task among them;
// none when it isn't a story. One that can't be read is left out.
func (r *Runner) storiesBeside(task *tasks.Task) []*tasks.Task {
	if task.Epic == "" {
		return nil
	}
	epic, err := r.state.Task(task.Epic)
	if err != nil || epic == nil {
		return nil
	}
	var stories []*tasks.Task
	for _, id := range epic.Stories {
		if story, err := r.state.Task(id); err == nil && story != nil {
			stories = append(stories, story)
		}
	}
	return stories
}

// Start begins the task's next turn: the agent works from brief, made from
// task as it is, in session. If it can't, it raises an issue, so the
// coordinator hears about it.
//
// The brief goes in a file and the prompt is one line pointing at it, never
// the brief itself: a pasted prompt gets wrapped and cut at 16 KB, and a
// leading "!" runs as a shell command past the gate (proof 02). The line's
// hash, echoed back by UserPromptSubmit, proves the agent got it.
func (r *Runner) Start(task *tasks.Task, agent agents.Agent, brief string, session Session) error {
	err := r.start(task, agent, brief, session)
	if err != nil {
		r.raiseIssue(task.ID, "the "+agent.Name+" turn didn't start: "+err.Error())
	}
	return err
}

func (r *Runner) start(task *tasks.Task, agent agents.Agent, brief string, session Session) error {
	if _, ok := engines.Get(agent.Engine); !ok {
		return fmt.Errorf("%s uses engine %q, which has no adapter", agent.Name, agent.Engine)
	}
	machine := r.fleet.Of(task.Worktree.Worker)
	id := task.NextTurnID()
	turn := machines.Turn{Task: task.ID, ID: id, Worktree: task.Worktree.Path, Engine: agent.Engine, Model: agent.Model,
		Brief: brief, BriefFile: filepath.Join(machines.StateDir, id+".md"), Knowledge: agent.Knowledge,
		Env: r.environment(task, id, agent), Resume: session.Resume}
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
	if err := r.recordStart(task, agent, id, machine.Name(), events.Hash(turn.Prompt), planFrom, &session); err != nil {
		return err
	}
	turn.Fork = session.Fork
	return machine.StartTurn(ctx, turn)
}

// recordStart records turn.started, before the session starts, so its first
// hook call finds the turn open. A session that has become busy since
// SessionToResume looked is forked: the check and the record are one step
// under r.starting, so of two turns starting from one session, the later
// sees the earlier open and forks.
func (r *Runner) recordStart(task *tasks.Task, agent agents.Agent, turn, worker, hash, planFrom string, session *Session) error {
	r.starting.Lock()
	defer r.starting.Unlock()
	data := map[string]any{"agent": agent.Name, "engine": agent.Engine, "hash": hash,
		"worker": worker, "planFrom": planFrom, "seen": len(task.Trail)}
	if session.Resumes() {
		session.Fork = session.Fork || tasks.SessionBusy(session.Resume, r.state.Tasks())
		data["fromSession"] = session.Resume
		if session.From != task.ID {
			data["sessionFrom"] = session.From
		}
		if session.Fork {
			data["forked"] = true
		}
	} else {
		data["freshBecause"] = session.Fresh
	}
	_, rejection, err := r.store.Propose(events.Proposal{Stream: task.ID, Actor: workerOf(worker), Type: events.TurnStarted,
		Correlation: turn, Data: data})
	if err != nil {
		return err
	}
	if rejection != nil {
		return rejection
	}
	return nil
}

// Stop ends the task's session, wherever it runs. The turn itself ends by its own event.
func (r *Runner) Stop(task string) { r.MachineOf(task).Stop(task) }

// IsRunning says whether the task's agent process is alive, wherever it runs.
func (r *Runner) IsRunning(task string) bool { return r.MachineOf(task).IsRunning(task) }

// enterPause keeps a typed line and its Enter apart, so the engine reads
// the Enter as sending the line, not as a newline pasted into it.
const enterPause = 300 * time.Millisecond

// TypeLine types a line into the task's session and presses Enter, wherever
// it runs, as you would in its terminal. Pane.Type drops write errors, so
// TypeLine checks only that it reached the session, not that the line arrived.
func (r *Runner) TypeLine(task, line string) error {
	pane, err := r.MachineOf(task).Attach(task, func([]byte) {})
	if err != nil {
		return err
	}
	defer pane.Detach()
	pane.Type([]byte(line))
	time.Sleep(enterPause)
	pane.Type([]byte("\r")) // what a terminal sends for Enter
	return nil
}

// MachineOf is the machine the task's work is on.
func (r *Runner) MachineOf(id string) machines.Machine {
	if task, err := r.state.Task(id); err == nil && task != nil && task.Worktree != nil {
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
