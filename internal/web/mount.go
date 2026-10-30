package web

import (
	"fmt"
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// mount puts routes on mux, each behind the wrapper its Auth names. owner
// says whose they are, "the ideas extension" or "the settings domain", for
// the panic when one clashes. Every extension's and domain's routes are
// mounted here, so they all sit behind a wrapper the same way.
func (s *Server) mount(owner string, routes []extensions.Route, mux *http.ServeMux) {
	for _, route := range routes {
		wrap := s.wrapperOf(route.Auth)
		mountRoute(owner, route, func(pattern string, handler http.HandlerFunc) { mux.Handle(pattern, wrap(handler)) })
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

// mountRoute registers one of owner's routes. A pattern that clashes with
// a route there already panics, as ServeMux does, but naming the owner
// and its pattern, which ServeMux's message doesn't.
func mountRoute(owner string, route extensions.Route, register func(pattern string, handler http.HandlerFunc)) {
	defer func() {
		if clash := recover(); clash != nil {
			panic(fmt.Sprintf("%s's route %q can't be mounted: %v", owner, route.Pattern, clash))
		}
	}()
	register(route.Pattern, route.Handler)
}
