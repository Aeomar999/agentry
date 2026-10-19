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
	events.AgentCreated:    allOf(nameIsNew, nameIsValid, engineIsKnown, hasPrompt, colorIsKnown, columnIsKnown),
	events.AgentChanged:    allOf(agentExists, engineIsKnown, hasPrompt, colorIsKnown, columnIsKnown, keepsACoordinator),
	events.AgentRemoved:    allOf(agentExists, removalKeepsACoordinator),
	events.AgentLearned:    allOf(agentExists, onlyItsOwnKnowledge, patchFits),
	events.AgentRenamed:    allOf(agentExists, newNameIsFree),
	events.TaskCreated:     allOf(coordinatorExists),
	events.TaskAssigned:    allOf(assigneeExists),
	events.TaskTransferred: allOf(coordinatorExists), // the new coordinator, which must be one
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

func newNameIsFree(roster agents.Roster, p events.Proposal) string {
	to := p.Text("to")
	if !agentName.MatchString(to) {
		return "an agent's name is lowercase letters, digits and dashes, starting with a letter: " + to
	}
	if _, exists := roster.Get(to); exists {
		return "an agent called " + to + " already exists"
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

// keepsACoordinator stops a change that would leave a team with agents but
// no coordinator to send its tasks to: the agent's team, or the one it moves to.
func keepsACoordinator(roster agents.Roster, p events.Proposal) string {
	before, _ := roster.Get(p.Text("name"))
	after := slices.Clone(roster)
	for i := range after {
		if after[i].Name == before.Name {
			after[i].Coordinates, after[i].Team = p.Flag("coordinates"), p.Text("team")
		}
	}
	changed, _ := after.Get(before.Name)
	return teamsKeepCoordinators(after, before.TeamName(), changed.TeamName())
}

func teamsKeepCoordinators(after agents.Roster, teams ...string) string {
	for _, team := range teams {
		if _, ok := after.Coordinator(team); !ok && slices.Contains(after.Teams(), team) {
			return "this would leave the " + team + " team with no coordinator"
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
	switch column := p.Text("column"); column {
	case agents.Builds, agents.Reviews, agents.Ships:
	default:
		return `an agent's work counts as building (""), review ("review") or shipping ("ship"), not ` + column
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

// patchFits: a patch makes, from the knowledge the agent has now, the text
// whose hash it carries. One made from a version since replaced doesn't, and
// its writer sends a full copy instead.
func patchFits(roster agents.Roster, p events.Proposal) string {
	if !agents.IsPatch(p) {
		return ""
	}
	patch, err := agents.PatchOf(p)
	if err != nil {
		return err.Error()
	}
	agent, _ := roster.Get(p.Text("name"))
	if _, err := patch.Apply(agent.Knowledge); err != nil {
		return "this patch doesn't fit what " + agent.Name + " knows now: " + err.Error()
	}
	return ""
}

func removalKeepsACoordinator(roster agents.Roster, p events.Proposal) string {
	agent, _ := roster.Get(p.Text("name"))
	after := slices.DeleteFunc(slices.Clone(roster), func(a agents.Agent) bool { return a.Name == agent.Name })
	if len(after.Teams()) == 0 {
		return "this would leave no coordinator"
	}
	return teamsKeepCoordinators(after, agent.TeamName())
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
	// A coordinator hands work only to its own team.
	if coordinator, ok := roster.Get(p.Actor.ID); ok && coordinator.TeamName() != agent.TeamName() {
		return p.Text("agent") + " is in the " + agent.TeamName() + " team, not " + coordinator.TeamName()
	}
	return ""
}
