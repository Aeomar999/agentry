package web

import (
	"net/http"
	"regexp"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/rules"
)

// Saving a team as a template of your own, and removing one. The rules in
// package rules check a template as it is saved; a refusal is your request's
// to fix, so it is a 400 here rather than Propose's 409.

// saveTemplate saves a team's agents, as they are now, as your template
// id. Saving again under the same id replaces it.
func (s *Server) saveTemplate(w http.ResponseWriter, r *http.Request) {
	var request struct{ ID, Description string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	roster, err := s.roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	team := roster.TeamNamed(r.PathValue("team"))
	if team == "" {
		http.Error(w, "there is no team called "+r.PathValue("team"), http.StatusNotFound)
		return
	}
	id := strings.TrimSpace(request.ID)
	if id == "" {
		id = templateIDFor(team)
		if reason := rules.CheckTemplateID(id); reason != "" {
			// The id is ours, not theirs: say where it came from, or the
			// refusal names an id they never typed.
			http.Error(w, reason+` (made from the team's name "`+team+`"): give the template an id`, http.StatusBadRequest)
			return
		}
	}
	proposal, err := agents.SaveTemplate(roster, team, id, strings.TrimSpace(request.Description), httpapi.You)
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	s.proposeTemplate(w, proposal, id)
}

// exportTemplate is your template id as a file to keep or to import into
// another Agentry. Only your own templates are exported: a built-in one
// comes with every Agentry already.
func (s *Server) exportTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, builtIn := agents.TemplateWithID(id); builtIn {
		http.Error(w, "the "+id+" template comes with Agentry, so it isn't exported: only your own templates are", http.StatusBadRequest)
		return
	}
	saved, found := s.state.Templates().WithID(id)
	if !found {
		http.Error(w, "there is no saved template "+id, http.StatusNotFound)
		return
	}
	// The rules held the id to letters, digits and dashes, so it is safe
	// in the header as it is.
	w.Header().Set("Content-Disposition", `attachment; filename="`+id+`.json"`)
	httpapi.Reply(w, saved)
}

// importTemplate saves a template exported as a file, under the rules a
// template you save from a team has. Importing an id you have replaces it.
func (s *Server) importTemplate(w http.ResponseWriter, r *http.Request) {
	var template agents.UserTemplate
	if !httpapi.Decode(w, r, &template) {
		return
	}
	template.Team, template.Description = strings.TrimSpace(template.Team), strings.TrimSpace(template.Description)
	s.proposeTemplate(w, template.Save(httpapi.You), template.ID)
}

// proposeTemplate writes a template.saved and replies with the template
// saved, or with the rules' refusal as a 400.
func (s *Server) proposeTemplate(w http.ResponseWriter, proposal events.Proposal, id string) {
	switch _, rejection, err := s.store.Propose(proposal); {
	case err != nil:
		httpapi.Failed(w, err)
		return
	case rejection != nil:
		http.Error(w, rejection.Reason, http.StatusBadRequest)
		return
	}
	saved, _ := s.state.Templates().WithID(id)
	httpapi.Reply(w, viewOf(saved.Template(), true))
}

// removeTemplate removes your template id. The teams made from it stay, and
// the built-in templates can't be removed.
func (s *Server) removeTemplate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if _, builtIn := agents.TemplateWithID(id); builtIn {
		http.Error(w, "the "+id+" template comes with Agentry, so it can't be removed", http.StatusConflict)
		return
	}
	if _, saved := s.state.Templates().WithID(id); !saved {
		http.Error(w, "there is no saved template "+id, http.StatusNotFound)
		return
	}
	httpapi.Propose(w, s.store, agents.RemoveTemplate(id, httpapi.You))
}

var notInAnID = regexp.MustCompile(`[^a-z0-9]+`)

// templateIDFor is the id a template saved without one gets: the team's
// name in lowercase, each run of anything but letters and digits made one
// dash, with none at the ends, so "My Team!" is "my-team". The rules still
// check it: a name starting with a digit, or one too long, is refused.
func templateIDFor(team string) string {
	return strings.Trim(notInAnID.ReplaceAllString(strings.ToLower(team), "-"), "-")
}
