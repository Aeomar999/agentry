package connections

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
	before, each := connectionLog()
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
	before, each := connectionLog()
	stream := append(before, each...)
	var roster Roster
	for _, e := range stream {
		roster = roster.Next(e)
	}
	if want := FromEvents(stream); !reflect.DeepEqual(roster, want) {
		t.Errorf("folding Next:\n%+v\nFromEvents:\n%+v", roster, want)
	}
}

// connectionLog is a roster's log so far, with one connection and an unused
// code, and one event of each type to apply to it.
func connectionLog() (before, each []events.Event) {
	now := time.Now()
	you := events.Actor{Kind: events.Human, ID: "you"}
	machine := Machine{Host: "laptop.lan", User: "dev", OS: "linux"}
	code, invited := Invite(you, now)
	_, made := Connect(code, machine)
	spare, invitedAgain := Invite(you, now)
	before = stored(now, invited, made, invitedAgain)

	_, invitedThird := Invite(you, now)
	_, madeDesk := Connect(spare, machine)
	each = stored(now, invitedThird, madeDesk, Revocation(you, made.Data["id"].(string)))
	return before, each
}

// stored turns proposals into events, as the log would give them back: at
// in UTC, with no monotonic reading.
func stored(at time.Time, proposals ...events.Proposal) []events.Event {
	var stream []events.Event
	for i, p := range proposals {
		stream = append(stream, events.Event{Position: int64(i + 1), At: at.UTC().Round(0), Proposal: p})
	}
	return stream
}

func grown(r Roster) Roster {
	return Roster{Connections: slices.Grow(r.Connections, 4), invites: slices.Grow(r.invites, 4)}
}

// whole is the roster with its slices out to their capacity.
func whole(r Roster) Roster {
	return Roster{Connections: r.Connections[:cap(r.Connections)], invites: r.invites[:cap(r.invites)]}
}
