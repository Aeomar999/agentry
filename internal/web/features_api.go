package web

import (
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/features"
)

// listFeatures answers whether the page shows each feature, and why, worked
// out from the current state on every request.
func (s *Server) listFeatures(w http.ResponseWriter, r *http.Request) {
	reply(w, map[string]any{"features": features.Manifest(s.config.Features, s.state)})
}
