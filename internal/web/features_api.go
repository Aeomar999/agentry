package web

import (
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

// mount puts f's routes on the API, each behind the wrapper its Auth names:
// deskToo for features.Desk, api for features.UI and any other.
func (s *Server) mount(f features.Feature, api, deskToo func(pattern string, handler http.HandlerFunc)) {
	if f.Routes == nil {
		return
	}
	for _, route := range f.Routes(s.coreOf(f)) {
		switch route.Auth {
		case features.Desk:
			deskToo(route.Pattern, route.Handler)
		default:
			api(route.Pattern, route.Handler)
		}
	}
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
