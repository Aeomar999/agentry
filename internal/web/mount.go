package web

import (
	"fmt"
	"net/http"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
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
// requireUIOrDesk for extensions.Desk, requireWorker for extensions.Worker,
// none for extensions.Open, whose handler checks its own code, and
// requireUI for extensions.UI. None is requireRealUIToken, which hands out
// the token.
func (s *Server) wrapperOf(auth extensions.Auth) func(http.HandlerFunc) http.Handler {
	switch {
	case auth.IsOpen():
		return func(next http.HandlerFunc) http.Handler { return next }
	case auth.Verify() != nil:
		return func(next http.HandlerFunc) http.Handler { return signed(auth.Verify(), next) }
	case auth.DeskToo():
		return s.requireUIOrDesk
	case auth.ForWorkers():
		return s.requireWorker
	}
	return s.requireUI
}

// requireWorker lets in a worker that shows its credential, and tells the
// handler which worker it is, through extensions.WorkerOf.
func (s *Server) requireWorker(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		roster, err := s.workers()
		if err != nil {
			httpapi.Failed(w, err)
			return
		}
		worker, ok := roster.WithCredential(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
		if !ok {
			http.Error(w, "this worker isn't joined; join it again from the Workers page", http.StatusUnauthorized)
			return
		}
		next(w, extensions.WithWorker(r, worker))
	})
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
