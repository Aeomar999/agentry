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
