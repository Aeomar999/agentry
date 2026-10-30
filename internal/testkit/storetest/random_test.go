package storetest

import (
	"fmt"
	"maps"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

var (
	you      = events.Actor{Kind: events.Human, ID: "you"}
	worker   = events.Actor{Kind: events.Worker, ID: "local"}
	policy   = events.Actor{Kind: events.Policy, ID: "rules"}
	watchdog = events.Actor{Kind: events.System, ID: "watchdog"}
)

// The small worlds the steps draw from, so they keep running into each
// other: the same few tasks, agents, turns and machines.
var (
	taskIDs      = []string{"task-1", "task-2", "task-3", "task-4", "task-5", "task-6"}
	agentNames   = []string{"tech-lead", "engineer", "reviewer", "tester", "designer"}
	turnIDs      = []string{"turn-1", "turn-2", "turn-3", "turn-4", "turn-5", "turn-6"}
	machineNames = []string{"laptop", "desktop", workers.Local}
)

// generator makes random proposals. A task step aims at what the task or
// the roster holds (its next turn, its agent, its last action, the
// roster's coordinator) as often as at anything else, and its
// actor is often the wrong one, so many steps are refused and some get
// deep into a task.
type generator struct {
	rand    *rand.Rand
	state   *current.State
	codes   []string // join codes invited so far, and one nobody made
	actions int      // action requests made so far, each with its own id
}

func pick[T any](g *generator, list ...T) T { return list[g.rand.IntN(len(list))] }

func (g *generator) agentActor() events.Actor {
	return events.Actor{Kind: events.Agent, ID: pick(g, agentNames...)}
}

func (g *generator) anyActor() events.Actor {
	return pick(g, you, worker, policy, watchdog, g.agentActor())
}

// onTask is a step on a random task, built from what the state holds for
// it: an empty task before it is created.
func (g *generator) onTask(build func(task *tasks.Task) events.Proposal) events.Proposal {
	id := pick(g, taskIDs...)
	task, _ := g.state.Task(id)
	if task == nil {
		task = &tasks.Task{}
	}
	p := build(task)
	p.Stream = id
	return p
}

// turn is the task's next turn, its last one, or any.
func (g *generator) turn(task *tasks.Task) string {
	candidates := []string{task.NextTurnID(), pick(g, turnIDs...)}
	if last := task.LastTurn(); last != nil {
		candidates = append(candidates, last.ID)
	}
	return pick(g, candidates...)
}

// action is the task's last action, any of its actions, or an id from
// toolu_0 to the newest: toolu_0 is never requested, so some steps aim at
// an action no task has.
func (g *generator) action(task *tasks.Task) string {
	candidates := []string{actionID(g.rand.IntN(g.actions + 1))}
	if len(task.Actions) > 0 {
		candidates = append(candidates, task.Actions[len(task.Actions)-1].ID, pick(g, task.Actions...).ID)
	}
	return pick(g, candidates...)
}

func actionID(n int) string { return "toolu_" + strconv.Itoa(n) }

// agent is the task's coordinator, the agent it was handed to, the one in
// its open turn, or any. Never "": no rule gets past an empty actor.
func (g *generator) agent(task *tasks.Task) string {
	candidates := []string{task.Coordinator, task.CurrentAgent(), pick(g, agentNames...)}
	if task.Handoff != nil {
		candidates = append(candidates, task.Handoff.To)
	}
	candidates = slices.DeleteFunc(candidates, func(name string) bool { return name == "" })
	return pick(g, candidates...)
}

// coordinator is the coordinator of the team assignee draws from, or any
// agent.
func (g *generator) coordinator() string {
	candidates := []string{pick(g, agentNames...)}
	if coordinator, ok := g.state.Roster().Coordinator(agents.DefaultTeam); ok {
		candidates = append(candidates, coordinator.Name)
	}
	return pick(g, candidates...)
}

// assignee is an agent of the coordinator's team that takes work, or any.
func (g *generator) assignee() string {
	candidates := []string{pick(g, agentNames...)}
	if assignees := g.state.Roster().Assignees(agents.DefaultTeam); len(assignees) > 0 {
		candidates = append(candidates, pick(g, assignees...).Name)
	}
	return pick(g, candidates...)
}

func (g *generator) agentOf(task *tasks.Task) events.Actor {
	return events.Actor{Kind: events.Agent, ID: g.agent(task)}
}

// createTask aims at the next task stream, the one the rules take, as often
// as at any.
func (g *generator) createTask() events.Proposal {
	p := g.onTask(func(*tasks.Task) events.Proposal {
		return events.Proposal{Actor: pick(g, you, you, g.agentActor()), Type: events.TaskCreated,
			Data: map[string]any{"title": "Random task", "repo": "/src/app", "coordinator": g.coordinator()}}
	})
	p.Stream = pick(g, g.state.NextTask(), p.Stream)
	return p
}

func (g *generator) worktreeReady() events.Proposal {
	return g.onTask(func(*tasks.Task) events.Proposal {
		return events.Proposal{Actor: worker, Type: events.WorktreeReady,
			Data: map[string]any{"path": "/w", "branch": "agentry/task", "port": 7901}}
	})
}

func (g *generator) assign() events.Proposal {
	return g.onTask(func(task *tasks.Task) events.Proposal {
		return events.Proposal{Actor: g.agentOf(task), Type: events.TaskAssigned,
			Data: map[string]any{"agent": g.assignee(), "instructions": "do it"}}
	})
}

func (g *generator) startTurn() events.Proposal {
	return g.onTask(func(task *tasks.Task) events.Proposal {
		turn := g.turn(task)
		return events.Proposal{Actor: pick(g, worker, g.anyActor()), Type: events.TurnStarted, Correlation: turn,
			Data: map[string]any{"agent": g.agent(task), "hash": "h-" + turn}}
	})
}

func (g *generator) endTurn() events.Proposal {
	return g.onTask(func(task *tasks.Task) events.Proposal {
		return events.Proposal{Actor: pick(g, worker, g.anyActor()), Type: events.TurnEnded, Correlation: g.turn(task),
			Data: map[string]any{"agent": g.agent(task), "report": pick(g, `{"action":"assign"}`, `{"action":"ask"}`, "Done.")}}
	})
}

func (g *generator) requestAction() events.Proposal {
	return g.onTask(func(task *tasks.Task) events.Proposal {
		g.actions++
		return events.Proposal{Actor: g.agentOf(task), Type: events.ActionRequested, Correlation: actionID(g.actions),
			Data: map[string]any{"tool": "Bash", "command": "ls"}}
	})
}

func (g *generator) decideAction() events.Proposal {
	return g.onTask(func(task *tasks.Task) events.Proposal {
		return events.Proposal{Actor: pick(g, policy, you, g.anyActor()), Correlation: g.action(task),
			Type: pick(g, events.ActionApproved, events.ActionDenied, events.ActionAsked), Data: map[string]any{"reason": "random"}}
	})
}

func (g *generator) finishAction() events.Proposal {
	return g.onTask(func(task *tasks.Task) events.Proposal {
		return events.Proposal{Actor: pick(g, worker, g.anyActor()), Type: events.ActionResult, Correlation: g.action(task)}
	})
}

func (g *generator) ask() events.Proposal {
	return g.onTask(func(task *tasks.Task) events.Proposal {
		return events.Proposal{Actor: pick(g, watchdog, g.agentOf(task)), Type: events.TaskNeedsYou,
			Data: map[string]any{"question": "Which branch?"}}
	})
}

func (g *generator) answer() events.Proposal {
	return g.onTask(func(*tasks.Task) events.Proposal {
		return events.Proposal{Actor: pick(g, you, g.anyActor()), Type: events.TaskAnswered, Data: map[string]any{"answer": "main"}}
	})
}

func (g *generator) finishTask() events.Proposal {
	// Not aimed at the coordinator: a task done early takes no more steps.
	return g.onTask(func(*tasks.Task) events.Proposal {
		return events.Proposal{Actor: g.anyActor(), Type: events.TaskDone, Data: map[string]any{"summary": "finished"}}
	})
}

// learn sets an agent's knowledge: as a full copy, as a patch on what the
// roster holds for it, or as a patch on a version it never held.
func (g *generator) learn() events.Proposal {
	name := pick(g, agentNames...)
	agent, _ := g.state.Roster().Get(name)
	agent.Name = name
	by, note := pick(g, you, g.agentActor()), "note "+strconv.Itoa(g.rand.IntN(5))
	switch g.rand.IntN(3) {
	case 0:
		return agent.Learned(by, note)
	case 1:
		return agent.Patching(by, agents.Diff(agent.Knowledge, agent.Knowledge+"; "+note))
	default:
		return agent.Patching(by, agents.Diff("a version it never held", note))
	}
}

func (g *generator) changeAgent() events.Proposal {
	agent := agents.Agent{Name: pick(g, agentNames...), Engine: "claude", Prompt: "Work.", Coordinates: g.rand.IntN(3) == 0}
	if g.rand.IntN(2) == 0 {
		agent.Workers = []string{pick(g, machineNames...)}
	}
	return agent.Proposal(pick(g, you, g.anyActor()), g.rand.IntN(2) == 0)
}

func (g *generator) renameAgent() events.Proposal {
	return agents.Agent{Name: pick(g, agentNames...)}.Renaming(you, pick(g, agentNames...))
}

func (g *generator) removeAgent() events.Proposal {
	return agents.Agent{Name: pick(g, agentNames...)}.Removal(pick(g, you, g.anyActor()))
}

func (g *generator) inviteWorker() events.Proposal {
	code, p := workers.Invite(you, time.Now())
	g.codes = append(g.codes, code)
	return p
}

func (g *generator) joinWorker() events.Proposal {
	_, p := workers.Join(pick(g, machineNames...), pick(g, g.codes...), workers.Machine{Host: "box", OS: "linux", Arch: "amd64"})
	return p
}

func (g *generator) connectWorker() events.Proposal {
	if g.rand.IntN(2) == 0 {
		return workers.Disconnected(pick(g, machineNames...))
	}
	return workers.Connected(pick(g, machineNames...), workers.Machine{Host: "box", OS: "linux", Arch: "amd64", Engines: []string{"claude"}})
}

func (g *generator) removeWorker() events.Proposal {
	if g.rand.IntN(2) == 0 {
		return workers.Left(pick(g, machineNames...))
	}
	return workers.Removal(pick(g, you, g.anyActor()), pick(g, machineNames...))
}

// next is one random step. Task steps come more often: a task needs many
// of them in order before its later rules can pass.
func (g *generator) next() events.Proposal {
	steps := []func() events.Proposal{
		g.createTask, g.worktreeReady, g.assign, g.assign, g.startTurn, g.startTurn, g.endTurn, g.endTurn,
		g.requestAction, g.requestAction, g.decideAction, g.decideAction, g.finishAction, g.finishAction, g.ask, g.answer, g.finishTask,
		g.learn, g.changeAgent, g.renameAgent, g.removeAgent,
		g.inviteWorker, g.joinWorker, g.connectWorker, g.removeWorker,
	}
	return pick(g, steps...)()
}

// seed is AGENTRY_SEED if set, so a failure can be rerun, else a fixed one.
func seed(t *testing.T) uint64 {
	text := os.Getenv("AGENTRY_SEED")
	if text == "" {
		return 1
	}
	seed, err := strconv.ParseUint(text, 10, 64)
	if err != nil {
		t.Fatalf("AGENTRY_SEED: %v", err)
	}
	return seed
}

// Test 4: random proposals, accepted or refused, through Propose. After
// every step (every equalEvery under -race) and at the end, the state
// equals a replay of the log: a refusal never reaches it.
func TestRandomProposalsKeepTheStateEqualToAReplay(t *testing.T) {
	const steps = 2000
	seed := seed(t)
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatalf("seeding %s: %v %v", p.Text("name"), rejection, err)
		}
	}
	g := &generator{rand: rand.New(rand.NewPCG(seed, seed)), state: state, codes: []string{"a-code-nobody-made"}}
	accepted, refused := 0, 0
	proposedOf, acceptedOf := map[events.Type]int{}, map[events.Type]int{}
	for i := range steps {
		p := g.next()
		proposedOf[p.Type]++
		_, rejection, err := s.Propose(p)
		if err != nil {
			t.Fatalf("seed %d, step %d: %v", seed, i+1, err)
		}
		if rejection == nil {
			accepted++
			acceptedOf[p.Type]++
		} else {
			refused++
		}
		if (i+1)%equalEvery != 0 && i+1 != steps {
			continue
		}
		if err := current.Equal(s, state); err != nil {
			t.Fatalf("seed %d, step %d (%s by %s, %s): %v", seed, i+1, p.Type, p.Actor, outcome(rejection), err)
		}
	}
	t.Logf("seed %d: %d accepted, %d refused; accepted by type %v", seed, accepted, refused, acceptedOf)
	if accepted < steps/10 || refused < steps/10 {
		t.Errorf("seed %d: %d accepted, %d refused; want at least %d of each", seed, accepted, refused, steps/10)
	}
	// The total hides a type that never gets past its rules: busy types fill it.
	for _, kind := range slices.Sorted(maps.Keys(proposedOf)) {
		if acceptedOf[kind] == 0 {
			t.Errorf("seed %d: none of %d %s proposals accepted", seed, proposedOf[kind], kind)
		}
	}
}

func outcome(rejection *events.Rejection) string {
	if rejection == nil {
		return "accepted"
	}
	return fmt.Sprintf("refused: %s", rejection.Reason)
}
