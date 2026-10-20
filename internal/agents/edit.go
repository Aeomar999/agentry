package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// A team edit saves a whole team at once, as the team panel edits it: its
// name, and its agents renamed, changed, added and removed, in one event, so
// the roster has all of it or none of it. What an agent knows and where it
// runs aren't part of it, and stay with the agent through a rename.

// TeamEdit is the team, as the roster names it, after an edit: To is its
// name, Agents every agent it keeps or adds, Removed those it leaves out.
type TeamEdit struct {
	Team    string        `json:"team"`
	To      string        `json:"to"`
	Agents  []EditedAgent `json:"agents"`
	Removed []string      `json:"removed"`
}

// EditedAgent is an agent as a team edit leaves it: what the panel edits of
// it, and Was, its name before the edit, "" for a new agent.
type EditedAgent struct {
	Was         string `json:"was"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
	Engine      string `json:"engine"`
	Model       string `json:"model"`
	Profile     string `json:"profile"`
	Coordinates bool   `json:"coordinates"`
	Color       string `json:"color"`
	Column      string `json:"column"`
}

// Proposal is the event that edits the team.
func (t TeamEdit) Proposal(by events.Actor) events.Proposal {
	removed := t.Removed
	if removed == nil {
		removed = []string{}
	}
	return events.Proposal{Stream: events.AgentsStream, Actor: by, Type: events.TeamEdited, Data: map[string]any{
		"team": t.Team, "to": t.To, "agents": t.Agents, "removed": removed,
	}}
}

// TeamEditOf reads a team.edited, whether a proposal not yet written holds
// its agents or the log gave them back as JSON.
func TeamEditOf(p events.Proposal) (TeamEdit, error) {
	encoded, err := json.Marshal(p.Data)
	if err != nil {
		return TeamEdit{}, err
	}
	var edit TeamEdit
	return edit, json.Unmarshal(encoded, &edit)
}

// Edited is a's definition as a team edit carries it, with no Was.
func (a Agent) Edited() EditedAgent {
	return EditedAgent{Name: a.Name, Description: a.Description, Prompt: a.Prompt, Engine: a.Engine, Model: a.Model,
		Profile: a.Profile, Coordinates: a.Coordinates, Color: a.Color, Column: a.Column}
}

// TeamVersion is the version of team as the team panel edits it: a hash of
// its name and its agents' definitions, in the roster's order. Any change
// to them, wherever it was made, changes it; an agent learning, or moving
// to another machine, doesn't.
func (r Roster) TeamVersion(team string) string {
	var members []EditedAgent
	for _, a := range r {
		if a.TeamName() == team {
			members = append(members, a.Edited())
		}
	}
	encoded, _ := json.Marshal(struct {
		Team   string
		Agents []EditedAgent
	}{team, members})
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:8])
}

// edited is the roster after edit, as the rules accepted it. Kept agents
// stay where they were, with their knowledge, workers and template; new
// ones go last. A coordinator other than the one before takes the template
// the team was made from, as a team.coordinator_moved gives it.
func (r Roster) edited(edit TeamEdit) Roster {
	lead, _ := r.Coordinator(edit.Team)
	newcomers := edit.To
	var after Roster
	for _, a := range r {
		if a.TeamName() != edit.Team {
			after = append(after, a)
			continue
		}
		team := edit.To
		if edit.To == edit.Team {
			team, newcomers = a.Team, a.Team // the default team's agents may name none
		}
		if i := slices.IndexFunc(edit.Agents, func(e EditedAgent) bool { return e.Was == a.Name }); i >= 0 {
			after = append(after, a.editedTo(edit.Agents[i], team))
		}
	}
	for _, e := range edit.Agents {
		if e.Was == "" {
			after = append(after, Agent{}.editedTo(e, newcomers))
		}
	}
	for _, e := range edit.Agents {
		if e.Coordinates && e.Was != lead.Name {
			after[after.index(e.Name)].Template = lead.Template
		}
	}
	return after
}

// editedTo is a as e defines it, in team, keeping what a team edit doesn't
// touch. An agent Agentry wrote stays untouched while its definition does.
func (a Agent) editedTo(e EditedAgent, team string) Agent {
	definition := e
	definition.Was = ""
	untouched := a.untouched && a.Edited() == definition
	a.Name, a.Description, a.Prompt, a.Engine, a.Model = e.Name, e.Description, e.Prompt, e.Engine, e.Model
	a.Profile, a.Coordinates, a.Color, a.Column = e.Profile, e.Coordinates, e.Color, e.Column
	a.Team, a.untouched = team, untouched
	return a
}
