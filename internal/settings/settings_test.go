package settings

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
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

func TestIdeasAreNeverAcceptedForYouUnlessYouSaySo(t *testing.T) {
	s := FromEvents([]events.Event{changed(map[string]any{VisionariesOnEpicClose: false})})
	if s.AutoAcceptIdeas || s.AutoStartIdeas {
		t.Errorf("a log that never named them should leave both off: %+v", s)
	}
	s = s.Next(changed(map[string]any{AutoAcceptIdeas: true, AutoStartIdeas: true}))
	if !s.AutoAcceptIdeas || !s.AutoStartIdeas || s.VisionariesOnEpicClose {
		t.Errorf("turned both on: %+v", s)
	}
	if s = s.Next(changed(map[string]any{AutoStartIdeas: false})); !s.AutoAcceptIdeas || s.AutoStartIdeas {
		t.Errorf("turned starting off: accepting should stay on: %+v", s)
	}
}

func TestTheAutoAcceptRuleIsStrictUntilYouLoosenIt(t *testing.T) {
	s := FromEvents([]events.Event{changed(map[string]any{AutoAcceptIdeas: true})})
	if s.AutoAcceptRule != EveryoneBacked {
		t.Errorf("a log that never named the rule should read as %s: %q", EveryoneBacked, s.AutoAcceptRule)
	}
	if s = s.Next(changed(map[string]any{AutoAcceptRule: NobodyDoubted})); s.AutoAcceptRule != NobodyDoubted || !s.AutoAcceptIdeas {
		t.Errorf("loosened: %+v", s)
	}
	for _, unknown := range []any{"mostBacked", "", 3, true} {
		if got := s.Next(changed(map[string]any{AutoAcceptRule: unknown})); got != s {
			t.Errorf("an unknown rule %v should leave the one before: %+v", unknown, got)
		}
	}
	if s = s.Next(changed(map[string]any{AutoAcceptIdeas: false})); s.AutoAcceptRule != NobodyDoubted {
		t.Errorf("turning accepting off keeps the rule: %+v", s)
	}
}

func TestTasksRunWithNoLimitUntilYouSetOne(t *testing.T) {
	s := FromEvents([]events.Event{changed(map[string]any{AutoAcceptIdeas: true})})
	if s.MaxRunningTasks != 0 {
		t.Errorf("a log that never named the limit should read as no limit: %d", s.MaxRunningTasks)
	}
	// As read back from the log, a number is a float64.
	if s = s.Next(changed(map[string]any{MaxRunningTasks: float64(2)})); s.MaxRunningTasks != 2 || !s.AutoAcceptIdeas {
		t.Errorf("set to 2: %+v", s)
	}
	if got := s.Next(changed(map[string]any{AutoStartIdeas: true})); got.MaxRunningTasks != 2 {
		t.Errorf("another setting changed: the limit should stay 2: %+v", got)
	}
	if got := s.Next(changed(map[string]any{MaxRunningTasks: -1})); got.MaxRunningTasks != 2 {
		t.Errorf("a negative limit should leave the one before: %+v", got)
	}
	if s = s.Next(changed(map[string]any{MaxRunningTasks: 0})); s.MaxRunningTasks != 0 {
		t.Errorf("cleared: it should be no limit again: %+v", s)
	}
}
