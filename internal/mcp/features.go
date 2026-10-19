package mcp

import (
	"context"
	"fmt"
	"maps"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/features"
)

// A session lists the tools and instructions of the features GET
// /api/features says are enabled or visible. It reads them at initialize,
// then again at every change in the log, and tells Claude Code when the
// set changes, so a team added mid-session brings its tools with it.

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

// followFeatures reads the features again each time the log moves, and
// sends notifications/tools/list_changed when the ones shown changed, until
// ctx ends. It waits on GET /api/changes itself: Watch, which waits on it
// too, doesn't run with --quiet, and returns only with news for you.
func (s *Server) followFeatures(ctx context.Context) {
	agentry := s.agentry
	agentry.Timeout = 40 * time.Second // the server answers a wait within 25 seconds
	var position int64
	for ctx.Err() == nil {
		var changes struct{ Position int64 }
		if err := agentry.CallContext(ctx, "GET", fmt.Sprintf("/api/changes?after=%d", position), nil, &changes); err != nil {
			select { // the server may be restarting
			case <-ctx.Done():
			case <-time.After(notifyEvery):
			}
			continue
		}
		if changes.Position == position {
			continue // the wait ran out with nothing new
		}
		position = changes.Position
		if s.showFeatures() {
			s.conn.notify("notifications/tools/list_changed")
		}
	}
}
