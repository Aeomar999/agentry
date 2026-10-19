package mcp

import (
	"maps"

	"gitlab.com/dev.chempa/agentry/internal/features"
)

// A session lists the tools and instructions of the features GET
// /api/features says are enabled or visible, as it read them at initialize.

// showFeatures reads which features are shown, and says whether that
// changed. A server that can't say leaves them as they were.
func (s *Server) showFeatures() (changed bool) {
	var reply struct{ Features []features.Line }
	if err := s.agentry.Call("GET", "/api/features", nil, &reply); err != nil {
		return false
	}
	shown := map[string]bool{}
	for _, line := range reply.Features {
		if line.Enabled || line.Visible {
			shown[line.Name] = true
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	changed = !maps.Equal(shown, s.shown)
	s.shown = shown
	return changed
}

// shownFeatures are the features shown now, in s.features' order.
func (s *Server) shownFeatures() []features.Feature {
	s.mu.Lock()
	defer s.mu.Unlock()
	var shown []features.Feature
	for _, f := range s.features {
		if s.shown[f.Name] {
			shown = append(shown, f)
		}
	}
	return shown
}
