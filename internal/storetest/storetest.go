// Package storetest opens stores for tests the way serve does: the rules
// check each proposal against a current.State the store keeps up to date.
package storetest

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// Open opens the log at path with the rules and a state, and closes it when
// the test ends.
func Open(t testing.TB, path string) (*store.Store, *current.State) {
	t.Helper()
	state := current.New()
	s, err := store.Open(path, state, func(p events.Proposal) *events.Rejection { return rules.Check(p, state) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s, state
}
