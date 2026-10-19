package store

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Archiving moves a done task's rows out of the live log into a blob, and
// leaves a task.archived marker in their place that says where they went
// (docs/design/log-growth.md section 4). The blob is part of the log: a
// full read of the stream is its rows up to the marker's "through", then
// the live rows after.

// Blobs is where Archive keeps a task's rows: sharedfiles.Disk, whose Put
// has the blob on disk for good before it returns.
type Blobs interface {
	Has(hash string) bool
	Put(r io.Reader) (hash string, size int64, err error)
}

// archiver writes the markers.
var archiver = events.Actor{Kind: events.System, ID: "archiver"}

// Archive moves a task's rows into a blob and appends its marker, which
// keeps the task as the list shows it (tasks.SummaryOf). First, with no
// lock held, it reads the stream's rows, rejections included, and puts
// them in blobs. Then, under the lock, it checks the stream took nothing
// since, so the model holds exactly the rows read, and calls summarise
// for the summary. It checks the marker and, in one transaction, appends
// it and deletes the rows it covers; the model takes the marker once that
// commits. summarise reads the model, so it must not take the store.
// summarise runs before the rules, so it must cope with a task the state
// can't hand out (archived, or its stream broken): it returns nil, which
// the rules refuse along with the task.
//
// It returns the marker and, when it is refused, the rejection: by the
// rules, or because the stream took an event since its rows were read,
// which leaves the task as it is for the next round. A refusal writes
// nothing, unlike a proposal's: the archiver asks again every round, and a
// proposal.rejected each time would itself change the stream. The error
// is for the database or the blobs failing. A blob put before either is
// named by no event, and the next round makes the same bytes, so finds it.
func (s *Store) Archive(stream string, summarise func() any, blobs Blobs) (events.Event, *events.Rejection, error) {
	rows, err := rowsOf(s.reads, stream, s.applied.Load())
	if err != nil {
		return events.Event{}, nil, err
	}
	if len(rows) == 0 {
		return events.Event{}, nil, fmt.Errorf("archiving %s: the stream has no rows", stream)
	}
	archive, err := encodeArchive(rows)
	if err != nil {
		return events.Event{}, nil, err
	}
	hash, _, err := blobs.Put(bytes.NewReader(archive))
	if err != nil {
		return events.Event{}, nil, fmt.Errorf("archiving %s: %w", stream, err)
	}
	through := rows[len(rows)-1].Position

	s.mu.Lock()
	defer s.mu.Unlock()
	if rejection, err := s.unmoved(stream, through); rejection != nil || err != nil {
		return events.Event{}, rejection, err
	}
	marker := events.Proposal{Stream: stream, Actor: archiver, Type: events.TaskArchived, Data: map[string]any{
		"through": through, "count": len(rows), "archive": hash, "created": createdIn(rows), "summary": summarise()}}
	rejection, err := s.archivable(marker, blobs)
	if rejection != nil || err != nil {
		return events.Event{}, rejection, err
	}
	event, err := s.commitArchive(marker)
	if err != nil {
		return events.Event{}, nil, err
	}
	if s.model != nil {
		give(s.model, s.catalogue, event, nil)
	}
	s.applied.Store(event.Position)
	s.changes.notify()
	return event, nil, nil
}

// unmoved checks, under the lock, that the stream's last row is still
// through, since its rows were read without the lock.
func (s *Store) unmoved(stream string, through int64) (*events.Rejection, error) {
	var last int64
	if err := s.db.QueryRow(`SELECT MAX(position) FROM events WHERE stream = ?`, stream).Scan(&last); err != nil {
		return nil, err
	}
	if last != through {
		return &events.Rejection{Check: "archive", Reason: stream + " took an event since its rows were read: it waits for the next round"}, nil
	}
	return nil, nil
}

// archivable checks a marker under the lock: the rules, and the blob it
// names on disk.
func (s *Store) archivable(marker events.Proposal, blobs Blobs) (*events.Rejection, error) {
	if rejection := s.validate(marker); rejection != nil {
		return rejection, nil
	}
	if !blobs.Has(marker.Text("archive")) {
		return &events.Rejection{Check: "archive", Reason: "the blob of " + marker.Stream + "'s rows isn't there"}, nil
	}
	return nil, nil
}

