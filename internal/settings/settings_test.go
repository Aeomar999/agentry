package settings

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

func changed(data map[string]any) events.Event {
	return events.Event{Proposal: events.Proposal{Stream: events.SettingsStream, Type: events.SettingsChanged, Data: data}}
}

func TestASettingNeverChangedIsOn(t *testing.T) {
	if s := FromEvents(nil); !s.VisionariesOnEpicClose {
		t.Error("the Visionaries after an epic closes should be on by default")
	}
}

func TestAChangeKeepsTheSettingsItDoesntName(t *testing.T) {
	s := FromEvents([]events.Event{changed(map[string]any{VisionariesOnEpicClose: false}), changed(map[string]any{"another": true})})
	if s.VisionariesOnEpicClose {
		t.Error("turned off, then another setting changed: it should stay off")
	}
	if s = s.Next(changed(map[string]any{VisionariesOnEpicClose: true})); !s.VisionariesOnEpicClose {
		t.Error("turned on again: it should be on")
	}
}
