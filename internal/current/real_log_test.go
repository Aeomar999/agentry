package current

import (
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// Test 3: every prefix of a real log. On a copy of a real agentry.db, named
// by AGENTRY_BENCH_DB as for TestReadModelBench, a state takes the log one
// event at a time through Apply, as the store feeds it live. Every 50
// events, each stream those events touched must equal FromEvents over the
// same prefix; at the end, Equal compares everything, and no stream may be
// broken: a replay must never fail on an event the store accepted.
//
//	AGENTRY_BENCH_DB=<copy>/agentry.db go test ./internal/current -run TestARealLog -v -count=1
func TestARealLogAppliedEventByEventEqualsAReplay(t *testing.T) {
	source := os.Getenv("AGENTRY_BENCH_DB")
	if source == "" {
		t.Skip("AGENTRY_BENCH_DB is not set")
	}
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "agentry.db") // Open may write: never to the copy itself
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := store.Open(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	state := New()
	accepted := map[string][]events.Event{} // each stream's accepted events so far
	touched := map[string]bool{}
	applied := 0
	check := func(at events.Event) {
		for stream := range touched {
			if err := equalPrefix(state, stream, accepted[stream]); err != nil {
				t.Fatalf("after event %d (position %d, %s): %v", applied, at.Position, at.Type, err)
			}
		}
		clear(touched)
	}
	for position := int64(0); ; {
		batch, err := s.After(position, 500)
		if err != nil {
			t.Fatal(err)
		}
		if len(batch) == 0 {
			break
		}
		for _, e := range batch {
			state.Apply(e)
			if e.Type != events.ProposalRejected {
				accepted[e.Stream] = append(accepted[e.Stream], e)
			}
			touched[e.Stream] = true
			if applied++; applied%50 == 0 {
				check(e)
			}
		}
		position = batch[len(batch)-1].Position
	}
	if err := Equal(s, state); err != nil {
		t.Fatal(err)
	}
	for stream := range accepted {
		if err := state.Broken(stream); err != nil {
			t.Errorf("%s is broken: %v", stream, err)
		}
	}
	t.Logf("%d events applied one by one, %d streams, checked every 50", applied, len(accepted))
}

// equalPrefix compares the state of one stream with a replay of accepted.
func equalPrefix(state *State, stream string, accepted []events.Event) error {
	held := func(value any) func() (any, error) {
		return func() (any, error) { return value, state.Broken(stream) }
	}
	switch stream {
	case events.AgentsStream:
		return equalReplay(stream, accepted, agents.FromEvents, held(state.Roster()))
	case events.WorkersStream:
		return equalReplay(stream, accepted, workers.FromEvents, held(state.Workers()))
	case events.ConnectionsStream:
		return equalReplay(stream, accepted, connections.FromEvents, held(state.Connections()))
	case events.PolicyStream:
		return nil
	}
	if events.IsIdeaStream(stream) {
		return equalReplay(stream, accepted, ideas.FromEvents, func() (any, error) { return state.Idea(stream) })
	}
	return equalReplay(stream, accepted, tasks.FromEvents, func() (any, error) { return state.Task(stream) })
}