// commitArchive appends the marker and deletes the rows it covers, in one
// transaction: the delete trigger allows it because it sees the marker,
// uncommitted. Unless exactly "count" rows go, the transaction is undone,
// marker and all.
func (s *Store) commitArchive(marker events.Proposal) (events.Event, error) {
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
	deleted, err := result.RowsAffected() // SQLite's changes()
	if err != nil {
		return events.Event{}, err
	}
	if count := marker.Number("count"); deleted != int64(count) {
		return events.Event{}, fmt.Errorf("archiving %s deleted %d rows, not the marker's %d: nothing was archived", marker.Stream, deleted, count)
	}
	return event, tx.Commit()
}

// A row is an event as stored, every column as text: an archive keeps the
// rows as they were, so the ones it gives back are the same rows. Rows are
// written with encoding/json, which would turn invalid UTF-8 in a column
// into U+FFFD; none holds any, since every value arrives through JSON.
type row struct {
	Position    int64  `json:"position"`
	At          string `json:"at"`
	Stream      string `json:"stream"`
	ActorKind   string `json:"actorKind"`
	ActorID     string `json:"actorID"`
	Type        string `json:"type"`
	Correlation string `json:"correlation"`
	Data        string `json:"data"`
}

// rowsOf is a stream's rows up to through, rejections included.
func rowsOf(db querier, stream string, through int64) ([]row, error) {
	found, err := db.Query(`SELECT `+columns+` FROM events WHERE stream = ? AND position <= ? ORDER BY position`, stream, through)
	if err != nil {
		return nil, err
	}
	defer found.Close()
	var rows []row
	for found.Next() {
		var r row
		if err := found.Scan(&r.Position, &r.At, &r.Stream, &r.ActorKind, &r.ActorID, &r.Type, &r.Correlation, &r.Data); err != nil {
			return nil, err
		}
		rows = append(rows, r)
	}
	return rows, found.Err()
}

// createdIn is the position of the task.created among a task's rows.
func createdIn(rows []row) int64 {
	for _, r := range rows {
		if r.Type == string(events.TaskCreated) {
			return r.Position
		}
	}
	return 0 // none: the rules refuse the marker
}

// encodeArchive is the rows as JSON lines, gzipped. Go's gzip writes no
// time into its header, so the same rows always make the same bytes.
func encodeArchive(rows []row) ([]byte, error) {
	var archive bytes.Buffer
	zipped := gzip.NewWriter(&archive)
	lines := json.NewEncoder(zipped)
	for _, r := range rows {
		if err := lines.Encode(r); err != nil {
			return nil, err
		}
	}
	if err := zipped.Close(); err != nil {
		return nil, err
	}
	return archive.Bytes(), nil
}

// ReadArchive is the events in an archive blob, rejections included, in
// order, as Log reads them: what an archived task's history starts with.
func ReadArchive(blob io.Reader) ([]events.Event, error) {
	rows, err := decodeArchive(blob)
	if err != nil {
		return nil, err
	}
	found := make([]events.Event, 0, len(rows))
	for _, r := range rows {
		e := events.Event{Position: r.Position, Proposal: events.Proposal{Stream: r.Stream, Type: events.Type(r.Type), Correlation: r.Correlation,
			Actor: events.Actor{Kind: events.ActorKind(r.ActorKind), ID: r.ActorID}}}
		if err := decode(&e, r.At, r.Data); err != nil {
			return nil, err
		}
		found = append(found, e)
	}
	return found, nil
}

// decodeArchive is the rows in an archive blob, as encodeArchive wrote
// them.
func decodeArchive(blob io.Reader) ([]row, error) {
	unzipped, err := gzip.NewReader(blob)
	if err != nil {
		return nil, err
	}
	lines := json.NewDecoder(unzipped)
	var rows []row
	for {
		var r row
		switch err := lines.Decode(&r); {
		case errors.Is(err, io.EOF):
			return rows, unzipped.Close()
		case err != nil:
			return nil, err
		}
		rows = append(rows, r)
	}
}
