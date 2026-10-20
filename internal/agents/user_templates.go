package agents

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// UserTemplate is a team you saved as a template of your own: its agents
// as they were when you saved it, kept until you remove it. Like a built-in
// Template, adding it makes ordinary agents.
type UserTemplate struct {
	ID          string    `json:"id"`
	Team        string    `json:"team"` // what a team made from it is called: the template's name
	Description string    `json:"description"`
	Agents      []Agent   `json:"agents"`
	From        string    `json:"from,omitempty"`    // the team it was saved from; a template saved before Agentry kept it has none
	BasedOn     string    `json:"basedOn,omitempty"` // the built-in template its team came from, if any, whose features its teams keep
	SavedAt     time.Time `json:"savedAt,omitzero"`  // when it was last saved, as the log has it
}

// Template is the saved template as one AddTeam adds, with the prompts
// you saved. Lookups by ID or team, as the Visionaries' are, still find only
// the built-in templates.
func (u UserTemplate) Template() Template {
	return Template{ID: u.ID, Team: u.Team, Description: u.Description, Agents: u.Agents, Given: true}
}

// SaveTemplate is the event that saves team's agents, as they are now, as
// the template id, whose teams are called name. It keeps what makes each agent and drops what is this
// install's or this team's alone: its knowledge, the machines it runs on
// (which another install, or this one later, may not have) and its team
// and template, which adding the template sets again. It is based on the
// built-in template the team is linked to, directly or through one of
// yours.
func SaveTemplate(roster Roster, yours UserTemplates, removed RemovedTemplates, team, name, id, description string, by events.Actor) (events.Proposal, error) {
	var members []Agent
	for _, a := range roster {
		if a.TeamName() == team {
			members = append(members, Agent{Name: a.Name, Description: a.Description, Prompt: a.Prompt,
				Engine: a.Engine, Model: a.Model, Profile: a.Profile, Coordinates: a.Coordinates,
				Color: a.Color, Column: a.Column})
		}
	}
	if len(members) == 0 {
		return events.Proposal{}, errors.New("there is no team " + team)
	}
	saving := UserTemplate{ID: id, Team: name, Description: description, Agents: members, From: team,
		BasedOn: baseOf(roster.TemplateOf(team), yours, removed)}.Save(by)
	saving.Data["new"] = true // never replacing one: the rules refuse a name or id of yours
	return saving, nil
}

// baseOf is the built-in template a team linked to template comes from:
// template itself when it is built in, the base of yours when it is yours,
// live or removed, and "" for none.
func baseOf(template string, yours UserTemplates, removed RemovedTemplates) string {
	if _, builtIn := TemplateWithID(template); builtIn {
		return template
	}
	if saved, ok := yours.WithID(template); ok {
		return saved.BasedOn
	}
	return removed[template]
}

// Save is the event that saves u, as SaveTemplate's does: the rules check
// it as one, wherever u came from. When it was saved is the event's own.
func (u UserTemplate) Save(by events.Actor) events.Proposal {
	data := map[string]any{"id": u.ID, "team": u.Team, "description": u.Description, "agents": u.Agents}
	if u.From != "" {
		data["from"] = u.From
	}
	if u.BasedOn != "" {
		data["basedOn"] = u.BasedOn
	}
	return events.Proposal{Stream: events.TemplatesStream, Actor: by, Type: events.TemplateSaved, Data: data}
}

// RemoveTemplate is the event that removes the template id. The teams made
// from it stay.
func RemoveTemplate(id string, by events.Actor) events.Proposal {
	return events.Proposal{Stream: events.TemplatesStream, Actor: by, Type: events.TemplateRemoved, Data: map[string]any{"id": id}}
}

// UserTemplates are the templates you saved and haven't removed, in the
// order you first saved them. Saving one again keeps its place.
type UserTemplates []UserTemplate

