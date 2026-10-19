// Package settings computes your settings from the settings stream. A
// setting you never changed has its default, so a log from before it
// existed reads as if you had left it alone.
package settings

import "gitlab.com/dev.chempa/agentry/internal/events"

// Settings are your choices for how Agentry works.
type Settings struct {
	VisionariesOnEpicClose bool // a Visionaries session starts after you close an epic, about what it delivered and what could follow
}

// The settings' names in settings.changed.
const VisionariesOnEpicClose = "visionariesOnEpicClose"

// Default is the settings before you change any.
func Default() Settings { return Settings{VisionariesOnEpicClose: true} }

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
	if _, given := e.Data[VisionariesOnEpicClose]; given {
		s.VisionariesOnEpicClose = e.Flag(VisionariesOnEpicClose)
	}
	return s
}
