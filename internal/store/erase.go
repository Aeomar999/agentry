package store

import (
	"errors"
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Erasing deletes a task's rows for good, and leaves a task.erased marker
// in their place that says who erased it, when and why, never what was in
// it (docs/design/log-growth.md section 5, "Removing a whole task"). Unlike
// an archive, nothing undoes it: the delete trigger lets the rows the
// marker covers go, and the rules refuse every event after it.
//
// What the store can't do is left to its caller: stopping a turn that
// runs, since only the runner can see one; the shared files' blobs, which
// another task's event may name; the copies of the task's words in other
// streams; and writing a new snapshot.

// Remover opens and deletes blobs: sharedfiles.Disk.
type Remover interface {
	Opener
	Remove(hash string) error
}

// Erase deletes a task's rows for who, appends its marker, and deletes the
// blobs that held its rows while it was archived. First, with no lock held,
// it reads the stream's rows, rejections included, for the marker: the last
// row's position and the task's task.created, and the blobs its
// task.archived markers name, read from its History, as a task archived
// again keeps its earlier markers in its latest blob. Then, under the lock,
// it checks the stream took nothing since and the rules allow the marker,
// and in one transaction appends it and deletes the rows it covers, their
// bytes zeroed by the connection's secure_delete (Open); the model takes the
// marker once that commits. Last, still under the lock, it gives the free
// pages back and empties the WAL (giveBackSpace), which still held the rows,
// and deletes the blobs.
//
// A blob that can't be read, lost or corrupt, doesn't stop it, or a lost
// blob would leave the task with no way to be erased: the live marker still
// names that blob, which is deleted with the rest, but the blobs its rows
// name, from earlier archives, can't be found, and the error after the
// commit names it.
//
// It returns the marker and, when it is refused, the rejection: by the
// rules, or because the stream took an event since its rows were read. A
// refusal writes nothing, as Archive's does. The error is for the database
// or the blobs failing. One after the commit comes with the marker: the
// task is erased, but some of its text may stay in the WAL, the free pages
// or a blob until GiveBackSpace runs again or the blob is removed, or in an
// earlier blob that an unreadable one named.
func (s *Store) Erase(stream string, who events.Actor, why string, blobs Remover) (events.Event, *events.Rejection, error) {
	live, err := s.Log(stream)
	if err != nil {
		return events.Event{}, nil, err
	}
	var unread error
	history, err := s.History(stream, func(marker events.Event) ([]events.Event, error) {
		kept, err := FromBlobs(blobs)(marker)
		if err != nil { // the live marker still names it, and the erasure goes on
			unread = fmt.Errorf("the blob %s can't be read, so the blobs its rows name can't be found: %w", marker.Text("archive"), err)
		}
		return kept, nil
	})
	if err != nil {
		return events.Event{}, nil, fmt.Errorf("erasing %s: %w", stream, err)
	}
	marker := erasureOf(stream, who, why, live, history)

	s.mu.Lock()
	defer s.mu.Unlock()
	if rejection, err := s.unmoved(stream, int64(marker.Number("through")), "erase", "erase it again"); rejection != nil || err != nil {
		return events.Event{}, rejection, err
	}
	if rejection := s.validate(marker); rejection != nil {
		return events.Event{}, rejection, nil
	}
	event, err := s.commitErase(marker)
	if err != nil {
		return events.Event{}, nil, err
	}
	if s.model != nil {
		give(s.model, s.catalogue, event, nil)
	}
	s.applied.Store(event.Position)
	s.changes.notify()
	return event, nil, s.forget(marker, blobs, unread)
}

// erasureOf is the marker that erases a stream's live rows, and the blobs
// the archive markers in its history name. For a stream with no rows it
// names none, which the rules refuse along with the task.
func erasureOf(stream string, who events.Actor, why string, live, history []events.Event) events.Proposal {
	var through int64
	if len(live) > 0 {
		through = live[len(live)-1].Position
	}
	archives := []string{}
	for _, e := range history {
		if e.Type == events.TaskArchived {
			archives = append(archives, e.Text("archive"))
		}
	}
	return events.Proposal{Stream: stream, Actor: who, Type: events.TaskErased, Data: map[string]any{
		"why": why, "through": through, "count": len(live), "created": taskCreatedIn(live), "archives": archives}}
}

// taskCreatedIn is the position of the task.created among a task's live
// rows, or, when archiving took it away, the "created" of the marker that
// stands for it.
func taskCreatedIn(live []events.Event) int64 {
	for _, e := range live {
		switch e.Type {
		case events.TaskCreated:
			return e.Position
		case events.TaskArchived:
			return int64(e.Number("created"))
		}
	}
	return 0 // none: the rules refuse the marker
}

// commitErase appends the marker and deletes the rows it covers, in one
// transaction, as commitArchive does: the delete trigger allows it because
// it sees the marker, uncommitted. Unless exactly "count" rows go, the
// transaction is undone, marker and all.
func (s *Store) commitErase(marker events.Proposal) (events.Event, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return events.Event{}, err
	}
	defer tx.Rollback()
	event, err := appendEvent(tx, marker)
	if err != nil {
		return events.Event{}, err
	}
	result, err := tx.Exec(`DELETE FROM events WHERE stream = ? AND position <= ?`, marker.Stream, marker.Number("through"))
	if err != nil {
		return events.Event{}, err
	}
	deleted, err := result.RowsAffected()
	if err != nil {
		return events.Event{}, err
	}
	if count := marker.Number("count"); deleted != int64(count) {
		return events.Event{}, fmt.Errorf("erasing %s deleted %d rows, not the marker's %d: nothing was erased", marker.Stream, deleted, count)
	}
	return event, tx.Commit()
}

// forget is the rest of an erasure, once the marker has committed: the
// free pages given back, the WAL emptied and the blobs the marker names
// deleted, each tried whatever the others do. unread is why a blob
// couldn't be read for the marker, if one couldn't.
func (s *Store) forget(marker events.Proposal, blobs Remover, unread error) error {
	failed := errors.Join(unread, s.giveBackSpace())
	for _, hash := range marker.List("archives") {
		if err := blobs.Remove(hash); err != nil {
			failed = errors.Join(failed, err)
		}
	}
	if failed != nil {
		return fmt.Errorf("%s is erased, but not all of it is gone yet: %w", marker.Stream, failed)
	}
	return nil
}
