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

// mount puts f's routes on mux, each behind the wrapper its Auth names.
func (s *Server) mount(f extensions.Extension, mux *http.ServeMux) {
	if f.Routes == nil {
		return
	}
	for _, route := range f.Routes(s.coreOf(f)) {
		wrap := s.wrapperOf(route.Auth)
		mountRoute(f.Name, route, func(pattern string, handler http.HandlerFunc) { mux.Handle(pattern, wrap(handler)) })
	}
}

// wrapperOf is the wrapper auth names: signed for extensions.Signed,
// requireUIOrDesk for extensions.Desk, and requireUI for extensions.UI. None is
// requireRealUIToken, which hands out the token.
func (s *Server) wrapperOf(auth extensions.Auth) func(http.HandlerFunc) http.Handler {
	switch {
	case auth.Verify() != nil:
		return func(next http.HandlerFunc) http.Handler { return signed(auth.Verify(), next) }
	case auth.DeskToo():
		return s.requireUIOrDesk
	}
	return s.requireUI
}

// mountRoute registers one of extension's routes. A pattern that clashes with
// a route there already panics, as ServeMux does, but naming the extension
// and its pattern, which ServeMux's message doesn't.
func mountRoute(extension string, route extensions.Route, register func(pattern string, handler http.HandlerFunc)) {
	defer func() {
		if clash := recover(); clash != nil {
			panic(fmt.Sprintf("the %s extension's route %q can't be mounted: %v", extension, route.Pattern, clash))
		}
	}()
	register(route.Pattern, route.Handler)
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
