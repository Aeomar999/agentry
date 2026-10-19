package agents

import (
	"reflect"
	"slices"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// The roster has spare capacity, so an append could write where a reader of
// the roster before would see it, and the whole array is compared.
func TestNextLeavesTheRosterAsItWas(t *testing.T) {
	before := seeded()
	for _, e := range everyAgentEvent(FromEvents(before)) {
		roster, want := slices.Grow(FromEvents(before), 4), slices.Grow(FromEvents(before), 4)
		next := roster.Next(e)
		if !reflect.DeepEqual(roster[:cap(roster)], want[:cap(want)]) {
			t.Errorf("%s changed the roster it was applied to", e.Type)
		}
		if after := FromEvents(append(slices.Clip(before), e)); !reflect.DeepEqual(next, after) {
			t.Errorf("after %s, Next differs from FromEvents:\n%+v\n%+v", e.Type, next, after)
		}
	}
}

func TestNextFoldsToWhatFromEventsGives(t *testing.T) {
	stream := seeded()
	stream = append(stream, everyAgentEvent(FromEvents(stream))...)
	var roster Roster
	for _, e := range stream {
		roster = roster.Next(e)
	}
	if want := FromEvents(stream); !reflect.DeepEqual(roster, want) {
		t.Errorf("folding Next:\n%+v\nFromEvents:\n%+v", roster, want)
	}
}

// everyAgentEvent is one event of each type, for agents of roster.
func everyAgentEvent(roster Roster) []events.Event {
	you := events.Actor{Kind: events.Human, ID: "you"}
	engineer, _ := roster.Get("engineer")
	tester, _ := roster.Get("tester")
	reviewer, _ := roster.Get("reviewer")
	designer := Agent{Name: "designer", Prompt: "Draw it.", Engine: "claude", Workers: []string{"laptop"}}
	engineer.Prompt = "Only write Go."
	return []events.Event{
		asEvent(designer.Proposal(you, false)),
		asEvent(engineer.Proposal(you, true)),
		asEvent(tester.Learned(you, "Run the slow tests too.")),
		asEvent(engineer.Renaming(you, "builder")),
		asEvent(reviewer.Removal(you)),
	}
}
