package store

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
)

// The schema's tests write the markers and run the deletes themselves, as
// Store.Archive and Store.Unarchive do: the marker and the delete, or the
// rows and the unarchiving, in one transaction. Archive's own follow them;
// Unarchive's are in unarchive_test.go.

const blob = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

// marker is a task.archived covering stream's rows from created to through.
func marker(stream string, created, through int64, count int) events.Proposal {
	return events.Proposal{Stream: stream, Actor: archiver, Type: events.TaskArchived,
		Data: map[string]any{"through": through, "count": count, "archive": blob, "created": created, "summary": map[string]any{"title": "Remove the TODOs"}}}
}

func unarchived(stream string, marker int64) events.Proposal {
	return events.Proposal{Stream: stream, Actor: archiver, Type: events.TaskUnarchived,
		Data: map[string]any{"marker": marker, "archive": blob}}
}

// inTransaction runs change in one transaction and commits it.
func inTransaction(t *testing.T, s *Store, change func(tx *sql.Tx)) {
	t.Helper()
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	change(tx)
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func appendIn(t *testing.T, tx *sql.Tx, p events.Proposal) events.Event {
	t.Helper()
	e, err := appendEvent(tx, p)
	if err != nil {
		t.Fatal(err)
	}
	return e
}

// deleteIn deletes stream's rows from position from to through and says
// how many went, or the error.
func deleteIn(tx *sql.Tx, stream string, from, through int64) (int64, error) {
	result, err := tx.Exec(`DELETE FROM events WHERE stream = ? AND position BETWEEN ? AND ?`, stream, from, through)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func refusedAsAppendOnly(err error) bool {
	return err != nil && strings.Contains(err.Error(), "events are append-only")
}

// acceptAll proposes each, and fails the test on any refusal.
func acceptAll(t *testing.T, s *Store, proposals ...events.Proposal) {
	t.Helper()
	for _, p := range proposals {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
}

func renamed(stream string) events.Proposal {
	return events.Proposal{Stream: stream, Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Remove every TODO"}}
}

func TestAStrayDeleteIsRefused(t *testing.T) {
	s := openTestStore(t)
	acceptAll(t, s, newTask("task-1"), renamed("task-1"))
	for _, statement := range []string{`DELETE FROM events`, `DELETE FROM events WHERE position = 1`} {
		if _, err := s.db.Exec(statement); !refusedAsAppendOnly(err) {
			t.Errorf("%s: %v, want refused", statement, err)
		}
	}
}

// The marker allows deleting exactly the rows it covers, in its own
// transaction before it commits: none past "through". (None of another
// stream either: TestAMarkerAllowsNoOtherStreamsRows.)
func TestAMarkerAllowsDeletingOnlyTheRowsItCovers(t *testing.T) {
	s := openTestStore(t)
	acceptAll(t, s, newTask("task-1"), renamed("task-1"), newTask("task-2"), renamed("task-1"))
	// task-1 is at 1, 2 and 4; the marker covers 1 and 2.
	inTransaction(t, s, func(tx *sql.Tx) {
		if _, err := deleteIn(tx, "task-1", 1, 2); !refusedAsAppendOnly(err) {
			t.Errorf("before the marker: %v, want refused", err)
		}
		appendIn(t, tx, marker("task-1", 1, 2, 2))
		if _, err := tx.Exec(`UPDATE events SET data = '{}' WHERE position = 1`); !refusedAsAppendOnly(err) {
			t.Errorf("updating a row the marker covers: %v, want refused", err)
		}
		if deleted, err := deleteIn(tx, "task-1", 1, 2); err != nil || deleted != 2 {
			t.Errorf("the rows the marker covers: %d deleted, %v", deleted, err)
		}
		if _, err := deleteIn(tx, "task-1", 4, 4); !refusedAsAppendOnly(err) {
			t.Errorf("a row past through: %v, want refused", err)
		}
	})
	var left []int64
	rows, err := s.db.Query(`SELECT position FROM events ORDER BY position`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var position int64
		rows.Scan(&position)
		left = append(left, position)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if want := []int64{3, 4, 5}; !slices.Equal(left, want) {
		t.Errorf("rows left %v, want %v (task-2, task-1's last, the marker)", left, want)
	}
}

// A marker allows only its own stream's rows: another task's row that
// sits between the marked task's rows, inside "through", stays.
func TestAMarkerAllowsNoOtherStreamsRows(t *testing.T) {
	s := openTestStore(t)
	acceptAll(t, s, newTask("task-1"), newTask("task-2"), renamed("task-1"))
	// task-1 is at 1 and 3, task-2 at 2; the marker covers 1 to 3.
	inTransaction(t, s, func(tx *sql.Tx) {
		appendIn(t, tx, marker("task-1", 1, 3, 2))
		if _, err := deleteIn(tx, "task-2", 2, 2); !refusedAsAppendOnly(err) {
			t.Errorf("task-2's row inside task-1's marker: %v, want refused", err)
		}
	})
}

// Once task.unarchived follows the marker, its rows are the task's again:
// the delete the marker allowed is refused. A second marker after that
// allows it again.
func TestUnarchivingEndsWhatTheMarkerAllows(t *testing.T) {
	s := openTestStore(t)
	acceptAll(t, s, newTask("task-1"), renamed("task-1"))
	var first events.Event
	inTransaction(t, s, func(tx *sql.Tx) {
		first = appendIn(t, tx, marker("task-1", 1, 2, 2))
		appendIn(t, tx, unarchived("task-1", first.Position))
	})
	inTransaction(t, s, func(tx *sql.Tx) {
		if _, err := deleteIn(tx, "task-1", 1, 2); !refusedAsAppendOnly(err) {
			t.Errorf("after task.unarchived: %v, want refused", err)
		}
	})
	inTransaction(t, s, func(tx *sql.Tx) {
		second := appendIn(t, tx, marker("task-1", 1, first.Position+1, 4))
		if deleted, err := deleteIn(tx, "task-1", 1, second.Position-1); err != nil || deleted != 4 {
			t.Errorf("archived again: %d deleted, %v, want the 4 rows before the second marker", deleted, err)
		}
	})
}

// versionZero is the schema every build before the migrations made: the
// ungated delete trigger, no user_version.
const versionZero = `
CREATE TABLE events (
	position    INTEGER PRIMARY KEY AUTOINCREMENT,
	at          TEXT NOT NULL,
	stream      TEXT NOT NULL,
	actor_kind  TEXT NOT NULL,
	actor_id    TEXT NOT NULL,
	type        TEXT NOT NULL,
	correlation TEXT NOT NULL DEFAULT '',
	data        TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX events_by_stream ON events (stream, position);
CREATE TRIGGER events_no_update BEFORE UPDATE ON events
	BEGIN SELECT RAISE(ABORT, 'events are append-only'); END;
CREATE TRIGGER events_no_delete BEFORE DELETE ON events
	BEGIN SELECT RAISE(ABORT, 'events are append-only'); END;
INSERT INTO events (at, stream, actor_kind, actor_id, type, data) VALUES
	('2026-10-19T09:00:00Z', 'task-1', 'human', 'you', 'task.created', '{"title":"Remove the TODOs","repo":"/src/app","coordinator":"tech-lead"}'),
	('2026-10-19T09:00:01Z', 'task-1', 'human', 'you', 'task.renamed', '{"title":"Remove every TODO"}'),
	('2026-10-19T09:00:02Z', 'task-2', 'human', 'you', 'task.created', '{"title":"Fix the list","repo":"/src/app","coordinator":"tech-lead"}');
`

// schemaOf reads what a test checks of a file's schema with a connection
// of its own: user_version, auto_vacuum and the delete trigger's SQL.
func schemaOf(t *testing.T, path string) (version, autoVacuum int, trigger string) {
	t.Helper()
	db, err := sql.Open(Driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`PRAGMA auto_vacuum`).Scan(&autoVacuum); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = 'events_no_delete'`).Scan(&trigger); err != nil {
		t.Fatal(err)
	}
	return version, autoVacuum, trigger
}

func execOn(t *testing.T, path, statements string) {
	t.Helper()
	db, err := sql.Open(Driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(statements); err != nil {
		t.Fatal(err)
	}
}

// A log made before the migrations opens at version 1, with the gated
// trigger and every event as it was; opening it again runs nothing.
func TestAVersionZeroLogMigratesOnce(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	execOn(t, path, versionZero)

	s, model := openAt(t, path)
	log, err := s.After(0, 10)
	if err != nil || len(log) != 3 || log[1].Text("title") != "Remove every TODO" || !model.has("task-2") {
		t.Errorf("the events after migrating: %v %v", log, err)
	}
	if _, err := s.db.Exec(`DELETE FROM events WHERE position = 1`); !refusedAsAppendOnly(err) {
		t.Errorf("a stray delete after migrating: %v, want refused", err)
	}
	s.Close()
	version, _, trigger := schemaOf(t, path)
	if version != 1 || !strings.Contains(trigger, "task.archived") {
		t.Fatalf("version %d, trigger %s; want 1 and the gated trigger", version, trigger)
	}

	// Had the migration run again, it would have put back the gated trigger.
	sentinel := `CREATE TRIGGER events_no_delete BEFORE DELETE ON events
	BEGIN SELECT RAISE(ABORT, 'events are append-only: sentinel'); END`
	execOn(t, path, `DROP TRIGGER events_no_delete; `+sentinel)
	again, _ := openAt(t, path)
	again.Close()
	if version, _, trigger := schemaOf(t, path); version != 1 || trigger != sentinel {
		t.Errorf("after a second open: version %d, trigger %s; the migration ran again", version, trigger)
	}
}

func TestANewLogStartsAtTheLatestSchema(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	s.Close()
	version, autoVacuum, trigger := schemaOf(t, path)
	if version != len(migrations) || autoVacuum != 2 || !strings.Contains(trigger, "task.archived") {
		t.Errorf("version %d, auto_vacuum %d, trigger %s; want %d, 2 (incremental) and the gated trigger", version, autoVacuum, trigger, len(migrations))
	}
}

// A log from a newer build isn't opened: its schema may refuse what this
// build would write, or allow what it shouldn't.
func TestALogFromANewerBuildIsNotOpened(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	s.Close()
	execOn(t, path, `PRAGMA user_version = 99`)
	if s, err := Open(path, withNotes, nil, nil); err == nil || !strings.Contains(err.Error(), "schema version 99") {
		if err == nil {
			s.Close()
		}
		t.Errorf("opening a version 99 log: %v", err)
	}
}

// A task counts once for the next id however often it was archived: the
// next task never takes an archived task's id.
func TestCreateCountsArchivedTasksOnce(t *testing.T) {
	s := openTestStore(t)
	create := func(want string) {
		t.Helper()
		made, rejection, err := s.Create(events.TaskPrefix, newTask(""))
		if err != nil || rejection != nil || made.Stream != want {
			t.Fatalf("Create: %s %v %v, want %s", made.Stream, rejection, err, want)
		}
	}
	create("task-1")
	create("task-2")
	acceptAll(t, s, renamed("task-1")) // task-1 at 1 and 3
	archive := func() {
		t.Helper()
		inTransaction(t, s, func(tx *sql.Tx) {
			var through int64
			var count int
			if err := tx.QueryRow(`SELECT MAX(position), COUNT(*) FROM events WHERE stream = 'task-1'`).Scan(&through, &count); err != nil {
				t.Fatal(err)
			}
			appendIn(t, tx, marker("task-1", 1, through, count))
			if deleted, err := deleteIn(tx, "task-1", 1, through); err != nil || deleted != int64(count) {
				t.Fatalf("archiving task-1: %d deleted, %v, want %d", deleted, err, count)
			}
		})
	}

	archive()
	create("task-3")

	// The rows back with their own positions, as Store.Unarchive puts them:
	// task.created at 1 again.
	inTransaction(t, s, func(tx *sql.Tx) {
		var at int64
		if err := tx.QueryRow(`SELECT position FROM events WHERE type = 'task.archived'`).Scan(&at); err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(`INSERT INTO events (position, at, stream, actor_kind, actor_id, type, data) VALUES
			(1, '2026-10-19T09:00:00Z', 'task-1', 'human', 'you', 'task.created', '{"title":"Remove the TODOs","repo":"/src/app","coordinator":"tech-lead"}'),
			(3, '2026-10-19T09:00:01Z', 'task-1', 'human', 'you', 'task.renamed', '{"title":"Remove every TODO"}')`); err != nil {
			t.Fatal(err)
		}
		appendIn(t, tx, unarchived("task-1", at))
	})
	create("task-4")

	archive() // its second marker names the same task.created
	create("task-5")
}

// putThen is blobs whose Put does something more once it has kept the
// blob: what happens between the blob and Archive's commit.
type putThen struct {
	sharedfiles.Disk
	then func() error
}

func (b putThen) Put(r io.Reader) (string, int64, error) {
	hash, size, err := b.Disk.Put(r)
	if err == nil {
		err = b.then()
	}
	return hash, size, err
}

func summary() any { return map[string]any{"title": "Remove every TODO"} }

// archivable is a store filled as fill fills it.
func archivable(t *testing.T) *Store {
	t.Helper()
	s := openTestStore(t)
	fill(t, s)
	return s
}

// fill gives task-1 rows at 1, 2, 3 (a rejection) and 5, around task-2's
// at 4.
func fill(t *testing.T, s *Store) {
	t.Helper()
	acceptAll(t, s, newTask("task-1"), renamed("task-1"))
	if _, rejection, err := s.Propose(newTask("task-1")); rejection == nil || err != nil {
		t.Fatal("a second task.created should be refused:", rejection, err)
	}
	acceptAll(t, s, newTask("task-2"), renamed("task-1"))
}

// rowCount is how many rows a stream has in the database.
func rowCount(t *testing.T, s *Store, stream string) int {
	t.Helper()
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM events WHERE stream = ?`, stream).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

// Archive moves every row of the stream, rejections included, into the
// blob as it was, and leaves the marker alone in the stream, saying where
// they went. Other streams keep theirs.
func TestArchiveMovesTheRowsIntoTheBlob(t *testing.T) {
	s := archivable(t)
	log, err := s.Log("task-1")
	if err != nil {
		t.Fatal(err)
	}
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	marker, rejection, err := s.Archive("task-1", summary, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if marker.Type != events.TaskArchived || marker.Actor != archiver || marker.Number("through") != 5 || marker.Number("count") != 4 ||
		marker.Number("created") != 1 || marker.Data["summary"].(map[string]any)["title"] != "Remove every TODO" {
		t.Errorf("the marker: %+v", marker)
	}
	if left, err := s.Log("task-1"); err != nil || len(left) != 1 || left[0].Position != marker.Position {
		t.Errorf("task-1 should be its marker alone: %v, %v", left, err)
	}
	if rowCount(t, s, "task-2") != 1 || s.LastPosition() != marker.Position {
		t.Errorf("task-2 should keep its row and the store be at the marker, %d: at %d", marker.Position, s.LastPosition())
	}
	blob, err := blobs.Open(marker.Text("archive"))
	if err != nil {
		t.Fatal(err)
	}
	defer blob.Close()
	if archived, err := ReadArchive(blob); err != nil || !reflect.DeepEqual(archived, log) {
		t.Errorf("the blob holds %+v, %v\nwant the rows as Log read them: %+v", archived, err, log)
	}
}

// A proposal that lands on the stream between the blob and the commit
// refuses the marker, and nothing is written: the task stays whole for the
// next round, which archives it with the new row too.
func TestAnEventAfterTheBlobLeavesTheTaskForTheNextRound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, model := openAt(t, path)
	fill(t, s)
	disk := sharedfiles.Disk{Dir: t.TempDir()}
	late := putThen{disk, func() error {
		_, _, err := s.Propose(renamed("task-1"))
		return err
	}}
	_, rejection, err := s.Archive("task-1", summary, late)
	if err != nil || rejection == nil || rejection.Check != "archive" || !strings.Contains(rejection.Reason, "next round") {
		t.Fatalf("archiving as task-1 changed: %v, %v, want refused for the next round", rejection, err)
	}
	if rowCount(t, s, "task-1") != 5 || s.LastPosition() != 6 || model.applied != 6 {
		t.Errorf("task-1 should keep its 5 rows and nothing more be written or applied: %d rows, at %d, the model given %d events",
			rowCount(t, s, "task-1"), s.LastPosition(), model.applied)
	}
	marker, rejection, err := s.Archive("task-1", summary, disk)
	if rejection != nil || err != nil || marker.Number("through") != 6 || marker.Number("count") != 5 {
		t.Errorf("the next round: %+v, %v, %v", marker, rejection, err)
	}
}

// A crash after the blob leaves it named by nothing: the next round makes
// the same bytes, so the same blob, and archives the task with it.
func TestTheRoundAfterACrashMakesTheSameBlob(t *testing.T) {
	s := archivable(t)
	disk := sharedfiles.Disk{Dir: t.TempDir()}
	var first []string
	crash := putThen{disk, func() error {
		first, _ = filepath.Glob(filepath.Join(disk.Dir, "*", "*"))
		return errors.New("the power went")
	}}
	if _, rejection, err := s.Archive("task-1", summary, crash); err == nil || rejection != nil {
		t.Fatalf("archiving through the crash: %v, %v", rejection, err)
	}
	if rowCount(t, s, "task-1") != 4 || len(first) != 1 {
		t.Fatalf("the crash should leave task-1's rows and one blob: %d rows, blobs %v", rowCount(t, s, "task-1"), first)
	}
	written, _ := os.ReadFile(first[0])

	marker, rejection, err := s.Archive("task-1", summary, disk)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	again, _ := filepath.Glob(filepath.Join(disk.Dir, "*", "*"))
	if !slices.Equal(again, first) || filepath.Base(first[0]) != marker.Text("archive") {
		t.Errorf("blobs after the next round %v, the marker names %s; want the one from before the crash, %v", again, marker.Text("archive"), first)
	}
	if now, _ := os.ReadFile(again[0]); !bytes.Equal(now, written) {
		t.Error("the blob's bytes changed")
	}
}

// Unless the delete takes exactly the marker's count of rows, nothing is
// archived: no marker, every row still there, the model not told. A test
// trigger keeps one row back, as a row the archiver didn't read would.
func TestADeleteOfTheWrongCountUndoesTheArchive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, model := openAt(t, path)
	acceptAll(t, s, newTask("task-1"), renamed("task-1"), renamed("task-1"))
	if _, err := s.db.Exec(`CREATE TRIGGER keep_one BEFORE DELETE ON events WHEN OLD.position = 2 BEGIN SELECT RAISE(IGNORE); END`); err != nil {
		t.Fatal(err)
	}
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	if _, rejection, err := s.Archive("task-1", summary, blobs); err == nil || rejection != nil || !strings.Contains(err.Error(), "deleted 2 rows, not the marker's 3") {
		t.Fatalf("archiving with a row kept back: %v, %v", rejection, err)
	}
	if rowCount(t, s, "task-1") != 3 || s.LastPosition() != 3 || model.applied != 3 {
		t.Errorf("the archive should be undone: %d rows, the store at %d, the model given %d events; want 3 of each",
			rowCount(t, s, "task-1"), s.LastPosition(), model.applied)
	}

	if _, err := s.db.Exec(`DROP TRIGGER keep_one`); err != nil {
		t.Fatal(err)
	}
	if marker, rejection, err := s.Archive("task-1", summary, blobs); rejection != nil || err != nil || model.applied != 4 || rowCount(t, s, "task-1") != 1 {
		t.Errorf("archiving with the trigger gone: %+v, %v, %v", marker, rejection, err)
	}
}

// A blob gone between the put and the commit refuses the marker: it
// would name nothing.
func TestAMarkerNamesABlobThatIsThere(t *testing.T) {
	s := archivable(t)
	disk := sharedfiles.Disk{Dir: t.TempDir()}
	lost := putThen{disk, func() error {
		blobs, _ := filepath.Glob(filepath.Join(disk.Dir, "*", "*"))
		return os.Remove(blobs[0])
	}}
	if _, rejection, err := s.Archive("task-1", summary, lost); err != nil || rejection == nil || rejection.Check != "archive" {
		t.Errorf("archiving to a lost blob: %v, %v, want refused", rejection, err)
	}
	if rowCount(t, s, "task-1") != 4 {
		t.Error("task-1 should keep its rows")
	}
}

// Archive summarises under the store's lock, after the rows are read and
// put in their blob: the summary is of exactly the rows the marker covers.
func TestArchiveSummarisesUnderTheLockAfterTheRows(t *testing.T) {
	s := archivable(t)
	put := false
	blobs := putThen{sharedfiles.Disk{Dir: t.TempDir()}, func() error { put = true; return nil }}
	underTheLock := func() any {
		if s.mu.TryLock() {
			s.mu.Unlock()
			t.Error("summarise ran without the store's lock")
		}
		if !put {
			t.Error("summarise ran before the rows were read into their blob")
		}
		return summary()
	}
	if _, rejection, err := s.Archive("task-1", underTheLock, blobs); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
}

// Only the store writes the markers: Propose refuses them from anyone, the
// archiver's own System actor too, and the task's events stay as they were
// (the refusal is logged in its stream, as every refusal is).
func TestProposeRefusesTheMarkers(t *testing.T) {
	s := archivable(t)
	before, err := s.Stream("task-1")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []events.Proposal{marker("task-1", 1, 5, 4), unarchived("task-1", 6)} {
		stored, rejection, err := s.Propose(p)
		if err != nil || rejection == nil || rejection.Check != "who" || stored.Type != events.ProposalRejected {
			t.Errorf("proposing %s as %s: %v, %v, stored %s, want refused by the who check", p.Type, p.Actor, rejection, err, stored.Type)
		}
	}
	if after, err := s.Stream("task-1"); err != nil || !reflect.DeepEqual(after, before) {
		t.Errorf("task-1's events after the refusals: %+v, %v\nwant them as they were: %+v", after, err, before)
	}
}
