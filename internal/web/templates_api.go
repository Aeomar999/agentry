package web

import (
	"cmp"
	"net/http"
	"regexp"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/rules"
)

// Saving a team as a template of your own, and removing one. The rules in
// package rules check a template as it is saved; a refusal is your request's
// to fix, so it is a 400 here rather than Propose's 409.

// saveTemplate saves a team's agents, as they are now, as your template
// name (by default the team's): a team made from it is called that. Its id,
// unless given, comes from the name. A name you or Agentry have a template
// called already, in any case, or an id one of yours has, is refused with a
// 409: saving never replaces a template.
func (s *Server) saveTemplate(w http.ResponseWriter, r *http.Request) {
	var request struct{ Name, ID, Description string }
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
	name := cmp.Or(strings.TrimSpace(request.Name), team)
	if taken := s.templateCalled(name); taken != "" {
		http.Error(w, taken, http.StatusConflict)
		return
	}
	id := strings.TrimSpace(request.ID)
	if id == "" {
		id = templateIDFor(name)
		if reason := rules.CheckTemplateID(id); reason != "" {
			// The id is ours, not theirs: say where it came from, or the
			// refusal names an id they never typed.
			http.Error(w, reason+` (made from the template's name "`+name+`"): give the template an id`, http.StatusBadRequest)
			return
		}
	}
	if yours, taken := s.state.Templates().WithID(id); taken {
		http.Error(w, yoursCalled(yours.Team), http.StatusConflict)
		return
	}
	proposal, err := agents.SaveTemplate(roster, team, name, id, strings.TrimSpace(request.Description), httpapi.You)
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	s.proposeTemplate(w, proposal, id)
}

// templateCalled is why name can't be a new template's, in the words the
// page shows under its name, or "" when it can: a template, yours or
// built in, already has it in any case.
func (s *Server) templateCalled(name string) string {
	if builtIn, ok := agents.TemplateNamed(name); ok {
		return builtIn.Team + " is a built-in template. Pick another name."
	}
	for _, yours := range s.state.Templates() {
		if strings.EqualFold(yours.Team, name) {
			return yoursCalled(yours.Team)
		}
	}
	return ""
}

func yoursCalled(template string) string {
	return "You have a template called " + template + " already. Pick another name."
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
	// When it was saved is this Agentry's: importing it saves it anew.
	saved.SavedAt = time.Time{}
	httpapi.Reply(w, saved)
}

// importTemplate saves a template exported as a file, under the rules a
// template you save from a team has. Importing an id you have replaces it,
// keeping its place; a name you have under another id is kept beside it.
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
	httpapi.Reply(w, yourView(saved))
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
