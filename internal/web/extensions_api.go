package web

import (
	"fmt"
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// listFeatures answers whether the page shows each extension, and why, worked
// out from the current state on every request, the board's columns, and
// what the log holds that this build skipped: an extension's streams it doesn't
// have, types it doesn't know.
func (s *Server) listFeatures(w http.ResponseWriter, r *http.Request) {
	httpapi.Reply(w, map[string]any{
		"features": extensions.Manifest(s.config.Extensions, s.state),
		"board":    map[string]any{"columns": tasks.BoardColumns(s.state.Roster(), s.state.Tasks())},
		"unknown":  s.state.Unknown(),
	})
}

// routesOf are f's routes, made with the Core it may use; none when it has
// none. A route whose Auth only a domain may take panics, naming f, as a
// clash does: every extension's routes come through here, and no domain's.
func (s *Server) routesOf(f extensions.Extension) []extensions.Route {
	if f.Routes == nil {
		return nil
	}
	routes := f.Routes(s.coreOf(f))
	for _, route := range routes {
		if route.Auth.ForWorkers() {
			panic(fmt.Sprintf("the %s extension's route %q can't be mounted: only a domain's route may take a worker's credential", f.Name, route.Pattern))
		}
	}
	return routes
}

// coreOf is what f's routes may use: its checked proposer, never the store,
// or one that writes nothing when the server was given none for it, and its
// own settings, none when the server was given no settings file.
func (s *Server) coreOf(f extensions.Extension) extensions.Core {
	log, found := s.config.Logs[f.Name]
	if !found {
		log = extensions.Unwritable(f.Name)
	}
	var settings extensions.Settings
	if s.config.Settings != nil {
		settings = s.config.Settings(f.Name)
	}
	return extensions.Core{Log: log, Events: s.events, Read: s.state, Defaults: s.config.defaults(), Settings: settings}
}
