package current

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/connections"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/settings"
)

// Test 3: every prefix of a real log. On a copy of a real agentry.db, named
// by AGENTRY_BENCH_DB as for TestReadModelBench, a state takes the log one
// event at a time through Apply, as the store feeds it live. Every 50
// events, each stream those events touched must equal a replay of the
// same prefix; at the end, Equal compares everything, and no stream may be
// broken: a replay must never fail on an event the store accepted.
//
//	AGENTRY_BENCH_DB=<copy>/agentry.db go test ./internal/core/current -run TestARealLog -v -count=1
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
	s, err := store.Open(path, withIdeas, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	state := New(withIdeas, ideas.Streams())
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
		return equalReplay(stream, accepted, infallible(agents.FromEvents), held(state.Roster()))
	case events.WorkersStream:
		return equalReplay(stream, accepted, infallible(workers.FromEvents), held(state.Workers()))
	case events.ConnectionsStream:
		return equalReplay(stream, accepted, infallible(connections.FromEvents), held(state.Connections()))
	case events.SettingsStream:
		return equalReplay(stream, accepted, infallible(settings.FromEvents), held(state.Settings()))
	case events.TemplatesStream:
		return equalReplay(stream, accepted, infallible(agents.UserTemplatesFromEvents), held(state.Templates()))
	case events.PolicyStream:
		return nil
	}
	if ideas.IsStream(stream) {
		return equalReplay(stream, accepted, infallible(ideas.FromEvents), func() (any, error) { return ideas.Get(state, stream) })
	}
	return equalReplay(stream, accepted, tasks.Replay, func() (any, error) { return state.Task(stream) })
}

// Equivalence test 2, snapshots of a real log: on a copy named by
// AGENTRY_BENCH_DB, the state at every 500th position and at the first
// event of each type goes through a snapshot and equals a replay (about 300
// cuts on a log of 150,000 events, so expect minutes). At the last cut it
// logs the snapshot's size and how long a write and a load take, a load
// being Restore and Open with no tail.
//
//	AGENTRY_BENCH_DB=<copy>/agentry.db go test ./internal/core/current -run TestARealLogThroughASnapshot -v -count=1 -timeout 60m
func TestARealLogThroughASnapshotEqualsAReplay(t *testing.T) {
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
	throughSnapshots(t, path, 500)
}

// The same on a fixture log, cut every 5 positions, so a change that
// breaks the test shows without AGENTRY_BENCH_DB.
func TestAFixtureLogThroughASnapshotEqualsAReplay(t *testing.T) {
	path, _ := seededLog(t, someProposals()...)
	throughSnapshots(t, path, 5)
}

// throughSnapshots is equivalence test 2 on the log at path, cut every
// `every` positions.
func throughSnapshots(t *testing.T, path string, every int64) {
	s, err := store.Open(path, withIdeas, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	all, err := everyEvent(s)
	s.Close()
	if err != nil {
		t.Fatal(err)
	}
	last := all[len(all)-1].Position
	cuts := cutsOf(all, every)
	if err := EqualThroughSnapshots(path, withIdeas, t.TempDir(), cuts, ideas.Streams()); err != nil {
		t.Fatal(err)
	}

	state := New(withIdeas, ideas.Streams())
	s, err = store.Open(path, withIdeas, state, nil)
	if err != nil {
		t.Fatal(err)
	}
	snapshot := filepath.Join(t.TempDir(), "state.snapshot")
	started := time.Now()
	if err := WriteSnapshot(snapshot, state, s, "bench"); err != nil {
		t.Fatal(err)
	}
	wrote := time.Since(started)
	s.Close()
	info, err := os.Stat(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	started = time.Now()
	restored := Restore(snapshot, "bench", withIdeas, ideas.Streams())
	read := time.Since(started)
	s, err = store.Open(path, withIdeas, restored, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	loaded := time.Since(started)
	if restored.resumed != last {
		t.Fatalf("Open replayed the whole log instead of going on from the snapshot at %d", last)
	}
	for _, stream := range []string{events.AgentsStream, events.WorkersStream, events.ConnectionsStream, events.SettingsStream, events.TemplatesStream} {
		if err := restored.Broken(stream); err != nil {
			t.Errorf("%s is broken: %v", stream, err)
		}
	}
	t.Logf("%d events, %d cuts equal a replay; the snapshot at %d: %.1f MB, written in %s, read in %s, opened (read, checked, rebuilt) in %s",
		last, len(cuts), last, float64(info.Size())/1e6, wrote.Round(time.Millisecond), read.Round(time.Millisecond), loaded.Round(time.Millisecond))
}

// cutsOf is every `every`th position of log, the first event of each type
// in it and its last event, in order.
func cutsOf(log []events.Event, every int64) []int64 {
	var cuts []int64
	seen := map[events.Type]bool{}
	for i, e := range log {
		if e.Position%every == 0 || !seen[e.Type] || i == len(log)-1 {
			cuts = append(cuts, e.Position)
		}
		seen[e.Type] = true
	}
	return cuts
}
