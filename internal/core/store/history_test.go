package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
)

// archivedFill is a store filled as fill fills it with task-1 archived,
// the blobs its rows went to, its marker, and its rows as Log read them
// before.
func archivedFill(t *testing.T) (*Store, sharedfiles.Disk, events.Event, []events.Event) {
	t.Helper()
	s := archivable(t)
	before, err := s.Log("task-1")
	if err != nil {
		t.Fatal(err)
	}
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	marker, rejection, err := s.Archive("task-1", summary, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	return s, blobs, marker, before
}

// positions are the events' positions, in their order.
func positions(found []events.Event) []int64 {
	var at []int64
	for _, e := range found {
		at = append(at, e.Position)
	}
	return at
}

// An archived task's history is its rows as they were, rejections
// included, then its marker; a stream not archived is its live rows.
func TestHistoryIsTheArchivedRowsThenTheLiveOnes(t *testing.T) {
	s, blobs, marker, before := archivedFill(t)
	history, err := s.History("task-1", FromBlobs(blobs))
	if err != nil {
		t.Fatal(err)
	}
	if want := append(before, marker); !reflect.DeepEqual(history, want) {
		t.Errorf("task-1's history: %v\nwant %v", positions(history), positions(want))
	}
	live, err := s.Log("task-2")
	if err != nil {
		t.Fatal(err)
	}
	if history, err := s.History("task-2", nil); err != nil || !reflect.DeepEqual(history, live) {
		t.Errorf("task-2's history: %v, %v; want its live rows %v", positions(history), err, positions(live))
	}
}

// Rows a crash left live, the blob's too, come in position order, each
// once, and so does a rejection after the marker.
func TestHistoryHasEachRowOnceInPositionOrder(t *testing.T) {
	s, blobs, marker, before := archivedFill(t)
	for _, left := range []events.Event{before[1], before[3]} { // at 2 and 5
		data, _ := json.Marshal(left.Data)
		if _, err := s.db.Exec(`INSERT INTO events (`+columns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, left.Position, left.At.Format(time.RFC3339Nano),
			left.Stream, left.Actor.Kind, left.Actor.ID, left.Type, left.Correlation, string(data)); err != nil {
			t.Fatal(err)
		}
	}
	if _, rejection, err := s.Propose(newTask("task-1")); rejection == nil || err != nil {
		t.Fatal("a second task.created should be refused:", rejection, err)
	}
	history, err := s.History("task-1", FromBlobs(blobs))
	if err != nil {
		t.Fatal(err)
	}
	if got, want := positions(history), []int64{1, 2, 3, 5, marker.Position, marker.Position + 1}; !reflect.DeepEqual(got, want) {
		t.Errorf("task-1's history is at %v, want %v", got, want)
	}
	if last := history[len(history)-1]; last.Type != events.ProposalRejected {
		t.Errorf("the last of task-1's history is %s, want the second task.created's rejection", last.Type)
	}
}

// A blob missing or not an archive is an error that names the stream.
func TestHistoryOfAMissingOrCorruptBlobIsAnError(t *testing.T) {
	s, blobs, marker, _ := archivedFill(t)
	hash := marker.Text("archive")
	path := filepath.Join(blobs.Dir, hash[:2], hash)
	if err := os.WriteFile(path, []byte("not gzip"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.History("task-1", FromBlobs(blobs)); err == nil || !strings.Contains(err.Error(), "task-1") {
		t.Errorf("a corrupt blob: %v, want an error naming task-1", err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, err := s.History("task-1", FromBlobs(blobs)); !errors.Is(err, os.ErrNotExist) || !strings.Contains(err.Error(), "task-1") {
		t.Errorf("a missing blob: %v, want not found, naming task-1", err)
	}
}

// A stream unarchived since its marker has its rows back, so its history
// is its live rows and no blob is read.
func TestHistoryOfAnUnarchivedStreamReadsNoBlob(t *testing.T) {
	s := openTestStore(t)
	acceptAll(t, s, newTask("task-1"), renamed("task-1"))
	inTransaction(t, s, func(tx *sql.Tx) {
		first := appendIn(t, tx, marker("task-1", 1, 2, 2))
		appendIn(t, tx, unarchived("task-1", first.Position))
	})
	read := func(events.Event) ([]events.Event, error) {
		t.Error("a blob was read")
		return nil, nil
	}
	live, err := s.Log("task-1")
	if err != nil {
		t.Fatal(err)
	}
	if history, err := s.History("task-1", read); err != nil || !reflect.DeepEqual(history, live) {
		t.Errorf("task-1's history: %v, %v; want its live rows %v", positions(history), err, positions(live))
	}
}
