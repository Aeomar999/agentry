package storetest

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// Redacting, as the state and the files on disk see it
// (docs/design/log-growth.md section 5, "Removing a secret"). The store's
// side, the trigger, the rewrite and its refusals, is in store's
// redact_test.go.

// pasted is the secret pasted into task-1, and nowhere else.
const pasted = "pasted-c41e77ab"

// short is a second secret in task-1, shorter than [redacted]: its cell
// grows and moves, which leaves the old one in a freeblock on its page.
const short = "Qz7#k"

// kept is task-2's text, which no redaction touches.
const kept = "kept-90b2"

func redact(t testing.TB, s *store.Store, stream, secret string) events.Event {
	t.Helper()
	event, rejection, err := s.Redact(stream, secret, you, "A token pasted by mistake.")
	if rejection != nil || err != nil {
		t.Fatalf("redacting %s: %v, %v", stream, rejection, err)
	}
	return event
}

// withPasted seeds the roster and gives task-1 the secret in a command, a
// direction long enough to need overflow pages, a report and a rejection
// quoting it, and the short secret in a direction of its own; task-2's
// rows sit among them on the same pages, and its last row is the log's.
func withPasted(t testing.TB, s *store.Store) {
	t.Helper()
	engineer := events.Actor{Kind: events.Agent, ID: "engineer"}
	for _, p := range append(agents.SeedProposals(nil),
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "One", "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Two " + kept, "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"}, Type: events.TaskAssigned, Data: map[string]any{"agent": "engineer", "instructions": "Deploy it."}},
		events.Proposal{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "agentry/task-1", "port": 7901}},
		events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "engineer", "hash": "h-1"}},
		events.Proposal{Stream: "task-1", Actor: engineer, Type: events.ActionRequested, Correlation: "toolu_1", Data: map[string]any{"tool": "Bash", "command": "curl -H 'Authorization: Bearer " + pasted + "' https://example.test"}},
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskDirected, Data: map[string]any{"text": strings.Repeat("Use the token "+pasted+". ", 400)}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Two, renamed " + kept}},
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskDirected, Data: map[string]any{"text": "The PIN is " + short + "."}},
		events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "engineer", "report": "Deployed with " + pasted + "."}},
	) {
		mustPropose(t, s, p)
	}
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: you, Type: events.FileRemoved, Data: map[string]any{"name": pasted + ".txt"}}); rejection == nil || err != nil {
		t.Fatal("removing a file task-1 doesn't have should be refused:", rejection, err)
	}
	mustPropose(t, s, events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Two, last " + kept}})
}

// Test 1: once a secret is redacted, it is in no byte of the log or its
// WAL, nor in a snapshot written after: not in the command, the
// direction, the report or the rejection. task-2's rows share their
// pages, so the old rows' bytes stay on them unless secure_delete zeroes
// them; and task-2's text is still found, so the search reads the live
// rows. The short secret's cell grows rather than shrinks in place, and
// its old bytes go too.
func TestARedactedSecretIsInNoByteOnDisk(t *testing.T) {
	dir := t.TempDir()
	s, state := Open(t, filepath.Join(dir, "agentry.db"))
	withPasted(t, s)
	if count, err := s.Matches("task-1", pasted); count != 4 || err != nil {
		t.Fatalf("the rows holding the secret: %d, %v, want the command, direction, report and rejection", count, err)
	}
	if count, err := s.Matches("task-1", short); count != 1 || err != nil {
		t.Fatalf("the rows holding the short secret: %d, %v, want its direction", count, err)
	}
	// Into the file itself, so the rewrite changes pages already there: in
	// the WAL alone, the checkpoint would copy only the rewritten pages.
	if err := s.GiveBackSpace(); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{pasted, short} {
		if holding := filesHolding(t, dir, secret); !slices.Contains(holding, "agentry.db") {
			t.Fatalf("before redacting, %q is in %v: want the log", secret, holding)
		}
	}

	redact(t, s, "task-1", pasted)
	redact(t, s, "task-1", short)
	if err := current.WriteSnapshot(filepath.Join(dir, "state.snapshot"), state, s, "build-1"); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{pasted, short} {
		if holding := filesHolding(t, dir, secret); len(holding) != 0 {
			t.Errorf("after redacting, %q is still in %v", secret, holding)
		}
	}
	if holding := filesHolding(t, dir, kept); len(holding) == 0 {
		t.Errorf("task-2's %q is in no file: the search can't see the live rows", kept)
	}
}

