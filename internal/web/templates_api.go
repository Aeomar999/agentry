package web

import (
	"cmp"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
)

// Saving a team as a template of your own, and removing one. The rules in
// package rules check a template as it is saved; a refusal is your request's
// to fix, so it is a 400 here rather than Propose's 409.

// saveTemplate saves a team's agents, as they are now, as your template
// name (by default the team's): a team made from it is called that. Its id,
// unless given, comes from the name and is always one a template may have.
// A name you or Agentry have a template called already, in any case, or an
// id given that one of yours has, is refused with a 409: saving never
// replaces a template. The rules refuse it too, for a save that came in at
// the same time; that refusal is the same 409. Once saved, the team is
// linked to the template, so it keeps the features of the built-in one the
// template is based on; a link refused leaves the template saved, and the
// reply says why the team isn't linked (notLinked).
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
	name, id := cmp.Or(strings.TrimSpace(request.Name), team), strings.TrimSpace(request.ID)
	if taken := s.templateTaken(name, id); taken != "" {
		http.Error(w, taken, http.StatusConflict)
		return
	}
	if id == "" {
		id = templateIDFor(name, s.templateIDTaken)
	}
	proposal, err := agents.SaveTemplate(roster, s.state.Templates(), s.state.RemovedTemplates(), team, name, id, strings.TrimSpace(request.Description), httpapi.You)
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	saved, ok := s.proposeTemplate(w, proposal, id, func() string { return s.templateTaken(name, id) })
	if !ok {
		return
	}
	httpapi.Reply(w, savedView{templateView: yourView(saved), NotLinked: s.link(team, id)})
}

// savedView is a template you saved from a team, and why that team isn't
// linked to it, if it isn't.
type savedView struct {
	templateView
	NotLinked string `json:"notLinked,omitempty"`
}

// link links team to the template id, and is why it couldn't, or "".
func (s *Server) link(team, id string) string {
	switch _, rejection, err := s.store.Propose(agents.TeamLinked(team, id, httpapi.You)); {
	case err != nil:
		return "the log failed: " + err.Error()
	case rejection != nil:
		return rejection.Reason
	}
	return ""
}

// linkTeam links a team, named in any case, to a template ({template}): a
// built-in one, one of yours, or one you removed, as the id the team was
// saved under. It is how a team saved as a template, but refused the link
// then, gets it.
func (s *Server) linkTeam(w http.ResponseWriter, r *http.Request) {
	var request struct{ Template string }
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
	template := strings.TrimSpace(request.Template)
	if why := s.link(team, template); why != "" {
		http.Error(w, why, http.StatusBadRequest)
		return
	}
	httpapi.Reply(w, map[string]string{"team": team, "template": template})
}

// templateTaken is why a new template can't be called name or have id (""
// for one still to be made), in the words the page shows under its name, or
// "" when it can. A removed template's id stays taken: its teams still
// name it.
func (s *Server) templateTaken(name, id string) string {
	if taken := s.templateCalled(name); taken != "" {
		return taken
	}
	if yours, taken := s.state.Templates().WithID(id); id != "" && taken {
		return yoursCalled(yours.Team)
	}
	if _, removed := s.state.RemovedTemplates()[id]; removed {
		return "You removed a template with the id " + id + ". Import its file to bring it back, or pick another id."
	}
	return ""
}

// templateCalled is why name can't be a new template's, or "" when it can:
// a template, yours or built in, already has it in any case.
func (s *Server) templateCalled(name string) string {
	if builtIn, ok := agents.TemplateNamed(name); ok {
		return builtInCalled(builtIn.Team)
	}
	for _, yours := range s.state.Templates() {
		if strings.EqualFold(yours.Team, name) {
			return yoursCalled(yours.Team)
		}
	}
	return ""
}

func builtInCalled(template string) string {
	return template + " is a built-in template. Pick another name."
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
// keeping its place. A name a built-in template or another of yours has, in
// any case, is refused with the 409 a save gives, in its words; the rules
// refuse it too, for an import that came in at the same time.
func (s *Server) importTemplate(w http.ResponseWriter, r *http.Request) {
	var template agents.UserTemplate
	if !httpapi.Decode(w, r, &template) {
		return
	}
	template.Team, template.Description = strings.TrimSpace(template.Team), strings.TrimSpace(template.Description)
	taken := func() string { return s.importTaken(template.Team, template.ID) }
	if why := taken(); why != "" {
		http.Error(w, why, http.StatusConflict)
		return
	}
	if saved, ok := s.proposeTemplate(w, template.Save(httpapi.You), template.ID, taken); ok {
		httpapi.Reply(w, yourView(saved))
	}
}

// importTaken is why an import can't be called name, or "" when it can: as
// templateCalled, except that the template of yours with its id is the one
// it replaces, so its own name is free.
func (s *Server) importTaken(name, id string) string {
	if builtIn, ok := agents.TemplateNamed(name); ok {
		return builtInCalled(builtIn.Team)
	}
	for _, yours := range s.state.Templates() {
		if yours.ID != id && strings.EqualFold(yours.Team, name) {
			return yoursCalled(yours.Team)
		}
	}
	return ""
}

// proposeTemplate writes a template.saved and is the template saved, or
// replies with the rules' refusal as a 400. taken, if given, words a
// refusal as a 409 when a template has its name or id by then.
func (s *Server) proposeTemplate(w http.ResponseWriter, proposal events.Proposal, id string, taken func() string) (agents.UserTemplate, bool) {
	switch _, rejection, err := s.store.Propose(proposal); {
	case err != nil:
		httpapi.Failed(w, err)
		return agents.UserTemplate{}, false
	case rejection != nil && taken != nil && taken() != "":
		http.Error(w, taken(), http.StatusConflict)
		return agents.UserTemplate{}, false
	case rejection != nil:
		http.Error(w, rejection.Reason, http.StatusBadRequest)
		return agents.UserTemplate{}, false
	}
	saved, _ := s.state.Templates().WithID(id)
	return saved, true
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

// templateIDFor is the id a template saved without one gets: its name in
// lowercase, each run of anything but letters and digits made one dash,
// with none at the ends, so "My Team!" is "my-team". The page has no id
// field, so the id always passes rules.CheckTemplateID and is free: "team-"
// goes before a name starting with a digit, "team" stands for one with no
// letters or digits, a long one is cut to fit, and one taken gets -2, -3...
func templateIDFor(name string, taken func(id string) bool) string {
	base := strings.Trim(notInAnID.ReplaceAllString(strings.ToLower(name), "-"), "-")
	switch {
	case base == "":
		base = "team"
	case base[0] < 'a':
		base = "team-" + base
	}
	id := fitted(base, "")
	for n := 2; taken(id); n++ {
		id = fitted(base, "-"+strconv.Itoa(n))
	}
	return id
}

// longestTemplateID is the most characters rules.CheckTemplateID allows.
const longestTemplateID = 31

// fitted is base and then suffix, base cut so the two fit in an id, with no
// dash left where it was cut.
func fitted(base, suffix string) string {
	if room := longestTemplateID - len(suffix); len(base) > room {
		base = strings.TrimRight(base[:room], "-")
	}
	return base + suffix
}

// templateIDTaken says whether a built-in template or one of yours, there
// or removed, has id.
func (s *Server) templateIDTaken(id string) bool {
	_, builtIn := agents.TemplateWithID(id)
	_, yours := s.state.Templates().WithID(id)
	_, removed := s.state.RemovedTemplates()[id]
	return builtIn || yours || removed
}
