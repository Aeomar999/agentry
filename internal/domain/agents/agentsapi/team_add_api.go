package agentsapi

import (
	"net/http"
	"slices"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
)

// addTeam adds a team, made from a template ({template}: a built-in one's
// team, or the id of one you saved) or from agents you define ({team,
// agents}, and copiedFrom: the id of the built-in template they were copied
// from, if they were): its coordinator first, so the team always has
// one, then its agents, each with its own prompt. When the policy lacks a
// profile the team needs, it refuses with the profiles missing, unless asked
// to add them first (addMissingProfiles), which it does in one change of the
// policy. hash is the version of the policy the page saw, if it saw one.
// Every error is JSON {error, missingProfiles?}.
func (a API) addTeam(writer http.ResponseWriter, r *http.Request) {
	w := &jsonErrors{ResponseWriter: writer}
	var request struct {
		Template           string
		Team               string
		Agents             []agents.Agent
		CopiedFrom         string
		AddMissingProfiles bool
		Hash               string
	}
	if !httpapi.Decode(w, r, &request) {
		return
	}
	copiedFrom := strings.TrimSpace(request.CopiedFrom)
	custom := strings.TrimSpace(request.Team) != "" || len(request.Agents) > 0 || copiedFrom != ""
	switch {
	case request.Template != "" && custom:
		http.Error(w, "a team comes from a template or from agents you define, not both", http.StatusBadRequest)
		return
	case custom:
		a.addCustomTeam(w, strings.TrimSpace(request.Team), copiedFrom, request.Agents, request.AddMissingProfiles, request.Hash)
		return
	case request.Template == "":
		http.Error(w, "name a template, or a team and its agents", http.StatusBadRequest)
		return
	}
	template, ok := agents.TemplateNamed(request.Template)
	if !ok {
		a.addFromYourTemplate(w, request.Template, request.AddMissingProfiles, request.Hash)
		return
	}
	if !a.hasProfiles(w, template.Team, a.missingProfiles(template.Agents), request.AddMissingProfiles, request.Hash) {
		return
	}
	roster, err := a.Roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	a.proposeTeam(w, agents.AddTeam(template, roster, httpapi.You))
}

// addFromYourTemplate adds the team of the template you saved as id, under
// the rules a custom team has: a saved template was checked as it was then,
// and the roster and the policy may have changed since. The agents checked
// are those AddTeam makes, with the team name and numbers it picks.
func (a API) addFromYourTemplate(w http.ResponseWriter, id string, addMissing bool, hash string) {
	saved, ok := a.State.Templates().WithID(id)
	if !ok {
		http.Error(w, "no team template called "+id, http.StatusNotFound)
		return
	}
	roster, err := a.Roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	template := saved.Template()
	made := template.MadeFor(roster)
	if len(made) == 0 { // the rules refuse a template without agents, so only a broken log has one
		http.Error(w, "your template "+id+" has no agents", http.StatusInternalServerError)
		return
	}
	if !a.mayAddTeam(w, made[0].TeamName(), made, roster, addMissing, hash) {
		return
	}
	a.proposeTeam(w, agents.AddTeam(template, roster, httpapi.You))
}

// addCustomTeam adds the team of the agents you define, once every agent
// passes: nothing is proposed before then, so a team is never half made. A
// team copied from a built-in template is made from it, as AddTeam's are.
func (a API) addCustomTeam(w http.ResponseWriter, team, copiedFrom string, members []agents.Agent, addMissing bool, hash string) {
	if _, builtIn := agents.TemplateWithID(copiedFrom); copiedFrom != "" && !builtIn {
		http.Error(w, "a team is copied from a built-in template, and "+copiedFrom+" isn't the id of one", http.StatusBadRequest)
		return
	}
	roster, err := a.Roster()
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	if !a.mayAddTeam(w, team, members, roster, addMissing, hash) {
		return
	}
	a.proposeTeam(w, agents.CustomTeam(team, copiedFrom, members, httpapi.You))
}

// mayAddTeam says whether the team of these members passes every check a
// team you define has, adding the profiles it lacks when asked; when it
// says no, it has replied.
func (a API) mayAddTeam(w http.ResponseWriter, team string, members []agents.Agent, roster agents.Roster, addMissing bool, hash string) bool {
	machines, err := a.Workers()
	if err != nil {
		httpapi.Failed(w, err)
		return false
	}
	problem := agents.CheckCustomTeam(team, members, roster)
	if problem == "" {
		problem = unknownEngine(members)
	}
	if problem == "" {
		problem = unknownWorker(members, machines)
	}
	missing := a.missingProfiles(members)
	if problem == "" {
		problem = unshippedProfile(members, missing)
	}
	if problem != "" {
		http.Error(w, problem, http.StatusBadRequest)
		return false
	}
	return a.hasProfiles(w, team, missing, addMissing, hash)
}

// unknownEngine says which agent names an engine Agentry can't run, or "".
func unknownEngine(members []agents.Agent) string {
	for _, a := range members {
		if engine := strings.TrimSpace(a.Engine); !slices.Contains(rules.KnownEngines, engine) {
			return strings.TrimSpace(a.Name) + "'s engine must be one of " + strings.Join(rules.KnownEngines, ", ") + ", not " + strconv.Quote(engine)
		}
	}
	return ""
}

// unknownWorker says which agent names a worker that hasn't joined, or "".
func unknownWorker(members []agents.Agent, machines workers.Roster) string {
	for _, a := range members {
		if problem := rules.UnknownWorker(machines, a.Workers); problem != "" {
			return strings.TrimSpace(a.Name) + ": " + problem
		}
	}
	return ""
}

// proposeTeam proposes the events that add a team, stopping at the first
// one refused.
func (a API) proposeTeam(w http.ResponseWriter, proposals []events.Proposal) {
	for _, p := range proposals[:len(proposals)-1] {
		if !httpapi.ProposeOrRefuse(w, a.Log, p) {
			return
		}
	}
	httpapi.Propose(w, a.Log, proposals[len(proposals)-1])
}
