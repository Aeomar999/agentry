// Package storetest opens stores for tests the way serve does: the rules
// check each proposal against a current.State the store keeps up to date.
// It also grows synthetic logs from a copy of a real one, for benchmarks.
package storetest

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// Open opens the log at path with the core's event types, the rules and a
// state. When the test ends it checks the state still equals a replay of
// the log, then closes it.
func Open(t testing.TB, path string) (*store.Store, *current.State) {
	t.Helper()
	return OpenWith(t, path, events.Core())
}

// OpenWith is Open with another catalogue, such as one with ideas registered.
// The state keeps the ideas' streams whenever the catalogue registers them.
func OpenWith(t testing.TB, path string, catalogue *events.Catalogue) (*store.Store, *current.State) {
	t.Helper()
	state := NewState(catalogue)
	s, err := store.Open(path, catalogue, state, func(p events.Proposal) *events.Rejection { return rules.Check(p, state, catalogue) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	t.Cleanup(func() { CheckEqual(t, s, state) }) // cleanups run last first: before the close
	return s, state
}

// NewState is an empty state for a log opened with catalogue, as serve
// makes it: it keeps the ideas' streams whenever the catalogue registers them.
func NewState(catalogue *events.Catalogue) *current.State {
	return current.New(catalogue, ideas.Streams())
}

// WithIdeas is the catalogue serve opens the log with: the core's event
// types and ideas'.
func WithIdeas() *events.Catalogue {
	catalogue, err := events.NewCatalogue(ideas.Registration())
	if err != nil {
		panic(err)
	}
	return catalogue
}

// CheckEqual fails the test if the state differs from a replay of the log.
func CheckEqual(t testing.TB, s *store.Store, state *current.State) {
	t.Helper()
	if err := current.Equal(s, state); err != nil {
		t.Errorf("the state differs from a replay of the log: %v", err)
	}
}
