// Package features is what a feature, such as ideas, adds to Agentry. It is
// a leaf: core packages import it, and it imports none that could import it
// back. Core reads a list of features; it never names one.
package features

import "gitlab.com/dev.chempa/agentry/internal/agents"

// A Feature is one capability a team or a setting turns on. Every field but
// Name may be left out.
type Feature struct {
	Name     string             // "ideas": its manifest entry, routes and UI folder use it
	Version  int                // raised when its events or state change
	Manifest func(Reader) Entry // whether the page shows it, and why
}

// Reader is the read side of the state a feature sees: current.State
// implements it. A feature never holds the store.
type Reader interface {
	Roster() agents.Roster
}

// Entry is whether the page shows a feature, and why.
type Entry struct {
	Enabled bool   `json:"enabled"`         // its trigger is present, so it can make new data
	Visible bool   `json:"visible"`         // the page shows it: enabled, or it has data to look at
	Why     string `json:"why,omitempty"`   // one line saying why it is on, kept or off
	Badge   int    `json:"badge,omitempty"` // a count for the sidebar, such as open ideas
}

// Line is a feature's line in GET /api/features.
type Line struct {
	Name string `json:"name"`
	Entry
}

// Manifest is every feature's line, in list's order, as of state. A feature
// with no Manifest is listed hidden.
func Manifest(list []Feature, state Reader) []Line {
	lines := []Line{}
	for _, f := range list {
		line := Line{Name: f.Name}
		if f.Manifest != nil {
			line.Entry = f.Manifest(state)
		}
		lines = append(lines, line)
	}
	return lines
}
