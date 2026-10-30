package web

import (
	"context"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents/agentsapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy/policyapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/settings/settingsapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles/sharedfilesapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks/tasksapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers/workersapi"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
)

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

// workersAPI is the Workers page's routes as web mounts them, and those a
// joining or joined worker calls when the server has a hub for workers.
func (s *Server) workersAPI() workersapi.API {
	api := workersapi.API{Log: s.store, Events: s.store, State: s.state, Workers: s.workers, Roster: s.roster,
		Local: s.config.Local, InstallCommands: s.installCommands}
	if s.config.Fleet != nil {
		api.About = func(ctx context.Context, name string) (machines.About, error) {
			return s.config.Fleet.Of(name).About(ctx)
		}
	}
	if s.config.Workers != nil {
		api.Disconnect = s.config.Workers.Disconnect
		api.Connect, api.Stream, api.ServeBinary = s.config.Workers.Connect, s.config.Workers.Stream, s.serveBinary
		api.ServeScript = s.serveScriptNamed
	}
	return api
}

func (s *Server) workers() (workers.Roster, error) {
	return s.state.Workers(), s.state.Broken(events.WorkersStream)
}
