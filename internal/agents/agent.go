// Package agents is who can be handed work: each agent's name, prompt,
// engine and the workers it may run on. Agents are events in the log, not code, so the user can add and
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
	// Team is the group it works in, such as Software or Writing: a task
	// goes to one team's coordinator, which hands it only to that team's
	// agents. "" is DefaultTeam, the team agents had before there were teams.
	Team string `json:"team"`
	// Workers are the machines it may run on. None means the machine Agentry
	// runs on, which is always there; workers are other machines (T-022).
	Workers []string `json:"workers"`
	// Color tells it apart on the page; one of Colors, or "" for the next free one.
	Color string `json:"color"`
	// Column is what its work counts as, and where a task shows on the
	// board while it has it: Builds (In progress), Reviews (Review), or Ships
	// (Shipping).
	Column string `json:"column"`

	// Knowledge is what the agent has learned as it works, in its own words
	// and yours; part of every brief. Changed only by agent.learned, so
	// editing the agent's settings never touches it.
	Knowledge string `json:"knowledge"`

	untouched bool // still as Agentry wrote it: you have never changed it
}

// Learned is the event that sets the agent's knowledge to knowledge.
func (a Agent) Learned(by events.Actor, knowledge string) events.Proposal {
	return events.Proposal{Stream: events.AgentsStream, Actor: by, Type: events.AgentLearned,
		Data: map[string]any{"name": a.Name, "knowledge": knowledge}}
}

// What an agent's work counts as. Builders change the work; reviewers
// check it, and a task can't be done until they approve its last change;
// shippers deploy or publish checked work without changing it.
const (
	Builds  = ""
	Reviews = "review"
	Ships   = "ship"
)

// Colors an agent can have. The page has a shade of each for both themes.
var Colors = []string{"blue", "violet", "teal", "green", "amber", "orange", "red", "pink", "slate"}

// Proposal is the event that creates (or with changed set, changes) this agent.
func (a Agent) Proposal(by events.Actor, changed bool) events.Proposal {
	kind := events.AgentCreated
	if changed {
		kind = events.AgentChanged
	}
	return events.Proposal{Stream: events.AgentsStream, Actor: by, Type: kind, Data: map[string]any{
		"name": a.Name, "description": a.Description, "prompt": a.Prompt,
		"engine": a.Engine, "model": a.Model, "profile": a.Profile, "coordinates": a.Coordinates,
		"workers": a.workers(), "color": a.Color, "column": a.Column, "team": a.Team,
	}}
}

// Renaming is the event that gives this agent a new name.
func (a Agent) Renaming(by events.Actor, to string) events.Proposal {
	return events.Proposal{Stream: events.AgentsStream, Actor: by, Type: events.AgentRenamed, Data: map[string]any{"name": a.Name, "to": to}}
}

// Removal is the event that removes this agent.
func (a Agent) Removal(by events.Actor) events.Proposal {
	return events.Proposal{Stream: events.AgentsStream, Actor: by, Type: events.AgentRemoved, Data: map[string]any{"name": a.Name}}
}

func (a Agent) workers() []string {
	if a.Workers == nil {
		return []string{}
	}
	return a.Workers
}

func fromEvent(e events.Event) Agent {
	return Agent{Name: e.Text("name"), Description: e.Text("description"), Prompt: e.Text("prompt"),
		Engine: e.Text("engine"), Model: e.Text("model"), Profile: e.Text("profile"), Coordinates: e.Flag("coordinates"),
		Workers: e.List("workers"), Color: e.Text("color"), Column: e.Text("column"), Team: e.Text("team"), untouched: e.Actor.Kind == events.System}
}

// Roster is every agent, in the order they were created.
type Roster []Agent

func FromEvents(stream []events.Event) Roster {
	var roster Roster
	for _, e := range stream {
		roster.Apply(e)
	}
	return roster
}

// Next is the roster after e, built on a copy: r itself is left as it was.
// Copying the slice is enough, as Apply replaces an agent's Workers whole.
func (r Roster) Next(e events.Event) Roster {
	r = slices.Clone(r)
	r.Apply(e)
	return r
}

// Apply changes the roster in place by one accepted event of its stream.
func (r *Roster) Apply(e events.Event) {
	roster := *r
	switch e.Type {
	case events.AgentCreated:
		roster = append(roster, fromEvent(e))
	case events.AgentChanged:
		if i := roster.index(e.Text("name")); i >= 0 {
			knowledge := roster[i].Knowledge
			roster[i] = fromEvent(e)
			roster[i].Knowledge = knowledge
		}
	case events.AgentRenamed:
		if i := roster.index(e.Text("name")); i >= 0 {
			roster[i].Name = e.Text("to")
		}
	case events.AgentLearned:
		if i := roster.index(e.Text("name")); i >= 0 {
			roster[i].Knowledge = e.Text("knowledge")
		}
	case events.AgentRemoved:
		if i := roster.index(e.Text("name")); i >= 0 {
			roster = slices.Delete(roster, i, i+1)
		}
	}
	*r = roster
}

func (r Roster) Get(name string) (Agent, bool) {
	if i := r.index(name); i >= 0 {
		return r[i], true
	}
	return Agent{}, false
}

// DefaultTeam is the team of agents that name none.
const DefaultTeam = "Software"

// TeamName is the agent's team, DefaultTeam when it names none.
func (a Agent) TeamName() string {
	if a.Team == "" {
		return DefaultTeam
	}
	return a.Team
}

// Teams are the teams the agents are in, in the order the first agent of
// each was created.
func (r Roster) Teams() []string {
	var teams []string
	for _, a := range r {
		if !slices.Contains(teams, a.TeamName()) {
			teams = append(teams, a.TeamName())
		}
	}
	return teams
}

// Coordinator is the coordinator of team, the one its tasks go to: the first
// of its agents that coordinates. With team "", the first team's.
func (r Roster) Coordinator(team string) (Agent, bool) {
	if team == "" && len(r.Teams()) > 0 {
		team = r.Teams()[0]
	}
	if i := slices.IndexFunc(r, func(a Agent) bool { return a.Coordinates && a.TeamName() == team }); i >= 0 {
		return r[i], true
	}
	return Agent{}, false
}

// Assignees are the agents of team a coordinator can hand work to.
func (r Roster) Assignees(team string) Roster {
	var assignees Roster
	for _, a := range r {
		if !a.Coordinates && a.TeamName() == team {
			assignees = append(assignees, a)
		}
	}
	return assignees
}

func (r Roster) index(name string) int {
	return slices.IndexFunc(r, func(a Agent) bool { return a.Name == name })
}
