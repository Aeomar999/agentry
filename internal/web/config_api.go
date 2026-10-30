package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy/policyapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
)

// showConfig is what the page needs to know about the server. policyProblem
// says policy.toml has a mistake, and missingProfiles names the profiles
// agents have that it lacks, with those agents, so every page can point to
// the Policy page, which says what to do.
func (s *Server) showConfig(w http.ResponseWriter, r *http.Request) {
	policyProblem := s.config.Policy != nil && s.config.Policy.Problem() != ""
	config := map[string]any{"defaultRepo": s.config.DefaultRepo, "budget": s.config.Budget, "maxTurns": s.config.MaxTurns,
		"engines": rules.KnownEngines, "workers": s.workerChoices(), "colors": agents.Colors, "canAddWorkers": s.config.Workers != nil,
		"policyProblem": policyProblem, "missingProfiles": s.missingProfilesInUse()}
	if trust := s.trustAt(r, requestURL(r)); trust != nil {
		config["trust"] = trust // the page is served with the server's own CA
	}
	httpapi.Reply(w, config)
}

// missingProfilesInUse is policy.MissingProfiles for the roster: none while
// the policy or the roster can't be read.
func (s *Server) missingProfilesInUse() map[string][]string {
	roster, err := s.roster()
	if s.config.Policy == nil || err != nil {
		return map[string][]string{}
	}
	return s.config.Policy.Current().MissingProfiles(policyapi.AgentProfiles(roster))
}

// setTheme records the page's theme, "light" or "dark". Sessions started
// after it use it, so the engines draw colours that suit the terminal.
func (s *Server) setTheme(w http.ResponseWriter, r *http.Request) {
	var request struct{ Theme string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	if request.Theme != "light" && request.Theme != "dark" {
		http.Error(w, `the theme is "light" or "dark"`, http.StatusBadRequest)
		return
	}
	if s.config.SetTheme != nil {
		s.config.SetTheme(request.Theme)
	}
	httpapi.Reply(w, map[string]string{"theme": request.Theme})
}

// workerChoices are the machines an agent can be given: this one, then
// every worker that has joined.
func (s *Server) workerChoices() []string {
	roster, _ := s.workers()
	return append([]string{workers.Local}, roster.Names()...)
}

// waitForChanges answers when the log has moved past ?after=, or after 25 s,
// with the newest position. The page long-polls it and refreshes on change:
// it reads the log at its own pace, like every other reader (proof 05).
func (s *Server) waitForChanges(w http.ResponseWriter, r *http.Request) {
	after, _ := strconv.ParseInt(r.URL.Query().Get("after"), 10, 64)
	ctx, cancel := context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	reader := s.store.NewReader(after)
	defer reader.Close()
	reader.Next(ctx) // returns early with events, or when ctx ends
	httpapi.Reply(w, map[string]int64{"position": s.store.LastPosition()})
}
