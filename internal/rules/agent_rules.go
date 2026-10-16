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
	events.AgentCreated: allOf(nameIsNew, nameIsValid, engineIsKnown, hasPrompt),
	events.AgentChanged: allOf(agentExists, engineIsKnown, hasPrompt, keepsACoordinator),
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
