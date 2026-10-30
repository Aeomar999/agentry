package workers

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// stored turns proposals into events, as the log would give them back: at
// in UTC, with no monotonic reading.
func stored(at time.Time, proposals ...events.Proposal) []events.Event {
	var stream []events.Event
	for i, p := range proposals {
		stream = append(stream, events.Event{Position: int64(i + 1), At: at.UTC().Round(0), Proposal: p})
	}
	return stream
}

func TestAMachineJoinsOnceWithItsCode(t *testing.T) {
	now := time.Now()
	you := events.Actor{Kind: events.Human, ID: "you"}
	code, invited := Invite(you, now)
	roster := FromEvents(stored(now, invited))
	if !roster.CanJoin(Hash(code), now) {
		t.Fatal("a fresh code should let a machine join")
	}
	if roster.CanJoin(Hash("not-the-code"), now) || roster.CanJoin(Hash(code), now.Add(InviteLifetime+time.Second)) {
		t.Error("a wrong or expired code let a machine join")
	}

	machine := Machine{Host: "laptop.lan", OS: "linux", Arch: "amd64", Engines: []string{"claude 2.1.301"}}
	credential, joined := Join("laptop", code, machine)
	roster = FromEvents(stored(now, invited, joined, Connected("laptop", machine)))
	if roster.CanJoin(Hash(code), now) {
		t.Error("a used code let a second machine join")
	}
	worker, ok := roster.WithCredential(credential)
	if !ok || worker.Name != "laptop" || !worker.Online || !worker.Has("claude") || worker.Has("codex") {
		t.Errorf("the worker by its credential: %+v, %v", worker, ok)
	}
	if _, ok := roster.WithCredential(Hash(credential)); ok {
		t.Error("the credential's hash, which the log keeps, let a worker in")
	}
	for _, e := range joined.Data {
		if e == credential || e == code {
			t.Error("the log holds a secret as it is, not its hash")
		}
	}

	roster = FromEvents(stored(now, invited, joined, Connected("laptop", machine), Disconnected("laptop"), Removal(you, "laptop")))
	if _, ok := roster.WithCredential(credential); ok || len(roster.Workers) != 0 {
		t.Error("a removed worker's credential still works")
	}
}
