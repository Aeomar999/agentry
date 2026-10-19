package web

import (
	"fmt"
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
)

// listFeatures answers whether the page shows each feature, and why, worked
// out from the current state on every request, and what the log holds that
// this build skipped: a feature's streams it doesn't have, types it doesn't know.
func (s *Server) listFeatures(w http.ResponseWriter, r *http.Request) {
	httpapi.Reply(w, map[string]any{"features": features.Manifest(s.config.Features, s.state), "unknown": s.state.Unknown()})
}

// mount puts f's routes on mux, each behind the wrapper its Auth names.
func (s *Server) mount(f features.Feature, mux *http.ServeMux) {
	if f.Routes == nil {
		return
	}
	for _, route := range f.Routes(s.coreOf(f)) {
		wrap := s.wrapperOf(route.Auth)
		mountRoute(f.Name, route, func(pattern string, handler http.HandlerFunc) { mux.Handle(pattern, wrap(handler)) })
	}
}

// wrapperOf is the wrapper auth names: signed for features.Signed,
// requireUIOrDesk for features.Desk, and requireUI for features.UI and any
// other. None is requireRealUIToken, which hands out the token.
func (s *Server) wrapperOf(auth features.Auth) func(http.HandlerFunc) http.Handler {
	switch {
	case auth.Verify() != nil:
		return func(next http.HandlerFunc) http.Handler { return signed(auth.Verify(), next) }
	case auth.DeskToo():
		return s.requireUIOrDesk
	}
	return s.requireUI
}

// mountRoute registers one of feature's routes. A pattern that clashes with
// a route there already panics, as ServeMux does, but naming the feature
// and its pattern, which ServeMux's message doesn't.
func mountRoute(feature string, route features.Route, register func(pattern string, handler http.HandlerFunc)) {
	defer func() {
		if clash := recover(); clash != nil {
			panic(fmt.Sprintf("the %s feature's route %q can't be mounted: %v", feature, route.Pattern, clash))
		}
	}()
	register(route.Pattern, route.Handler)
}

// coreOf is what f's routes may use: its checked proposer, never the store,
// or one that writes nothing when the server was given none for it.
func (s *Server) coreOf(f features.Feature) features.Core {
	log, found := s.config.Logs[f.Name]
	if !found {
		log = features.Unwritable(f.Name)
	}
	return features.Core{Log: log, Events: s.events, Read: s.state, Defaults: s.config.defaults()}
}
