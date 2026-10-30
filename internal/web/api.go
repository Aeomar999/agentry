package web

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents/agentsapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy/policyapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/settings/settingsapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles/sharedfilesapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks/tasksapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
)

// defaults are the limits a new task gets when it names none.
func (c Config) defaults() tasks.Limits { return tasks.Limits{Budget: c.Budget, MaxTurns: c.MaxTurns} }

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

func (s *Server) task(id string) (*tasks.Task, error) { return s.state.Task(id) }

func (s *Server) roster() (agents.Roster, error) {
	return s.state.Roster(), s.state.Broken(events.AgentsStream)
}

// policyAPI is the Policy page's routes as web mounts them, and the one
// way a team's missing profiles are added to the policy too.
func (s *Server) policyAPI() policyapi.API {
	return policyapi.API{Log: s.store, Events: s.store, Policy: s.config.Policy, Roster: s.roster,
		Version: s.version(), Worktrees: s.config.Worktrees}
}

// settingsAPI is the Settings page's routes as web mounts them, redacting
// a secret too.
func (s *Server) settingsAPI() settingsapi.API {
	return settingsapi.API{Log: s.store, State: s.state, Store: s.store, Blobs: s.config.Blobs, Redactions: s.config.Redactions,
		Extensions: s.config.Extensions, Forget: s.archives.Forget, WriteOverOldText: s.tasksAPI().WriteOverOldText}
}

// tasksAPI is the tasks' routes as web mounts them, and the one way a
// message to a task's coordinator is sent, filing a task under an epic,
// finding a repo's folder and writing over a redaction's old text too.
func (s *Server) tasksAPI() tasksapi.API {
	return tasksapi.API{Log: s.store, Store: s.store, State: s.state, Archives: s.archives, Roster: s.roster, Fleet: s.config.Fleet,
		DefaultRepo: s.config.DefaultRepo, Defaults: s.config.defaults(), Clones: s.config.Clones, Blobs: s.config.Blobs,
		StopSession: s.config.StopSession, Snapshots: s.config.Snapshots, SharedFiles: filesShown, CarriesUIToken: s.carriesUIToken}
}

// filesShown is a task's shared files as its detail shows them.
func filesShown(t *tasks.Task) any { return sharedfilesapi.FilesOf(t) }

// agentsAPI is the Agents page's routes as web mounts them.
func (s *Server) agentsAPI() agentsapi.API {
	return agentsapi.API{Log: s.store, Events: s.store, State: s.state, Roster: s.roster, Workers: s.workers,
		Policy: s.config.Policy, ApplyPolicy: s.policyAPI().Apply, EffectiveOf: effectiveOf, Version: s.version()}
}

// effectiveOf is policyapi.EffectiveOf as an agent on the Agents page shows it.
func effectiveOf(p *policy.Policy, profile string) any {
	effective := policyapi.EffectiveOf(p, profile)
	return &effective
}

// version is this Agentry's, as the comment above a profile it adds says.
func (s *Server) version() string {
	if s.config.Local.Version == "" {
		return "(unknown version)"
	}
	return s.config.Local.Version
}
