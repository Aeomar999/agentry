package rules

import (
	"regexp"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
)

// KnownEngines are the engines an agent may name. The engines package has a
// test that keeps this list and its adapters the same.
var KnownEngines = []string{"claude", "codex"}

// An agent's name ends up in file names, environment variables and the trail.
var agentName = regexp.MustCompile(`^[a-z][a-z0-9-]{0,30}$`)

// rosterRule checks a proposal against the agents that exist.
type rosterRule func(roster agents.Roster, p events.Proposal) string

var rosterRules = map[events.Type]rosterRule{
	events.AgentCreated: allOf(nameIsNew, nameIsValid, engineIsKnown, hasPrompt, workersAreKnown, colorIsKnown, columnIsKnown),
	events.AgentChanged: allOf(agentExists, engineIsKnown, hasPrompt, workersAreKnown, colorIsKnown, columnIsKnown, keepsACoordinator),
	events.AgentRemoved: allOf(agentExists, removalKeepsACoordinator),
	events.AgentLearned: allOf(agentExists, onlyItsOwnKnowledge),
	events.TaskCreated:  allOf(coordinatorExists),
	events.TaskAssigned: allOf(assigneeExists),
}

func allOf(rules ...rosterRule) rosterRule {
	return func(roster agents.Roster, p events.Proposal) string {
		for _, r := range rules {
			if reason := r(roster, p); reason != "" {
				return reason
			}
		}
		return ""
	}
}

func nameIsNew(roster agents.Roster, p events.Proposal) string {
	if _, exists := roster.Get(p.Text("name")); exists {
		return "an agent called " + p.Text("name") + " already exists"
	}
	return ""
}

func nameIsValid(_ agents.Roster, p events.Proposal) string {
	if !agentName.MatchString(p.Text("name")) {
		return "an agent's name is lowercase letters, digits and dashes, starting with a letter: " + p.Text("name")
	}
	return ""
}

func engineIsKnown(_ agents.Roster, p events.Proposal) string {
	if !slices.Contains(KnownEngines, p.Text("engine")) {
		return "unknown engine " + p.Text("engine") + "; known: " + strings.Join(KnownEngines, ", ")
	}
	return ""
}

func hasPrompt(_ agents.Roster, p events.Proposal) string {
	if strings.TrimSpace(p.Text("prompt")) == "" {
		return "an agent needs a prompt"
	}
	return ""
}

func agentExists(roster agents.Roster, p events.Proposal) string {
	if _, exists := roster.Get(p.Text("name")); !exists {
		return "no agent called " + p.Text("name")
	}
	return ""
}

// keepsACoordinator stops a change that would leave no agent to send tasks to.
func keepsACoordinator(roster agents.Roster, p events.Proposal) string {
	if p.Flag("coordinates") {
		return ""
	}
	for _, a := range roster {
		if a.Coordinates && a.Name != p.Text("name") {
			return ""
		}
	}
	return "this would leave no coordinator"
}

// KnownWorkers are the machines an agent may name. None yet: workers come
// with T-022, and until then every agent runs on the machine Agentry runs on.
var KnownWorkers []string

func workersAreKnown(_ agents.Roster, p events.Proposal) string {
	for _, worker := range p.List("workers") {
		if !slices.Contains(KnownWorkers, worker) {
			if len(KnownWorkers) == 0 {
				return "no worker called " + worker + ": there are no workers yet, so agents run on this machine"
			}
			return "no worker called " + worker + "; known: " + strings.Join(KnownWorkers, ", ")
		}
	}
	return ""
}

func colorIsKnown(_ agents.Roster, p events.Proposal) string {
	if color := p.Text("color"); color != "" && !slices.Contains(agents.Colors, color) {
		return "unknown colour " + color + "; known: " + strings.Join(agents.Colors, ", ")
	}
	return ""
}

func columnIsKnown(_ agents.Roster, p events.Proposal) string {
	if column := p.Text("column"); column != "" && column != "review" {
		return `an agent's work counts as In progress ("") or Review ("review"), not ` + column
	}
	return ""
}

// onlyItsOwnKnowledge: an agent changes its own knowledge, never another's.
func onlyItsOwnKnowledge(_ agents.Roster, p events.Proposal) string {
	if p.Actor.Kind == events.Agent && p.Actor.ID != p.Text("name") {
		return p.Actor.ID + " can't change what " + p.Text("name") + " knows"
	}
	return ""
}

func removalKeepsACoordinator(roster agents.Roster, p events.Proposal) string {
	agent, _ := roster.Get(p.Text("name"))
	if !agent.Coordinates {
		return ""
	}
	for _, a := range roster {
		if a.Coordinates && a.Name != agent.Name {
			return ""
		}
	}
	return "this would leave no coordinator"
}

func coordinatorExists(roster agents.Roster, p events.Proposal) string {
	agent, exists := roster.Get(p.Text("coordinator"))
	switch {
	case !exists:
		return "no agent called " + p.Text("coordinator")
	case !agent.Coordinates:
		return p.Text("coordinator") + " is not a coordinator"
	}
	return ""
}

func assigneeExists(roster agents.Roster, p events.Proposal) string {
	agent, exists := roster.Get(p.Text("agent"))
	switch {
	case !exists:
		return "no agent called " + p.Text("agent")
	case agent.Coordinates:
		return p.Text("agent") + " is a coordinator and doesn't take work"
	}
	return ""
}
