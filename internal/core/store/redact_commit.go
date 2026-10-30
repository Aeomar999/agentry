package store

import (
	"database/sql"
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// redactionOf is the event.redacted for the rows rewritten.
func redactionOf(stream string, who events.Actor, why string, rewrites []rewrittenRow) events.Proposal {
	positions, fields, removed := []int64{}, [][]string{}, 0
	for _, rw := range rewrites {
		positions = append(positions, rw.position)
		fields = append(fields, rw.fields)
		removed += rw.removed
	}
	return events.Proposal{Stream: stream, Actor: redactor, Type: events.EventRedacted, Data: map[string]any{
		"positions": positions, "fields": fields, "removed": removed, "who": who.String(), "why": why}}
}

// commitRedact appends the redaction and rewrites the rows it names, in
// one transaction: the update trigger allows it because it sees the
// redaction, uncommitted. It returns the redaction with the stream's
// accepted events before it, read after the rewrite, for the model's
// refold. Unless each update changes exactly its row, the transaction is
// undone, redaction and all.
func (s *Store) commitRedact(redaction events.Proposal, rewrites []rewrittenRow) (events.Event, []events.Event, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return events.Event{}, nil, err
	}
	defer tx.Rollback()
	event, err := appendEvent(tx, redaction)
	if err != nil {
		return events.Event{}, nil, err
	}
	for _, rw := range rewrites {
		if err := rewriteRow(tx, redaction.Stream, rw); err != nil {
			return events.Event{}, nil, err
		}
	}
	before, err := beforeRefold(tx, s.catalogue, event)
	if err != nil {
		return events.Event{}, nil, err
	}
	return event, before, tx.Commit()
}

// rewriteRow updates one row's data, and fails unless exactly that row
// changed.
func rewriteRow(tx *sql.Tx, stream string, rw rewrittenRow) error {
	result, err := tx.Exec(`UPDATE events SET data = ? WHERE stream = ? AND position = ?`, rw.data, stream, rw.position)
	if err != nil {
		return err
	}
	updated, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if updated != 1 {
		return fmt.Errorf("redacting %s updated %d rows at %d, not one: nothing was redacted", stream, updated, rw.position)
	}
	return nil
}
