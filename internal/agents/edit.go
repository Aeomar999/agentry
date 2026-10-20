package agents

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

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
		if i := after.index(e.Name); e.Coordinates && e.Was != lead.Name && i >= 0 { // a log edited by hand may keep an agent the team didn't have
			after[i].Template = lead.Template
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

// EditTeam is the edit that leaves team, as the roster names it, called to
// and made of rows, in the team panel's order. Each row's text is trimmed;
// a row named as one of the team's agents, and no other row was it, is that
// agent; the team's agents no row was are removed; and a new agent with no
// prompt gets a default one, as an agent of a custom team does.
func EditTeam(roster Roster, team, to string, rows []EditedAgent) TeamEdit {
	plain := make([]Agent, len(rows))
	for i, row := range rows {
		plain[i] = Agent{Name: row.Name, Description: row.Description, Prompt: row.Prompt, Engine: row.Engine, Model: row.Model,
			Profile: row.Profile, Coordinates: row.Coordinates, Color: row.Color, Column: row.Column}
	}
	to, plain = trimmed(to, plain)
	edit := TeamEdit{Team: team, To: to}
	for i, agent := range plain {
		row := agent.Edited()
		row.Was = strings.TrimSpace(rows[i].Was)
		if was, ok := roster.Get(row.Name); row.Was == "" && ok && was.TeamName() == team &&
			!slices.ContainsFunc(rows, func(r EditedAgent) bool { return strings.TrimSpace(r.Was) == row.Name }) {
			row.Was = row.Name
		}
		if row.Was == "" && row.Prompt == "" {
			row.Prompt = defaultPrompt(Agent{Name: row.Name, Description: row.Description, Coordinates: row.Coordinates, Column: row.Column, Team: to})
		}
		edit.Agents = append(edit.Agents, row)
	}
	for _, a := range roster {
		if a.TeamName() == team && !slices.ContainsFunc(edit.Agents, func(e EditedAgent) bool { return e.Was == a.Name }) {
			edit.Removed = append(edit.Removed, a.Name)
		}
	}
	return edit
}

// Renames are the names agents had, each to the name the agent has now, as
// a fold of the agents stream: what a task still naming an agent by a name
// it had should name instead. A removed agent's names go with it.
type Renames map[string]string

// RenamesIn folds the agents stream into its renames.
func RenamesIn(stream []events.Event) Renames {
	renames := Renames{}
	for _, e := range stream {
		renames.Apply(e)
	}
	return renames
}

// Apply follows one accepted event of the agents stream. The renames of a
// team edit happen at once, so two agents can swap names.
func (r Renames) Apply(e events.Event) {
	switch e.Type {
	case events.AgentRenamed:
		r.move(map[string]string{e.Text("name"): e.Text("to")})
	case events.TeamEdited:
		edit, _ := TeamEditOf(e.Proposal) // the rules accepted it as one
		moves := map[string]string{}
		for _, a := range edit.Agents {
			if a.Was != "" && a.Was != a.Name {
				moves[a.Was] = a.Name
			}
		}
		r.move(moves)
		for _, name := range edit.Removed {
			r.forget(name)
		}
	case events.AgentRemoved:
		r.forget(e.Text("name"))
	}
}

func (r Renames) move(moves map[string]string) {
	for old, now := range r {
		if to, ok := moves[now]; ok {
			r[old] = to
		}
	}
	for was, to := range moves {
		r[was] = to
	}
}

func (r Renames) forget(name string) {
	for old, now := range r {
		if now == name {
			delete(r, old)
		}
	}
	delete(r, name)
}
