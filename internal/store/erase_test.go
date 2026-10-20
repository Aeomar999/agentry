package store

import (
	"database/sql"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
)

// The schema's tests write the erasure and run the deletes themselves, as
// archive_test.go does for the markers. Erase's own follow them; the ones
// that need the rules and the state, and the search of the files' bytes,
// are in storetest's erase_test.go.

// erasure is a task.erased covering stream's rows from created to through.
func erasure(stream string, created, through int64, count int) events.Proposal {
	return events.Proposal{Stream: stream, Actor: you, Type: events.TaskErased,
		Data: map[string]any{"why": "Asked to be forgotten.", "through": through, "count": count, "created": created, "archives": []string{}}}
}

// An erasure allows deleting exactly the rows it covers, in its own
// transaction before it commits: none past "through", none of another
// stream, and never the erasure itself. A stray delete is still refused
// after it.
func TestAnErasureAllowsDeletingOnlyTheRowsItCovers(t *testing.T) {
	s := openTestStore(t)
	acceptAll(t, s, newTask("task-1"), newTask("task-2"), renamed("task-1"), renamed("task-1"))
	// task-1 is at 1, 3 and 4, task-2 at 2; the erasure covers 1 and 3.
	inTransaction(t, s, func(tx *sql.Tx) {
		if _, err := deleteIn(tx, "task-1", 1, 3); !refusedAsAppendOnly(err) {
			t.Errorf("before the erasure: %v, want refused", err)
		}
		erased := appendIn(t, tx, erasure("task-1", 1, 3, 2))
		if _, err := tx.Exec(`UPDATE events SET data = '{}' WHERE position = 1`); !refusedAsAppendOnly(err) {
			t.Errorf("updating a row the erasure covers: %v, want refused", err)
		}
		if deleted, err := deleteIn(tx, "task-1", 1, 3); err != nil || deleted != 2 {
			t.Errorf("the rows the erasure covers: %d deleted, %v", deleted, err)
		}
		for _, stray := range []struct {
			name          string
			stream        string
			from, through int64
		}{
			{"the row just past through", "task-1", 4, 4},
			{"another stream's row before through", "task-2", 2, 2},
			{"the erasure", "task-1", erased.Position, erased.Position},
		} {
			if _, err := deleteIn(tx, stray.stream, stray.from, stray.through); !refusedAsAppendOnly(err) {
				t.Errorf("%s: %v, want refused", stray.name, err)
			}
		}
	})
	if _, err := s.db.Exec(`DELETE FROM events`); !refusedAsAppendOnly(err) {
		t.Errorf("a stray delete after the erasure: %v, want refused", err)
	}
	if rowCount(t, s, "task-1") != 2 || rowCount(t, s, "task-2") != 1 {
		t.Errorf("task-1 should keep its row past through and its erasure, task-2 its row: %d, %d", rowCount(t, s, "task-1"), rowCount(t, s, "task-2"))
	}
}

// A log at version 1, with archiving's trigger, migrates to 2: the
// trigger allows an erasure's rows and still an archive's, until the
// archive is undone.
func TestAVersionOneLogMigratesToErasing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	execOn(t, path, versionZero+migrations[0]+`; PRAGMA user_version = 1;`)
	s, _ := openAt(t, path)
	acceptAll(t, s, newTask("task-3"))
	// versionZero's task-1 is at 1 and 2, task-2 at 3; task-3 is at 4.
	inTransaction(t, s, func(tx *sql.Tx) {
		appendIn(t, tx, erasure("task-1", 1, 2, 2))
		appendIn(t, tx, marker("task-2", 3, 3, 1))
		for stream, want := range map[string]int64{"task-1": 2, "task-2": 1} {
			if deleted, err := deleteIn(tx, stream, 1, 3); err != nil || deleted != want {
				t.Errorf("%s's rows: %d deleted, %v, want %d", stream, deleted, err, want)
			}
		}
		if _, err := deleteIn(tx, "task-3", 4, 4); !refusedAsAppendOnly(err) {
			t.Errorf("a row no marker covers: %v, want refused", err)
		}
	})
	s.Close()
	if version, _, trigger := schemaOf(t, path); version != 2 || !strings.Contains(trigger, "task.archived") || !strings.Contains(trigger, "task.erased") {
		t.Errorf("version %d, trigger %s; want 2 and both cases", version, trigger)
	}
}

