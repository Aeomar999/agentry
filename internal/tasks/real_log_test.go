package tasks_test

import (
	"os"
	"path/filepath"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// TestARealLogReplays replays every task stream of a copy of a real
// agentry.db and fails on any it can't replay: apply must take every event
// the rules ever accepted. It is skipped unless AGENTRY_BENCH_DB names the
// copy, which it copies again so the run never writes to it:
//
//	D=$(mktemp -d); sqlite3 "file:$HOME/.agentry/agentry.db?mode=ro" "VACUUM INTO '$D/agentry.db'"
//	AGENTRY_BENCH_DB=$D/agentry.db go test ./internal/tasks -run TestARealLogReplays -v -count=1
func TestARealLogReplays(t *testing.T) {
	source := os.Getenv("AGENTRY_BENCH_DB")
	if source == "" {
		t.Skip("AGENTRY_BENCH_DB is not set")
	}
	content, err := os.ReadFile(source)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "agentry.db")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	catalogue := storetest.WithIdeas()
	state := current.New(catalogue, ideas.Streams()) // only for the task order: every stream is replayed below
	s, err := store.Open(path, catalogue, state, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	ids := state.TaskIDs()
	for _, id := range ids {
		stream, err := s.Stream(id)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tasks.Replay(stream); err != nil {
			t.Errorf("%s (%d events): %v", id, len(stream), err)
		}
	}
	t.Logf("replayed %d task streams", len(ids))
}
