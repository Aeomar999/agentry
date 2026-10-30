package workers

import (
	"reflect"
	"slices"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// The roster has spare capacity, so an append could write where a reader of
// the roster before would see it, and the whole arrays are compared.
func TestNextLeavesTheRosterAsItWas(t *testing.T) {
	before, each := workerLog()
	for _, e := range each {
		roster, want := grown(FromEvents(before)), grown(FromEvents(before))
		next := roster.Next(e)
		if !reflect.DeepEqual(whole(roster), whole(want)) {
			t.Errorf("%s changed the roster it was applied to", e.Type)
		}
		if after := FromEvents(append(slices.Clip(before), e)); !reflect.DeepEqual(next, after) {
			t.Errorf("after %s, Next differs from FromEvents:\n%+v\n%+v", e.Type, next, after)
		}
	}
}

func TestNextFoldsToWhatFromEventsGives(t *testing.T) {
	before, each := workerLog()
	stream := append(before, each...)
	var roster Roster
	for _, e := range stream {
		roster = roster.Next(e)
	}
	if want := FromEvents(stream); !reflect.DeepEqual(roster, want) {
		t.Errorf("folding Next:\n%+v\nFromEvents:\n%+v", roster, want)
	}
}

// workerLog is a roster's log so far, with a worker online and an unused
// code, and one event of each type to apply to it.
func workerLog() (before, each []events.Event) {
	now := time.Now()
	you := events.Actor{Kind: events.Human, ID: "you"}
	machine := Machine{Host: "laptop.lan", OS: "linux", Arch: "amd64", Engines: []string{"claude 2.1.301"}}
	code, invited := Invite(you, now)
	_, joined := Join("laptop", code, machine)
	spare, invitedAgain := Invite(you, now)
	before = stored(now, invited, joined, Connected("laptop", machine), invitedAgain)

	_, invitedThird := Invite(you, now)
	_, joinedDesk := Join("desk", spare, machine)
	machine.Version, machine.Missing = "1.2.0", []string{"git"}
	each = stored(now, invitedThird, joinedDesk, Connected("laptop", machine), Disconnected("laptop"), Removal(you, "laptop"))
	return before, each
}

func grown(r Roster) Roster {
	return Roster{Workers: slices.Grow(r.Workers, 4), invites: slices.Grow(r.invites, 4)}
}

// whole is the roster with its slices out to their capacity.
func whole(r Roster) Roster {
	return Roster{Workers: r.Workers[:cap(r.Workers)], invites: r.invites[:cap(r.invites)]}
}