// Erase deletes every row of the stream, rejections included, and leaves
// its marker alone in the stream, saying who, why and which rows; other
// streams keep theirs. A task never archived has no blobs to delete.
func TestEraseLeavesTheMarkerAlone(t *testing.T) {
	s := archivable(t)
	erased, rejection, err := s.Erase("task-1", you, "Asked to be forgotten.", sharedfiles.Disk{Dir: t.TempDir()})
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if erased.Type != events.TaskErased || erased.Actor != you || erased.Text("why") != "Asked to be forgotten." || erased.Number("through") != 5 ||
		erased.Number("count") != 4 || erased.Number("created") != 1 || len(erased.List("archives")) != 0 {
		t.Errorf("the marker: %+v", erased)
	}
	if left, err := s.Log("task-1"); err != nil || len(left) != 1 || left[0].Position != erased.Position {
		t.Errorf("task-1 should be its marker alone: %v, %v", left, err)
	}
	if rowCount(t, s, "task-2") != 1 || s.LastPosition() != erased.Position {
		t.Errorf("task-2 should keep its row and the store be at the marker, %d: at %d", erased.Position, s.LastPosition())
	}
	var secureDelete int
	if err := s.db.QueryRow(`PRAGMA secure_delete`).Scan(&secureDelete); err != nil || secureDelete != 1 {
		t.Errorf("secure_delete after Erase: %d, %v, want on, as Open set it", secureDelete, err)
	}
}

// A delete of fewer rows than the marker counts undoes the erasure: no
// marker, every row still there, the model not told.
func TestADeleteOfTheWrongCountUndoesTheErasure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, model := openAt(t, path)
	acceptAll(t, s, newTask("task-1"), renamed("task-1"), renamed("task-1"))
	if _, err := s.db.Exec(`CREATE TRIGGER keep_one BEFORE DELETE ON events WHEN OLD.position = 2 BEGIN SELECT RAISE(IGNORE); END`); err != nil {
		t.Fatal(err)
	}
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	if _, rejection, err := s.Erase("task-1", you, "", blobs); err == nil || rejection != nil || !strings.Contains(err.Error(), "deleted 2 rows, not the marker's 3") {
		t.Fatalf("erasing with a row kept back: %v, %v", rejection, err)
	}
	if rowCount(t, s, "task-1") != 3 || s.LastPosition() != 3 || model.applied != 3 {
		t.Errorf("the erasure should be undone: %d rows, the store at %d, the model given %d events; want 3 of each",
			rowCount(t, s, "task-1"), s.LastPosition(), model.applied)
	}
}

// Erase deletes the blobs that held an archived task's rows, every one its
// markers named: archived, unarchived and archived again, it had two, and
// the first marker is in the second blob, not the live rows.
func TestEraseDeletesTheArchivesBlobs(t *testing.T) {
	s := archivable(t)
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	first, rejection, err := s.Archive("task-1", summary, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if _, rejection, err := s.Unarchive("task-1", blobs); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	acceptAll(t, s, renamed("task-1")) // so the second blob differs
	second, rejection, err := s.Archive("task-1", summary, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	erased, rejection, err := s.Erase("task-1", you, "", blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if want := []string{first.Text("archive"), second.Text("archive")}; !slices.Equal(erased.List("archives"), want) || erased.Number("created") != 1 {
		t.Errorf("the marker names %v, created %d; want %v and 1", erased.List("archives"), erased.Number("created"), want)
	}
	for _, hash := range erased.List("archives") {
		if blobs.Has(hash) {
			t.Errorf("the blob %s is still there", hash)
		}
	}
	if left, err := s.Log("task-1"); err != nil || len(left) != 1 {
		t.Errorf("task-1 should be its marker alone, its archive markers gone too: %v, %v", left, err)
	}
}

// An erased task counts once for the next id, whatever it was when it
// went: the next task never takes its id.
func TestCreateCountsErasedTasksOnce(t *testing.T) {
	s := openTestStore(t)
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	create := func(want string) {
		t.Helper()
		made, rejection, err := s.Create(events.TaskPrefix, newTask(""))
		if err != nil || rejection != nil || made.Stream != want {
			t.Fatalf("Create: %s %v %v, want %s", made.Stream, rejection, err, want)
		}
	}
	erase := func(stream string) {
		t.Helper()
		if _, rejection, err := s.Erase(stream, you, "", blobs); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	create("task-1")
	create("task-2")
	erase("task-2")
	create("task-3")
	if _, rejection, err := s.Archive("task-1", summary, blobs); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	erase("task-1") // its marker stands for the archived task.created
	create("task-4")
}