func UserTemplatesFromEvents(stream []events.Event) UserTemplates {
	var t UserTemplates
	for _, e := range stream {
		t.Apply(e)
	}
	return t
}

// Next is the templates after e, built on a copy: t itself is left as it
// was. Copying the slice is enough, as Apply replaces a template whole.
func (t UserTemplates) Next(e events.Event) UserTemplates {
	t = slices.Clone(t)
	t.Apply(e)
	return t
}

// Apply changes the templates in place by one accepted event of their stream.
func (t *UserTemplates) Apply(e events.Event) {
	id := e.Text("id")
	switch e.Type {
	case events.TemplateSaved:
		members, _ := TemplateAgents(e.Data) // the rules accepted them as agents
		saved := UserTemplate{ID: id, Team: e.Text("team"), Description: e.Text("description"), Agents: members,
			From: e.Text("from"), BasedOn: e.Text("basedOn"), SavedAt: e.At}
		if i := t.index(id); i >= 0 {
			(*t)[i] = saved
		} else {
			*t = append(*t, saved)
		}
	case events.TemplateRemoved:
		*t = slices.DeleteFunc(*t, func(u UserTemplate) bool { return u.ID == id })
	}
}

// WithID is the saved template with this ID, if there is one.
func (t UserTemplates) WithID(id string) (UserTemplate, bool) {
	if i := t.index(id); i >= 0 {
		return t[i], true
	}
	return UserTemplate{}, false
}

func (t UserTemplates) index(id string) int {
	return slices.IndexFunc(t, func(u UserTemplate) bool { return u.ID == id })
}

// Clone is a copy of the templates that shares nothing with them.
func (t UserTemplates) Clone() UserTemplates {
	if t == nil {
		return nil
	}
	cloned := make(UserTemplates, len(t))
	for i, u := range t {
		u.Agents = slices.Clone(u.Agents)
		for j, a := range u.Agents {
			u.Agents[j] = a.clone()
		}
		cloned[i] = u
	}
	return cloned
}

// RemovedTemplates are the ids of the templates you removed, each with the
// built-in template it was based on then ("" for none): the teams made
// from one still name its id, and keep that base's features. A removed id
// stays taken until an import saves it again.
type RemovedTemplates map[string]string

// RemovedTemplatesFromEvents is the removed templates after the templates
// stream.
func RemovedTemplatesFromEvents(stream []events.Event) RemovedTemplates {
	var removed RemovedTemplates
	var live UserTemplates
	for _, e := range stream {
		removed.Apply(e, live)
		live.Apply(e)
	}
	return removed
}

// Next is the removed templates after e, built on a copy: r itself is left
// as it was. live are the templates as they were before e, which hold the
// base of a template e removes.
func (r RemovedTemplates) Next(e events.Event, live UserTemplates) RemovedTemplates {
	r = r.Clone()
	r.Apply(e, live)
	return r
}

// Apply changes the removed templates in place by one accepted event of
// the templates stream; live are the templates as they were before it.
func (r *RemovedTemplates) Apply(e events.Event, live UserTemplates) {
	id := e.Text("id")
	switch e.Type {
	case events.TemplateSaved:
		delete(*r, id)
	case events.TemplateRemoved:
		gone, _ := live.WithID(id) // the rules accepted it as one of yours
		if *r == nil {
			*r = RemovedTemplates{}
		}
		(*r)[id] = gone.BasedOn
	}
}

// Clone is a copy of the removed templates that shares nothing with them.
func (r RemovedTemplates) Clone() RemovedTemplates {
	return maps.Clone(r)
}

// TemplateAgents reads a template.saved's agents, whether a proposal not
// yet written holds them as agents or the log gave them back as JSON.
func TemplateAgents(data map[string]any) ([]Agent, error) {
	encoded, err := json.Marshal(data["agents"])
	if err != nil {
		return nil, err
	}
	var members []Agent
	return members, json.Unmarshal(encoded, &members)
}
