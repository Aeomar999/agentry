package storetest

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
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
	if count, rejection, err := s.Preview("task-1", pasted); count != 4 || rejection != nil || err != nil {
		t.Fatalf("the rows holding the secret: %d, %v, want the command, direction, report and rejection", count, err)
	}
	if count, rejection, err := s.Preview("task-1", short); count != 1 || rejection != nil || err != nil {
		t.Fatalf("the rows holding the short secret: %d, %v, want its direction", count, err)
	}
	// Into the file itself, so the rewrite changes pages already there: in
	// the WAL alone, the checkpoint would copy only the rewritten pages.
	if err := s.GiveBackSpace(); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{pasted, short} {
		if holding := FilesHolding(t, dir, secret); !slices.Contains(holding, "agentry.db") {
			t.Fatalf("before redacting, %q is in %v: want the log", secret, holding)
		}
	}

	redact(t, s, "task-1", pasted)
	redact(t, s, "task-1", short)
	if err := current.WriteSnapshot(filepath.Join(dir, "state.snapshot"), state, s, "build-1"); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{pasted, short} {
		if holding := FilesHolding(t, dir, secret); len(holding) != 0 {
			t.Errorf("after redacting, %q is still in %v", secret, holding)
		}
	}
	if holding := FilesHolding(t, dir, kept); len(holding) == 0 {
		t.Errorf("task-2's %q is in no file: the search can't see the live rows", kept)
	}
}

// Test 1b: a reader in the middle of a read keeps Redact from emptying the
// WAL, which still holds the old rows, and Redact says so; once the reader
// is done, GiveBackSpace, run again as serve and the routes run it,
// empties it, and the secret is in no byte on disk.
func TestASecretAReaderKeptInTheWALGoesWhenTheSpaceIsGivenBack(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentry.db")
	s, _ := Open(t, path)
	withPasted(t, s)
	reading := readingIn(t, path)
	_, rejection, err := s.Redact("task-1", pasted, you, "A token pasted by mistake.")
	if rejection != nil || err == nil || !strings.Contains(err.Error(), "the old text may stay in the WAL") {
		t.Fatalf("redacting with a reader in the WAL: %v, %v, want redacted and an error saying so", rejection, err)
	}
	if holding := FilesHolding(t, dir, pasted); !slices.Contains(holding, "agentry.db-wal") {
		t.Fatalf("with a reader in the WAL, the secret is in %v: want the WAL, or this test shows nothing", holding)
	}
	if err := reading.Rollback(); err != nil {
		t.Fatal(err)
	}
	if err := s.GiveBackSpace(); err != nil {
		t.Fatal(err)
	}
	if holding := FilesHolding(t, dir, pasted); len(holding) != 0 {
		t.Errorf("once the space is given back, the secret is still in %v", holding)
	}
}

// readingIn is a read under way on the log at path, from a connection of
// its own, which keeps a checkpoint from emptying the WAL until it ends.
func readingIn(t *testing.T, path string) *sql.Tx {
	t.Helper()
	db, err := sql.Open(store.Driver, path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	reading, err := db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { reading.Rollback() })
	var rows int
	if err := reading.QueryRow(`SELECT COUNT(*) FROM events`).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	return reading
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
		return rules.Check(p, resumed, events.Core(), ideas.Extension())
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
	if holding := FilesHolding(t, filepath.Dir(newer), pasted); len(holding) != 0 {
		t.Errorf("a snapshot of the resumed state holds the secret: %v", holding)
	}
	if holding := FilesHolding(t, dir, pasted); !reflect.DeepEqual(holding, []string{"older.snapshot"}) {
		t.Errorf("the secret is in %v, want only the snapshot from before", holding)
	}
}

// Test 4: an archived task and an erased one are refused for that, each
// saying why, before what their live rows hold: whether a marker holds
// the secret or no row does. Preview says the same, and the log is as it
// was.
func TestAnArchivedOrErasedTaskIsNotRedacted(t *testing.T) {
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	twoTasks(t, s)
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	ArchiveIn(t, s, state, "task-1", blobs)
	erase(t, s, "task-2", blobs)
	for _, c := range []struct{ stream, secret, want string }{
		{"task-1", "One, renamed", store.ArchivedRedactionRefusal("task-1")}, // its marker's summary holds the title
		{"task-1", "not-there", store.ArchivedRedactionRefusal("task-1")},
		{"task-2", "forgotten", "task-2 is erased: there is nothing left to redact"}, // its marker holds the reason
		{"task-2", "not-there", "task-2 is erased: there is nothing left to redact"},
	} {
		last := s.LastPosition()
		if count, rejection, err := s.Preview(c.stream, c.secret); count != 0 || err != nil || rejection == nil || rejection.Reason != c.want {
			t.Errorf("previewing %q in %s: %d, %v, %v, want %q", c.secret, c.stream, count, rejection, err, c.want)
		}
		if _, rejection, err := s.Redact(c.stream, c.secret, you, ""); err != nil || rejection == nil || rejection.Reason != c.want {
			t.Errorf("redacting %q in %s: %v, %v, want %q", c.secret, c.stream, rejection, err, c.want)
		}
		if s.LastPosition() != last {
			t.Errorf("a refused redaction of %s wrote to the log", c.stream)
		}
	}
}

