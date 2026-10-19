// Package settings computes your settings from the settings stream. A
// setting you never changed has its default, so a log from before it
// existed reads as if you had left it alone.
package settings

import (
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Settings are your choices for how Agentry works.
type Settings struct {
	VisionariesOnEpicClose bool // a Visionaries session starts after you close an epic, about what it delivered and what could follow
	AutoAcceptIdeas        bool   // Agentry accepts an idea the Visionaries agree on, by AutoAcceptRule, as you would
	AutoStartIdeas         bool   // and starts the task it made; only with AutoAcceptIdeas
	AutoAcceptRule         string // which ideas count as agreed: EveryoneBacked or NobodyDoubted
}

// The settings' names in settings.changed.
const (
	VisionariesOnEpicClose = "visionariesOnEpicClose"
	AutoAcceptIdeas        = "autoAcceptIdeas"
	AutoStartIdeas         = "autoStartIdeas"
	AutoAcceptRule         = "autoAcceptRule"
)

// The rules AutoAcceptRule can be.
const (
	EveryoneBacked = "everyoneBacked" // every Visionaries viewpoint backs the idea and none doubts it
	NobodyDoubted  = "nobodyDoubted"  // none doubts it, and a few viewpoints back it (ideas.MinBackers)
)

// AutoAcceptRules are the rules AutoAcceptRule can be, the strict one first.
var AutoAcceptRules = []string{EveryoneBacked, NobodyDoubted}

// Default is the settings before you change any.
func Default() Settings { return Settings{VisionariesOnEpicClose: true, AutoAcceptRule: EveryoneBacked} }

// FromEvents applies the settings stream's accepted events in order.
func FromEvents(stream []events.Event) Settings {
	s := Default()
	for _, e := range stream {
		s = s.Next(e)
	}
	return s
}

// Next is the settings after e. An event names only the settings it
// changes; the others stay as they were.
func (s Settings) Next(e events.Event) Settings {
	if e.Type != events.SettingsChanged {
		return s
	}
	for name, setting := range map[string]*bool{VisionariesOnEpicClose: &s.VisionariesOnEpicClose,
		AutoAcceptIdeas: &s.AutoAcceptIdeas, AutoStartIdeas: &s.AutoStartIdeas} {
		if _, given := e.Data[name]; given {
			*setting = e.Flag(name)
		}
	}
	// A rule this version doesn't know leaves the one before, so a log from
	// a later version still replays.
	if rule := e.Text(AutoAcceptRule); slices.Contains(AutoAcceptRules, rule) {
		s.AutoAcceptRule = rule
	}
	return s
}
