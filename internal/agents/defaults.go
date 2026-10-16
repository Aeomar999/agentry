package agents

import (
	"embed"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

//go:embed defaults/*.md
var defaultPrompts embed.FS

// defaults are the agents a new install starts with. Each prompt is
// defaults/<name>.md. After the first start they are ordinary agents in the
// log, and changing this list changes nothing for an existing install.
var defaults = []Agent{
	{Name: "coordinator", Coordinates: true, Engine: "claude", Profile: "read-only",
		Description: "Takes every task, decides who works on it next, and when it is done"},
	{Name: "engineer", Engine: "claude", Profile: "developer",
		Description: "Writes and changes code on the task branch, with tests, and commits it"},
	{Name: "reviewer", Engine: "claude", Profile: "read-only",
		Description: "Reviews the task branch's changes for bugs, clarity and fit; changes nothing"},
	{Name: "tester", Engine: "claude", Profile: "developer",
		Description: "Runs the tests, adds missing ones, and tries the change for real"},
	{Name: "deployer", Engine: "claude", Profile: "developer",
		Description: "Merges the finished task branch into dev and pushes it (the push asks you first)"},
}

// SeedProposals are the events that create the default agents, when there are
// no agents at all yet.
func SeedProposals(current Roster) []events.Proposal {
	if len(current) > 0 {
		return nil
	}
	seeder := events.Actor{Kind: events.System, ID: "defaults"}
	var proposals []events.Proposal
	for _, agent := range defaults {
		prompt, err := defaultPrompts.ReadFile("defaults/" + agent.Name + ".md")
		if err != nil {
			panic("missing default prompt: " + err.Error()) // caught by the tests
		}
		agent.Prompt = strings.TrimSpace(string(prompt))
		proposals = append(proposals, agent.Proposal(seeder, false))
	}
	return proposals
}

// ProfileUpgrades fill in the policy profile of default agents created
// before agents had profiles. They never change a profile already set.
func ProfileUpgrades(current Roster) []events.Proposal {
	upgrader := events.Actor{Kind: events.System, ID: "defaults"}
	var proposals []events.Proposal
	for _, shipped := range defaults {
		if agent, exists := current.Get(shipped.Name); exists && agent.Profile == "" {
			agent.Profile = shipped.Profile
			proposals = append(proposals, agent.Proposal(upgrader, true))
		}
	}
	return proposals
}
