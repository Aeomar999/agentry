package agents

import (
	"embed"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

//go:embed defaults/*.md defaults/*/*.md
var defaultPrompts embed.FS

// defaults are the agents a new install starts with: the Software team (the
// first of Templates). Each prompt is defaults/<name>.md. After the first
// start they are ordinary agents in the log; Upgrades brings the ones you
// have never changed up to date.
var defaults = []Agent{
	{Name: "tech-lead", Coordinates: true, Engine: "claude", Profile: "read-only", Color: "orange",
		Description: "Takes every software task, plans it in small steps, decides who works on it next, and when it is done"},
	{Name: "engineer", Engine: "claude", Profile: "developer", Color: "blue",
		Description: "Writes and changes code on the task branch, with tests, and commits it"},
	{Name: "reviewer", Engine: "claude", Profile: "read-only", Color: "violet", Column: Reviews,
		Description: "Reviews the task branch's changes for bugs, clarity and fit; changes nothing"},
	{Name: "tester", Engine: "claude", Profile: "developer", Color: "teal", Column: Reviews,
		Description: "Runs the tests, adds missing ones, and tries the change for real"},
	{Name: "deployer", Engine: "claude", Profile: "developer", Color: "green", Column: Ships,
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
	for _, agent := range Templates[0].Prompted() {
		proposals = append(proposals, agent.Proposal(seeder, false))
	}
	return proposals
}

// Upgrades fill in what default agents created by an older Agentry lack (a
// policy profile, a colour, the board column their work counts as) and bring
// their prompts up to date. Only agents you have never changed: an empty
// field you chose (the default profile) stays as you left it.
func Upgrades(current Roster) []events.Proposal {
	upgrader := events.Actor{Kind: events.System, ID: "defaults"}
	var proposals []events.Proposal
	for _, shipped := range Templates[0].Prompted() {
		agent, exists := current.Get(shipped.Name)
		if !exists || !agent.untouched {
			continue
		}
		filled := false
		for _, field := range []struct {
			value   *string
			shipped string
		}{
			{&agent.Profile, shipped.Profile}, {&agent.Color, shipped.Color}, {&agent.Column, shipped.Column},
		} {
			if *field.value == "" && field.shipped != "" {
				*field.value, filled = field.shipped, true
			}
		}
		if agent.Prompt != shipped.Prompt {
			agent.Prompt, filled = shipped.Prompt, true
		}
		if filled {
			proposals = append(proposals, agent.Proposal(upgrader, true))
		}
	}
	return proposals
}
