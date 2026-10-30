// Package storetest opens stores for tests the way serve does: the rules
// check each proposal against a current.State the store keeps up to date.
// It also grows synthetic logs from a copy of a real one, for benchmarks.
package storetest

import (
	"math/rand/v2"
	"slices"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/schedules"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// Open opens the log at path with the core's event types, the rules and a
// state. When the test ends it checks the state still equals a replay of
// the log, and that snapshots of it do, then closes it.
func Open(t testing.TB, path string) (*store.Store, *current.State) {
	t.Helper()
	return OpenWith(t, path, events.Core())
}

// OpenWith is Open with another catalogue, such as WithExtensions. The state
// keeps the streams of every extension serve installs, and the rules run the
// extensions' own, as serve does; under a catalogue without them, their
// streams are foreign.
func OpenWith(t testing.TB, path string, catalogue *events.Catalogue) (*store.Store, *current.State) {
	t.Helper()
	s, state := openUnchecked(t, path, catalogue)
	t.Cleanup(func() { s.Close() })
	t.Cleanup(func() { // cleanups run last first: before the close
		CheckEqual(t, s, state)
		CheckSnapshots(t, path, catalogue, s.LastPosition())
	})
	return s, state
}

// openUnchecked is OpenWith with no checks when the test ends, and no
// close: the test closes the store itself.
func openUnchecked(t testing.TB, path string, catalogue *events.Catalogue) (*store.Store, *current.State) {
	t.Helper()
	state := NewState(catalogue)
	s, err := store.Open(path, catalogue, state, func(p events.Proposal) *events.Rejection { return rules.Check(p, state, catalogue, Installed()...) })
	if err != nil {
		t.Fatal(err)
	}
	return s, state
}

// NewState is an empty state for a log opened with catalogue, as serve
// makes it: it keeps the streams of ideas and schedules whenever the
// catalogue registers them.
func NewState(catalogue *events.Catalogue) *current.State {
	return current.New(catalogue, extensions.StreamsOf(Installed()...)...)
}

// WithExtensions is the catalogue serve opens the log with: the core's event
// types, and those of ideas and schedules.
func WithExtensions() *events.Catalogue {
	catalogue, err := events.NewCatalogue(extensions.Registrations(Installed()...)...)
	if err != nil {
		panic(err)
	}
	return catalogue
}

// Installed are the extensions serve installs (cmd/agentry's
// installedExtensions), which a test log is opened with.
func Installed() []extensions.Extension {
	return []extensions.Extension{ideas.Extension(), schedules.Extension()}
}

// snapshotCuts is how many positions CheckSnapshots takes in a longer log:
// every position of a log this long or shorter.
const snapshotCuts = 40

// CheckSnapshots fails the test if a snapshot of the log at path, opened
// with catalogue, differs from a replay once opened, as serve opens it
// (current.EqualThroughSnapshots): at every position of a short log, and at
// a seeded sample of a longer one's and its last.
func CheckSnapshots(t testing.TB, path string, catalogue *events.Catalogue, last int64) {
	t.Helper()
	cuts := make([]int64, 0, snapshotCuts)
	if last <= snapshotCuts {
		for position := range last {
			cuts = append(cuts, position+1)
		}
	} else {
		sample := rand.New(rand.NewPCG(1, uint64(last)))
		for range snapshotCuts - 1 {
			cuts = append(cuts, 1+sample.Int64N(last))
		}
		cuts = append(cuts, last)
		slices.Sort(cuts)
	}
	if err := current.EqualThroughSnapshots(path, catalogue, t.TempDir(), slices.Compact(cuts), extensions.StreamsOf(Installed()...)...); err != nil {
		t.Errorf("a snapshot differs from a replay of the log: %v", err)
	}
}

// CheckEqual fails the test if the state differs from a replay of the log.
func CheckEqual(t testing.TB, s *store.Store, state *current.State) {
	t.Helper()
	if err := current.Equal(s, state); err != nil {
		t.Errorf("the state differs from a replay of the log: %v", err)
	}
}
