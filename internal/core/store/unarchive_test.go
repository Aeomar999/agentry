package store

import (
	"bytes"
	"context"
	"io"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
)

// The state's side of unarchiving, that the task comes back as a log never
// archived has it, is in storetest's unarchive_test.go, with the rules.

// openedAs is an Opener that opens every blob as open says.
type openedAs func(hash string) (io.ReadCloser, error)

func (o openedAs) Open(hash string) (io.ReadCloser, error) { return o(hash) }

// unarchive unarchives stream, and fails the test unless it is accepted.
func unarchive(t *testing.T, s *Store, stream string, blobs Opener) events.Event {
	t.Helper()
	event, rejection, err := s.Unarchive(stream, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	return event
}

// hashes are the fingerprints of the events' rows, as stored now.
func hashes(t *testing.T, s *Store, found []events.Event) []string {
	t.Helper()
	var all []string
	for _, e := range found {
		hash, ok, err := s.Hash(e.Position)
		if err != nil || !ok {
			t.Fatalf("the row at %d: %v, %v", e.Position, ok, err)
		}
		all = append(all, hash)
	}
	return all
}

// Unarchive puts back every row the marker moved, rejections included,
// at its own position and with every column as it was, then appends
// task.unarchived; the model refolds the stream from the accepted rows
// below it, the marker among them.
func TestUnarchivePutsTheRowsBackAsTheyWere(t *testing.T) {
	model, made := &folds{}, newCreated()
	check := func(p events.Proposal) *events.Rejection {
		rejection := onlyOnce(made)(p)
		if rejection == nil {
			made.Apply(events.Event{Proposal: p})
		}
		return rejection
	}
	s, err := Open(filepath.Join(t.TempDir(), "agentry.db"), withNotes, model, check)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	fill(t, s) // task-1 at 1, 2, 3 (a rejection) and 5
	before, err := s.Log("task-1")
	if err != nil {
		t.Fatal(err)
	}
	stored := hashes(t, s, before)
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	marker, rejection, err := s.Archive("task-1", summary, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}

	event := unarchive(t, s, "task-1", blobs)
	if event.Type != events.TaskUnarchived || event.Actor != archiver || int64(event.Number("marker")) != marker.Position || event.Text("archive") != marker.Text("archive") {
		t.Errorf("task.unarchived: %+v", event)
	}
	log, err := s.Log("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if want := append(before, marker, event); !reflect.DeepEqual(log, want) {
		t.Errorf("task-1's rows: %v, want %v", positions(log), positions(want))
	}
	if now := hashes(t, s, before); !reflect.DeepEqual(now, stored) {
		t.Error("a restored row differs from the row archived")
	}
	if last := model.given[len(model.given)-1]; last != "Refold task-1 [1 2 5 6] 7" || s.LastPosition() != 7 {
		t.Errorf("the model was last given %q, the store is at %d; want the refold of 7 from 1, 2, 5 and 6", last, s.LastPosition())
	}
}

// unchanged fails the test if the store wrote or applied anything since
// last, applied and rows.
func unchanged(t *testing.T, s *Store, model *created, last int64, applied, rows int) {
	t.Helper()
	if s.LastPosition() != last || model.applied != applied || rowCount(t, s, "task-1") != rows {
		t.Errorf("the store at %d, the model given %d events, task-1 with %d rows; want %d, %d and %d, nothing written",
			s.LastPosition(), model.applied, rowCount(t, s, "task-1"), last, applied, rows)
	}
}

// archivedWith is archivedFill's store, opened with its model.
func archivedWith(t *testing.T) (*Store, *created, sharedfiles.Disk, events.Event) {
	t.Helper()
	s, model := openAt(t, filepath.Join(t.TempDir(), "agentry.db"))
	fill(t, s)
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	marker, rejection, err := s.Archive("task-1", summary, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	return s, model, blobs, marker
}

// A stream not archived, or archived and unarchived since, is refused,
// and nothing is written: no task.unarchived and no rejection.
func TestUnarchivingAStreamNotArchivedIsRefused(t *testing.T) {
	s, model, blobs, _ := archivedWith(t)
	unarchive(t, s, "task-1", blobs)
	last, applied := s.LastPosition(), model.applied
	for _, stream := range []string{"task-1", "task-2", "task-99"} {
		if _, rejection, err := s.Unarchive(stream, blobs); err != nil || rejection == nil || rejection.Reason != stream+" isn't archived" {
			t.Errorf("unarchiving %s: %v, %v; want refused as not archived", stream, rejection, err)
		}
	}
	unchanged(t, s, model, last, applied, 6)
}

// A blob that is missing, isn't an archive, or holds another stream's row
// is an error, and nothing is written.
func TestUnarchivingFromABadBlobWritesNothing(t *testing.T) {
	s, model, disk, marker := archivedWith(t)
	last, applied := s.LastPosition(), model.applied
	// The blob's own rows, every count right, but one moved to task-2; and
	// the same rows with one more, past the marker's "through".
	blob, err := disk.Open(marker.Text("archive"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := decodeArchive(blob)
	blob.Close()
	if err != nil {
		t.Fatal(err)
	}
	past := append(slices.Clone(rows), rows[len(rows)-1])
	past[len(past)-1].Position = marker.Position + 100
	pastThrough, err := encodeArchive(past)
	if err != nil {
		t.Fatal(err)
	}
	rows[1].Stream = "task-2"
	another, err := encodeArchive(rows)
	if err != nil {
		t.Fatal(err)
	}
	for name, blobs := range map[string]Opener{
		"missing":        sharedfiles.Disk{Dir: t.TempDir()},
		"not an archive": openedAs(func(string) (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("not gzip")), nil }),
		"another's row":  openedAs(func(string) (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(another)), nil }),
		"row past through": openedAs(func(string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(pastThrough)), nil
		}),
		"torn part of the way": openedAs(func(string) (io.ReadCloser, error) {
			return io.NopCloser(bytes.NewReader(another[:len(another)/2])), nil
		}),
	} {
		if _, rejection, err := s.Unarchive("task-1", blobs); err == nil || rejection != nil {
			t.Errorf("%s: %v, %v; want an error", name, rejection, err)
		}
	}
	unchanged(t, s, model, last, applied, 1)
	if rowCount(t, s, "task-2") != 1 {
		t.Errorf("task-2 has %d rows, want its one", rowCount(t, s, "task-2"))
	}
	if left, err := s.Log("task-1"); err != nil || len(left) != 1 || left[0].Position != marker.Position {
		t.Errorf("task-1 should be its marker alone: %v, %v", left, err)
	}
}

// Unless exactly the marker's count of rows go back in, nothing is
// unarchived: no row back, no task.unarchived, the model not told. A test
// trigger keeps one row out, as a blob short of a row would.
func TestAnInsertOfTheWrongCountUndoesTheUnarchive(t *testing.T) {
	s, model, blobs, _ := archivedWith(t)
	last, applied := s.LastPosition(), model.applied
	if _, err := s.db.Exec(`CREATE TRIGGER keep_out BEFORE INSERT ON events WHEN NEW.position = 2 BEGIN SELECT RAISE(IGNORE); END`); err != nil {
		t.Fatal(err)
	}
	if _, rejection, err := s.Unarchive("task-1", blobs); err == nil || rejection != nil || !strings.Contains(err.Error(), "inserted 3 rows, not the marker's 4") {
		t.Fatalf("unarchiving with a row kept out: %v, %v", rejection, err)
	}
	unchanged(t, s, model, last, applied, 1)

	if _, err := s.db.Exec(`DROP TRIGGER keep_out`); err != nil {
		t.Fatal(err)
	}
	unarchive(t, s, "task-1", blobs)
	if rowCount(t, s, "task-1") != 6 {
		t.Errorf("task-1 has %d rows, want its 4, the marker and task.unarchived", rowCount(t, s, "task-1"))
	}
}

// An event accepted on the stream between reading the marker and the lock
// refuses the unarchiving, as the marker may no longer be the one to undo.
// A rejection after the marker doesn't: it changed nothing.
func TestOnlyAnAcceptedEventAfterTheMarkerRefusesTheUnarchive(t *testing.T) {
	s, model, blobs, _ := archivedWith(t)
	if _, rejection, err := s.Propose(newTask("task-1")); rejection == nil || err != nil {
		t.Fatal("a second task.created should be refused:", rejection, err)
	}
	last, applied := s.LastPosition(), model.applied
	late := openedAs(func(hash string) (io.ReadCloser, error) {
		if _, _, err := s.Propose(renamed("task-1")); err != nil {
			return nil, err
		}
		return blobs.Open(hash)
	})
	_, rejection, err := s.Unarchive("task-1", late)
	if err != nil || rejection == nil || !strings.Contains(rejection.Reason, "took an event since its marker was read") {
		t.Fatalf("unarchiving as task-1 changed: %v, %v; want refused", rejection, err)
	}
	unchanged(t, s, model, last+1, applied+1, 3) // the marker, the rejection and the rename

	s, model, blobs, _ = archivedWith(t)
	if _, rejection, err := s.Propose(newTask("task-1")); rejection == nil || err != nil {
		t.Fatal("a second task.created should be refused:", rejection, err)
	}
	unarchive(t, s, "task-1", blobs)
	if rowCount(t, s, "task-1") != 7 {
		t.Errorf("task-1 has %d rows, want its 4, the marker, the rejection and task.unarchived", rowCount(t, s, "task-1"))
	}
}

// Readers that follow the log by position (After, and the Reader that
// /api/changes waits on) passed the restored rows' positions long ago:
// they see only task.unarchived, which is new.
func TestReadersByPositionSeeOnlyTaskUnarchived(t *testing.T) {
	s, _, blobs, _ := archivedWith(t)
	position := s.LastPosition()
	reader := s.NewReader(position)
	defer reader.Close()
	event := unarchive(t, s, "task-1", blobs)

	if found, err := s.After(position, 100); err != nil || !reflect.DeepEqual(found, []events.Event{event}) {
		t.Errorf("After(%d): %v, %v; want task.unarchived alone, at %d", position, positions(found), err, event.Position)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if batch, err := reader.Next(ctx); err != nil || !reflect.DeepEqual(batch, []events.Event{event}) {
		t.Errorf("the reader: %v, %v; want task.unarchived alone", positions(batch), err)
	}
}

// Once unarchived, the rows are the task's again: the delete trigger
// refuses them to anyone, and an Archive that read the stream before the
// Unarchive is refused under the lock, so it deletes nothing. Archiving it
// again archives every row anew, the first marker and task.unarchived
// among them.
func TestTheArchiverCantDeleteRestoredRows(t *testing.T) {
	s, _, blobs, marker := archivedWith(t)
	var event events.Event
	between := putThen{blobs, func() error {
		event = unarchive(t, s, "task-1", blobs)
		return nil
	}}
	_, rejection, err := s.Archive("task-1", summary, between)
	if err != nil || rejection == nil || !strings.Contains(rejection.Reason, "took an event since its rows were read") {
		t.Fatalf("archiving with an unarchive between the read and the lock: %v, %v; want refused", rejection, err)
	}
	if rowCount(t, s, "task-1") != 6 || s.LastPosition() != event.Position {
		t.Errorf("task-1 should keep its 6 rows and nothing more be written: %d rows, at %d", rowCount(t, s, "task-1"), s.LastPosition())
	}
	for _, position := range []int64{1, 2, 3, 5} {
		if _, err := s.db.Exec(`DELETE FROM events WHERE position = ?`, position); !refusedAsAppendOnly(err) {
			t.Errorf("deleting the restored row at %d: %v, want refused", position, err)
		}
	}

	log, err := s.Log("task-1")
	if err != nil {
		t.Fatal(err)
	}
	second, rejection, err := s.Archive("task-1", summary, blobs)
	if rejection != nil || err != nil || second.Number("count") != 6 || int64(second.Number("through")) != event.Position {
		t.Fatalf("archiving again: %+v, %v, %v; want all 6 rows through %d", second, rejection, err, event.Position)
	}
	history, err := s.History("task-1", FromBlobs(blobs))
	if err != nil || !reflect.DeepEqual(history, append(log, second)) {
		t.Errorf("task-1's history after archiving again: %v, %v; want %v", positions(history), err, positions(append(log, second)))
	}
	if !blobs.Has(marker.Text("archive")) || second.Text("archive") == marker.Text("archive") {
		t.Error("the first blob should stay, and the second be another")
	}
}