// Test 5: with the rules on, as serve has them, a row with the secret in a
// key is refused for that, not for its redaction naming no fields, whether
// or not a value holds it too, and the log is as it was. Preview gives
// the same reason, with no rows.
func TestARowWithTheSecretInAKeyIsRefusedBeforeTheRules(t *testing.T) {
	for name, extra := range map[string]any{
		"only the key": map[string]any{pasted: "x"},
		"and a value":  map[string]any{pasted: "x " + pasted},
	} {
		s, _ := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
		for _, p := range append(agents.SeedProposals(nil),
			events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "One", "repo": "/r", "coordinator": "tech-lead", "extra": extra}},
		) {
			mustPropose(t, s, p)
		}
		last := s.LastPosition()
		want := "the row at " + strconv.FormatInt(last, 10) + " holds the secret in a key, which redacting doesn't rewrite"
		if count, rejection, err := s.Preview("task-1", pasted); count != 0 || err != nil || rejection == nil || rejection.Reason != want {
			t.Errorf("%s: Preview %d, %v, %v, want %q", name, count, rejection, err, want)
		}
		before, err := s.Stream("task-1")
		if err != nil {
			t.Fatal(err)
		}
		if _, rejection, err := s.Redact("task-1", pasted, you, ""); err != nil || rejection == nil || rejection.Reason != want {
			t.Errorf("%s: %v, %v, want %q", name, rejection, err, want)
		}
		after, err := s.Stream("task-1")
		if err != nil {
			t.Fatal(err)
		}
		if s.LastPosition() != last || !reflect.DeepEqual(after, before) {
			t.Errorf("%s: a refused redaction changed the log:\n%v\n%v", name, before, after)
		}
	}
}

// Test 6: with the rules on, a row that holds the secret only in a number,
// alone or beside a string holding it in another row, is refused for
// that, not for its redaction naming no fields: in a live task, in an
// agent's knowledge and in an archived task. A task's reason offers to
// erase it, the knowledge's only a longer piece of the text. Preview gives
// the same reason, and the log is as it was.
func TestASecretLeftInANumberIsRefusedBeforeTheRules(t *testing.T) {
	for _, c := range []struct {
		name, stream, secret, remedy string
		archived                     bool
		// held proposes the row holding the secret in a number, after
		// any others holding it, and returns its position.
		held func(*testing.T, *store.Store, *current.State) int64
	}{
		{"a live task, alone", "task-1", "7901", erasable, false, inAPort("One")},
		{"a live task, beside a string", "task-1", "7901", erasable, false, inAPort("PIN 7901")},
		{"an archived task", "task-1", "7901", erasable, true, inAPort("One")},
		{"knowledge", events.AgentsStream, "5151", "redact a longer piece of the text", false, func(t *testing.T, s *store.Store, state *current.State) int64 {
			engineer, _ := state.Roster().Get("engineer")
			learned := engineer.Learned(you, "plain")
			learned.Data["n"] = 5151
			return proposed(t, s, learned)
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
			for _, p := range agents.SeedProposals(nil) {
				mustPropose(t, s, p)
			}
			at := c.held(t, s, state)
			blobs := sharedfiles.Disk{Dir: t.TempDir()}
			preview, redact := s.Preview, func(stream, secret string) (events.Event, *events.Rejection, error) {
				return s.Redact(stream, secret, you, "")
			}
			if c.archived {
				ArchiveIn(t, s, state, c.stream, blobs)
				preview = func(stream, secret string) (int, *events.Rejection, error) {
					return s.PreviewArchived(stream, secret, blobs)
				}
				redact = func(stream, secret string) (events.Event, *events.Rejection, error) {
					return s.RedactArchived(stream, secret, you, "", summarise(state, stream), blobs)
				}
			}
			before, err := s.Stream(c.stream)
			if err != nil {
				t.Fatal(err)
			}
			last := s.LastPosition()
			want := "the row at " + strconv.FormatInt(at, 10) + " holds the secret outside its text, as in a number, which redacting can't rewrite: " + c.remedy
			if count, rejection, err := preview(c.stream, c.secret); count != 0 || err != nil || rejection == nil || rejection.Reason != want {
				t.Errorf("Preview %d, %v, %v, want %q", count, rejection, err, want)
			}
			if _, rejection, err := redact(c.stream, c.secret); err != nil || rejection == nil || rejection.Reason != want {
				t.Errorf("%v, %v, want %q", rejection, err, want)
			}
			after, err := s.Stream(c.stream)
			if err != nil {
				t.Fatal(err)
			}
			if s.LastPosition() != last || !reflect.DeepEqual(after, before) {
				t.Errorf("a refused redaction changed the log:\n%v\n%v", before, after)
			}
		})
	}
}

// erasable is what a refused redaction of a task offers instead.
const erasable = "erase the task, or redact a longer piece of the text"

// inAPort creates task-1 with title and has its worktree take port 7901,
// returning that row's position.
func inAPort(title string) func(*testing.T, *store.Store, *current.State) int64 {
	return func(t *testing.T, s *store.Store, _ *current.State) int64 {
		mustPropose(t, s, events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": title, "repo": "/r", "coordinator": "tech-lead"}})
		return proposed(t, s, events.Proposal{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "agentry/task-1", "port": 7901}})
	}
}

// proposed proposes p, which must be accepted, and returns its position.
func proposed(t *testing.T, s *store.Store, p events.Proposal) int64 {
	t.Helper()
	e, rejection, err := s.Propose(p)
	if rejection != nil || err != nil {
		t.Fatalf("proposing %s on %s: %v, %v", p.Type, p.Stream, rejection, err)
	}
	return e.Position
}
