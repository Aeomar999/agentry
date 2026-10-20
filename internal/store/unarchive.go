package store

import (
	"database/sql"
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Unarchive puts an archived task's rows back from the blob its marker
// names, and appends task.unarchived, which makes the task whole again
// (docs/design/log-growth.md section 4, "Reopening an archived task").
// First, with no lock held, it finds the stream's marker and reads the
// blob's rows. Then, under the lock, it checks the marker is still the
// stream's last accepted event and the rules allow the proposal, and in
// one transaction inserts the rows at their own positions and appends
// task.unarchived. The model refolds the stream from the rows below it.
// The blob stays: the marker still names it, and its rows are in it.
//
// It returns task.unarchived and, when it is refused, the rejection: the
// stream isn't archived, or took an event since the marker was read, or
// the rules refuse it. A refusal writes nothing, as Archive's does. The
// error is for a blob missing or unreadable, a count of rows that doesn't
// match the marker's, which undoes the transaction, or the database
// failing.
func (s *Store) Unarchive(stream string, blobs Opener) (events.Event, *events.Rejection, error) {
	live, err := s.Log(stream)
	if err != nil {
		return events.Event{}, nil, err
	}
	marker, archived := MarkerOf(live)
	if !archived {
		return events.Event{}, &events.Rejection{Check: "unarchive", Reason: stream + " isn't archived"}, nil
	}
	rows, err := keptBy(marker, blobs)
	if err != nil {
		return events.Event{}, nil, fmt.Errorf("unarchiving %s: %w", stream, err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if rejection, err := s.stillMarked(marker); rejection != nil || err != nil {
		return events.Event{}, rejection, err
	}
	unarchived := events.Proposal{Stream: stream, Actor: archiver, Type: events.TaskUnarchived, Data: map[string]any{
		"marker": marker.Position, "archive": marker.Text("archive")}}
	if rejection := s.validate(unarchived); rejection != nil {
		return events.Event{}, rejection, nil
	}
	event, before, err := s.commitUnarchive(marker, rows, unarchived)
	if err != nil {
		return events.Event{}, nil, err
	}
	if s.model != nil {
		give(s.model, s.catalogue, event, before)
	}
	s.applied.Store(event.Position)
	s.changes.notify()
	return event, nil, nil
}

// keptBy is the rows a marker's blob keeps for it. Each must be of the
// marker's stream: rows of another would go back into that stream. A row
// past the marker's "through" is not left out here: it makes the count
// wrong, which refuses the whole blob.
func keptBy(marker events.Event, blobs Opener) ([]row, error) {
	blob, err := blobs.Open(marker.Text("archive"))
	if err != nil {
		return nil, err
	}
	defer blob.Close()
	all, err := decodeArchive(blob)
	if err != nil {
		return nil, fmt.Errorf("reading the blob: %w", err)
	}
	for _, r := range all {
		if r.Stream != marker.Stream {
			return nil, fmt.Errorf("the blob holds a row of %s at %d", r.Stream, r.Position)
		}
	}
	return all, nil
}

// stillMarked checks, under the lock, that the marker is still its
// stream's last accepted event, since it was read without the lock. A
// rejection after it doesn't count: it changed nothing.
func (s *Store) stillMarked(marker events.Event) (*events.Rejection, error) {
	var last int64
	if err := s.db.QueryRow(`SELECT MAX(position) FROM events WHERE stream = ? AND type != ?`,
		marker.Stream, events.ProposalRejected).Scan(&last); err != nil {
		return nil, err
	}
	if last != marker.Position {
		return &events.Rejection{Check: "unarchive", Reason: marker.Stream + " took an event since its marker was read"}, nil
	}
	return nil, nil
}

// commitUnarchive inserts the marker's rows at their own positions, which
// AUTOINCREMENT never gave out again, and appends task.unarchived, in one
// transaction, and returns it with the stream's accepted events before it
// for the model's refold. Unless exactly the marker's "count" of rows go
// in, the transaction is undone, task.unarchived and all.
func (s *Store) commitUnarchive(marker events.Event, rows []row, unarchived events.Proposal) (events.Event, []events.Event, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return events.Event{}, nil, err
	}
	defer tx.Rollback()
	inserted, err := insertRows(tx, rows)
	if err != nil {
		return events.Event{}, nil, err
	}
	if count := marker.Number("count"); inserted != int64(count) {
		return events.Event{}, nil, fmt.Errorf("unarchiving %s inserted %d rows, not the marker's %d: nothing was unarchived", marker.Stream, inserted, count)
	}
	event, err := appendEvent(tx, unarchived)
	if err != nil {
		return events.Event{}, nil, err
	}
	before, err := beforeRefold(tx, s.catalogue, event)
	if err != nil {
		return events.Event{}, nil, err
	}
	return event, before, tx.Commit()
}

// insertRows inserts rows as they were stored, every column, and says how
// many went in.
func insertRows(tx *sql.Tx, rows []row) (int64, error) {
	var inserted int64
	for _, r := range rows {
		result, err := tx.Exec(`INSERT INTO events (`+columns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			r.Position, r.At, r.Stream, r.ActorKind, r.ActorID, r.Type, r.Correlation, r.Data)
		if err != nil {
			return 0, err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, err
		}
		inserted += n
	}
	return inserted, nil
}
