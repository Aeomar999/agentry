// Package agents is who can be handed work: each agent's name, prompt and
// engine. Agents are events in the log, not code, so the user can add and
// change them; the defaults are only what a new install starts with.
package agents

import (
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

type Agent struct {
	Name        string `json:"name"`
	Description string `json:"description"` // one line, shown to the coordinator when it chooses
	Prompt      string `json:"prompt"`      // the agent's standing instructions
	Engine      string `json:"engine"`      // claude | codex
	Model       string `json:"model"`       // "" means the engine's default
	Profile     string `json:"profile"`     // its policy profile in policy.toml; "" means the default profile
	Coordinates bool   `json:"coordinates"` // decides who works next, instead of working
}

// Proposal is the event that creates (or with changed set, changes) this agent.
func (a Agent) Proposal(by events.Actor, changed bool) events.Proposal {
	kind := events.AgentCreated
	if changed {
		kind = events.AgentChanged
	}
	return events.Proposal{Stream: events.AgentsStream, Actor: by, Type: kind, Data: map[string]any{
		"name": a.Name, "description": a.Description, "prompt": a.Prompt,
		"engine": a.Engine, "model": a.Model, "profile": a.Profile, "coordinates": a.Coordinates,
	}}
}

func fromEvent(e events.Event) Agent {
	return Agent{Name: e.Text("name"), Description: e.Text("description"), Prompt: e.Text("prompt"),
		Engine: e.Text("engine"), Model: e.Text("model"), Profile: e.Text("profile"), Coordinates: e.Flag("coordinates")}
}

// Roster is every agent, in the order they were created.
type Roster []Agent

func FromEvents(stream []events.Event) Roster {
	var roster Roster
	for _, e := range stream {
		switch e.Type {
		case events.AgentCreated:
			roster = append(roster, fromEvent(e))
		case events.AgentChanged:
			if i := roster.index(e.Text("name")); i >= 0 {
				roster[i] = fromEvent(e)
			}
		}
	}
	return roster
}

func (r Roster) Get(name string) (Agent, bool) {
	if i := r.index(name); i >= 0 {
		return r[i], true
	}
	return Agent{}, false
}

// Coordinator is the first agent that coordinates, the one new tasks go to.
func (r Roster) Coordinator() (Agent, bool) {
	if i := slices.IndexFunc(r, func(a Agent) bool { return a.Coordinates }); i >= 0 {
		return r[i], true
	}
	return Agent{}, false
}

// Workers are the agents a coordinator can hand work to.
func (r Roster) Workers() Roster {
	var workers Roster
	for _, a := range r {
		if !a.Coordinates {
			workers = append(workers, a)
		}
	}
	return workers
}

func (r Roster) index(name string) int {
	return slices.IndexFunc(r, func(a Agent) bool { return a.Name == name })
}
