package machines

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// Fleet is every machine agents can work on now: this one, always, and
// each worker while it is connected.
type Fleet struct {
	local  Machine
	mu     sync.Mutex
	online map[string]Online
	lost   map[string]time.Time // when each worker that was online went offline
}

// OfflineGrace is how long a worker that went offline is taken to still be
// running what it ran. Its sessions don't stop when its link drops, and it
// dials back in seconds, as after a network blip or the server restarting;
// meanwhile the watchdog mustn't take its agents for crashed.
const OfflineGrace = 30 * time.Second

// Online is a connected worker: the machine, and what it said it has.
type Online struct {
	Machine
	Engines []string
}

func NewFleet(local Machine) *Fleet {
	return &Fleet{local: local, online: map[string]Online{}, lost: map[string]time.Time{}}
}

// Connect adds a worker that came online. If it was already connected (a
// new connection before the old one was noticed gone), the new one wins.
func (f *Fleet) Connect(m Machine, engines []string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.online[m.Name()] = Online{Machine: m, Engines: engines}
}

// Disconnect removes the worker, unless a newer connection replaced it,
// and reports whether it did.
func (f *Fleet) Disconnect(m Machine) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.online[m.Name()].Machine != m {
		return false
	}
	delete(f.online, m.Name())
	f.lost[m.Name()] = time.Now()
	return true
}

// Expect says a worker was online before Agentry started, so it gets the
// same grace as one that just went offline while it dials back in.
func (f *Fleet) Expect(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lost[name] = time.Now()
}

// Get is the named machine, if it is here now.
func (f *Fleet) Get(name string) (Machine, bool) {
	if IsLocal(name) {
		return f.local, true
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	online, ok := f.online[name]
	return online.Machine, ok
}

// Of is the machine a task's work is on (named by its worktree), or this
// one if it has none yet. A worker that is offline is a machine that has
// nothing running and can't be reached, so the watchdog and the page see
// that rather than a missing machine.
func (f *Fleet) Of(worker string) Machine {
	if m, ok := f.Get(worker); ok {
		return m
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return Offline{Worker: worker, Since: f.lost[worker]}
}

// For picks the machine an agent works on next. An agent with no workers
// works here. Otherwise it is the first of its workers that is online and
// has its engine, but the one the task's work is on already, if that is one
// of them, so the work moves only when it must.
func (f *Fleet) For(agent agents.Agent, current string) (Machine, error) {
	if len(agent.Workers) == 0 {
		return f.local, nil
	}
	if IsLocal(current) {
		current = workers.Local
	}
	choices := agent.Workers
	if slices.Contains(choices, current) {
		others := slices.DeleteFunc(slices.Clone(choices), func(name string) bool { return name == current })
		choices = append([]string{current}, others...)
	}
	var reasons []string
	for _, name := range choices {
		m, ok := f.Get(name)
		switch {
		case !ok:
			reasons = append(reasons, name+" is offline")
		case !f.has(name, agent.Engine):
			reasons = append(reasons, name+" has no "+agent.Engine)
		default:
			return m, nil
		}
	}
	return nil, fmt.Errorf("%s can't run anywhere now: %s", agent.Name, strings.Join(reasons, ", "))
}

// WaitFor is For, but if every machine the agent may use is offline and one
// of them went offline less than OfflineGrace ago (or Agentry has just
// started), it waits for that one to dial back in before giving up.
func (f *Fleet) WaitFor(ctx context.Context, agent agents.Agent, current string) (Machine, error) {
	for {
		m, err := f.For(agent, current)
		if err == nil || !f.comingBack(agent.Workers) {
			return m, err
		}
		select {
		case <-ctx.Done():
			return nil, err
		case <-time.After(250 * time.Millisecond):
		}
	}
}

// comingBack says whether any of the named workers is offline but within
// its grace, so may reconnect any moment.
func (f *Fleet) comingBack(names []string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, name := range names {
		if _, online := f.online[name]; !online && time.Since(f.lost[name]) < OfflineGrace {
			return true
		}
	}
	return false
}

// has says whether a machine has an engine. This one is taken to: it is
// where Agentry was set up.
func (f *Fleet) has(name, engine string) bool {
	if IsLocal(name) {
		return true
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return (workers.Worker{Engines: f.online[name].Engines}).Has(engine)
}

// WaitOn is the named machine, for an agent the coordinator sent there: it
// must be online and have the agent's engine. One that went offline within
// OfflineGrace is waited for, as in WaitFor.
func (f *Fleet) WaitOn(ctx context.Context, agent agents.Agent, name string) (Machine, error) {
	for {
		m, online := f.Get(name)
		switch {
		case online && f.has(name, agent.Engine):
			return m, nil
		case online:
			return nil, fmt.Errorf("%s has no %s, which %s runs on", name, agent.Engine, agent.Name)
		case !f.comingBack([]string{name}):
			return nil, fmt.Errorf("%s is offline", name)
		}
		select {
		case <-ctx.Done():
			return nil, fmt.Errorf("%s is offline", name)
		case <-time.After(250 * time.Millisecond):
		}
	}
}
