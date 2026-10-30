package agentsapi

import (
	"net/http"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
)

// teamTemplates are the teams you can add, each agent with what a team made
// from it gets, prompt included, so the page can copy one: the ready-made
// ones, then yours in the order you saved them.
func (a API) teamTemplates(w http.ResponseWriter, _ *http.Request) {
	list := []templateView{}
	for _, t := range agents.Templates {
		list = append(list, viewOf(t, false))
	}
	for _, u := range a.State.Templates() {
		list = append(list, yourView(u))
	}
	httpapi.Reply(w, list)
}

// templateView is a team template as the page lists it.
type templateView struct {
	ID          string       `json:"id"` // what GET /api/agents says an agent was made from
	Team        string       `json:"team"`
	Description string       `json:"description"`
	Features    []string     `json:"features"` // what a team made from it turns on, so the page can find that team
	Agents      []memberView `json:"agents"`
	UserMade    bool         `json:"userMade"`          // one you saved, not one Agentry ships
	SavedAt     string       `json:"savedAt,omitempty"` // yours: when you last saved it, RFC 3339
	From        string       `json:"from,omitempty"`    // yours: the team you saved it from, if the log says
	BasedOn     string       `json:"basedOn,omitempty"` // yours: the built-in template whose extensions its teams keep
}

type memberView struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Coordinates bool   `json:"coordinates"`
	Prompt      string `json:"prompt"`
	Engine      string `json:"engine"`
	Model       string `json:"model"`
	Profile     string `json:"profile"`
	Color       string `json:"color"`
	Column      string `json:"column"`
	Resume      string `json:"resume"`
}

// yourView is a template you saved, with when and where from, and the
// extensions of the built-in template it is based on, which its teams keep.
func yourView(u agents.UserTemplate) templateView {
	view := viewOf(u.Template(), true)
	view.SavedAt, view.From, view.BasedOn = u.SavedAt.UTC().Format(time.RFC3339), u.From, u.BasedOn
	if base, ok := agents.TemplateWithID(u.BasedOn); ok && base.Features != nil {
		view.Features = base.Features
	}
	return view
}

func viewOf(t agents.Template, userMade bool) templateView {
	view := templateView{ID: t.ID, Team: t.Team, Description: t.Description, Features: t.Features, UserMade: userMade}
	if view.Features == nil {
		view.Features = []string{}
	}
	for _, a := range t.Prompted() {
		view.Agents = append(view.Agents, memberView{Name: a.Name, Description: a.Description, Coordinates: a.Coordinates,
			Prompt: a.Prompt, Engine: a.Engine, Model: a.Model, Profile: a.Profile, Color: a.Color, Column: a.Column, Resume: a.Resume})
	}
	return view
}
