package rules

import (
	"slices"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// workerRule checks a proposal against the agents and the workers that exist.
type workerRule func(roster agents.Roster, machines workers.Roster, p events.Proposal) string

var workerRules = map[events.Type]workerRule{
	events.AgentCreated:       workersAreKnown,
	events.AgentChanged:       workersAreKnown,
	events.WorkerJoined:       allWorkers(workerNameIsFree, nameIsValidFor("worker"), codeIsValid),
	events.WorkerConnected:    workerExists,
	events.WorkerDisconnected: workerExists,
	events.WorkerRemoved:      allWorkers(workerExists, noAgentRunsOnIt),
}

func allWorkers(rules ...workerRule) workerRule {
	return func(roster agents.Roster, machines workers.Roster, p events.Proposal) string {
		for _, r := range rules {
			if reason := r(roster, machines, p); reason != "" {
				return reason
			}
		}
		return ""
	}
}

// workersAreKnown: an agent runs on machines that have joined, or on the
// one Agentry runs on (workers.Local), which is always there. None is that
// one too.
func workersAreKnown(_ agents.Roster, machines workers.Roster, p events.Proposal) string {
	for _, name := range p.List("workers") {
		if _, ok := machines.Get(name); ok || name == workers.Local {
			continue
		}
		if len(machines.Workers) == 0 {
			return "no worker called " + name + ": no machine has joined yet, so agents run on this one"
		}
		return "no worker called " + name + "; joined: " + strings.Join(machines.Names(), ", ")
	}
	return ""
}

func workerNameIsFree(_ agents.Roster, machines workers.Roster, p events.Proposal) string {
	name := p.Text("name")
	if name == workers.Local {
		return name + " is the name of the machine Agentry runs on; pick another"
	}
	if _, exists := machines.Get(name); exists {
		return "a worker called " + name + " has already joined"
	}
	return ""
}

// nameIsValidFor uses the rule for agents' names: a worker's name ends up in
// the trail and in the agents' settings the same way.
func nameIsValidFor(what string) workerRule {
	return func(_ agents.Roster, _ workers.Roster, p events.Proposal) string {
		if !agentName.MatchString(p.Text("name")) {
			return "a " + what + "'s name is lowercase letters, digits and dashes, starting with a letter: " + p.Text("name")
		}
		return ""
	}
}

func codeIsValid(_ agents.Roster, machines workers.Roster, p events.Proposal) string {
	if !machines.CanJoin(p.Text("code"), time.Now()) {
		return "that join code is unknown, used or expired; make a new one on the Workers page"
	}
	return ""
}

func workerExists(_ agents.Roster, machines workers.Roster, p events.Proposal) string {
	if _, exists := machines.Get(p.Text("name")); !exists {
		return "no worker called " + p.Text("name")
	}
	return ""
}

func noAgentRunsOnIt(roster agents.Roster, _ workers.Roster, p events.Proposal) string {
	var users []string
	for _, a := range roster {
		if slices.Contains(a.Workers, p.Text("name")) {
			users = append(users, a.Name)
		}
	}
	if len(users) > 0 {
		return strings.Join(users, ", ") + " can run on " + p.Text("name") + "; take it off their workers first"
	}
	return ""
}