// Test 2: the state after a redaction is the one a replay of the
// rewritten log gives, live and after a restart, and shows [redacted]
// where the secret was; the redaction itself changes nothing. (Open's own cleanup checks the same of a replay
// and of a snapshot at every position.)
func TestARedactionReplaysAsItWasLive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, live := Open(t, path)
	withPasted(t, s)
	before, err := live.Task("task-1")
	if err != nil {
		t.Fatal(err)
	}
	redact(t, s, "task-1", pasted)
	CheckEqual(t, s, live)

	task, err := live.Task("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if !task.LastActiveAt.Equal(before.LastActiveAt) {
		t.Errorf("task-1 was last active at %v after the redaction, want %v: a redaction is no activity", task.LastActiveAt, before.LastActiveAt)
	}
	if held := holds(t, task, pasted); held {
		t.Error("task-1 still holds the secret")
	}
	if !holds(t, task, "Use the token [redacted].") || !holds(t, task, "Bearer [redacted]") || !holds(t, task, "Deployed with [redacted].") {
		t.Errorf("task-1 should show [redacted] where the secret was: %+v", task)
	}

	_, restarted := Open(t, path)
	again, err := restarted.Task("task-1")
	if err != nil || !reflect.DeepEqual(again, task) {
		t.Errorf("task-1 after a restart: %+v, %v, want %+v", again, err, task)
	}
}

// holds says whether text is anywhere in what value shows of itself.
func holds(t *testing.T, value any, text string) bool {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Contains(string(encoded), text)
}

// Test 3: a state resumed from a snapshot taken before a redaction, opened
// on the redacted log, goes on from the snapshot, and the tail's
// redaction folds task-1 again from the rewritten rows: it equals a
// replay, and a snapshot of it holds no copy of the secret. The old one
// still does, which is why Redact's caller writes a new one.
func TestASnapshotFromBeforeARedactionGoesOnToTheRedactedLog(t *testing.T) {
	dir := t.TempDir()
	path, older := filepath.Join(dir, "agentry.db"), filepath.Join(dir, "older.snapshot")
	s, state := Open(t, path)
	withPasted(t, s)
	if err := current.WriteSnapshot(older, state, s, "build-1"); err != nil {
		t.Fatal(err)
	}
	taken := s.LastPosition()
	redact(t, s, "task-1", pasted)

	resumed := current.Restore(older, "build-1", events.Core(), ideas.Streams())
	again, err := store.Open(path, events.Core(), resumed, func(p events.Proposal) *events.Rejection {
		return rules.Check(p, resumed, events.Core(), ideas.Feature())
	})
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if resumed.Resumed() != taken {
		t.Fatalf("Open went on from %d, want the snapshot's %d", resumed.Resumed(), taken)
	}
	CheckEqual(t, again, resumed)
	newer := filepath.Join(t.TempDir(), "state.snapshot")
	if err := current.WriteSnapshot(newer, resumed, again, "build-1"); err != nil {
		t.Fatal(err)
	}
	if holding := filesHolding(t, filepath.Dir(newer), pasted); len(holding) != 0 {
		t.Errorf("a snapshot of the resumed state holds the secret: %v", holding)
	}
	if holding := filesHolding(t, dir, pasted); !reflect.DeepEqual(holding, []string{"older.snapshot"}) {
		t.Errorf("the secret is in %v, want only the snapshot from before", holding)
	}
}

// Test 4: Redact asks the rules, so an archived task and an erased one
// are refused, each saying why, and the log is as it was.
func TestAnArchivedOrErasedTaskIsNotRedacted(t *testing.T) {
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	twoTasks(t, s)
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	ArchiveIn(t, s, state, "task-1", blobs)
	erase(t, s, "task-2", blobs)
	for stream, want := range map[string]string{
		"task-1": rules.ArchivedRedactionRefusal("task-1"), // its marker's summary holds the title
		"task-2": rules.ErasedRefusal("task-2"),            // its marker holds the reason
	} {
		secret := map[string]string{"task-1": "One, renamed", "task-2": "forgotten"}[stream]
		last := s.LastPosition()
		if _, rejection, err := s.Redact(stream, secret, you, ""); err != nil || rejection == nil || rejection.Reason != want {
			t.Errorf("redacting %s: %v, %v, want %q", stream, rejection, err, want)
		}
		if s.LastPosition() != last {
			t.Errorf("a refused redaction of %s wrote to the log", stream)
		}
	}
}
