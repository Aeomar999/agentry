package storetest

import (
	"math/rand/v2"
	"path/filepath"
	"sync"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
)

// Test 7: readers call every getter of the state, and the store's own
// reads, while random proposals run through Propose (run with -race). A
// reader that sees the log at N finds the state at N or later.
func TestReadersRacePropose(t *testing.T) {
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		if _, _, err := s.Propose(p); err != nil {
			t.Fatal(err)
		}
	}
	var readers sync.WaitGroup
	done := make(chan struct{})
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				if !readEverything(t, s, state) {
					return
				}
			}
		})
	}
	// Before the store closes in cleanup, even when t.Fatal stops the test.
	defer func() { close(done); readers.Wait() }()
	g := &generator{rand: rand.New(rand.NewPCG(7, 7)), state: state, codes: []string{"a-code-nobody-made"}}
	for range 300 {
		if _, _, err := s.Propose(g.next()); err != nil {
			t.Fatal(err)
		}
	}
}

// readEverything reports whether the reads passed: a reader that failed
// once stops, so it doesn't log the same failure on every pass.
func readEverything(t *testing.T, s *store.Store, state *current.State) bool {
	logAt := s.LastPosition()
	if position := state.Position(); position < logAt {
		t.Errorf("the log is at %d but the state only at %d", logAt, position)
		return false
	}
	if _, err := s.After(logAt-5, 10); err != nil {
		t.Error(err)
		return false
	}
	for _, task := range state.Tasks() {
		_ = len(task.Trail) + len(task.Turns)
	}
	if task, _ := state.Task("task-1"); task != nil {
		_ = task.Title + string(task.Status())
	}
	_ = len(state.Roster()) + len(state.Workers().Workers) + len(state.Connections().Connections)
	_ = state.Broken(events.AgentsStream)
	return true
}
