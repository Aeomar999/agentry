package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

const columns = `position, at, stream, actor_kind, actor_id, type, correlation, data`

// Every read of the log stops at the applied position, so no reader sees an
// event before the model has it (Store.applied). The reads go through the
// read-only pool: an event at or below applied is committed, so they see it.

// Stream returns a stream's accepted events in order. Rejections are left out:
// they are in the log for people to read, but they never happened.
func (s *Store) Stream(name string) ([]events.Event, error) {
	return queryEvents(s.reads, `SELECT `+columns+` FROM events
		WHERE stream = ? AND type != ? AND position <= ? ORDER BY position`, name, events.ProposalRejected, s.applied.Load())
}

// After returns up to limit events of any stream after a position, rejections
// included. Readers use it to pull the log at their own pace (proof 05).
func (s *Store) After(position int64, limit int) ([]events.Event, error) {
	return queryEvents(s.reads, `SELECT `+columns+` FROM events
		WHERE position > ? AND position <= ? ORDER BY position LIMIT ?`, position, s.applied.Load(), limit)
}

// IdeaIDs returns every idea's stream name, oldest first.
func (s *Store) IdeaIDs() ([]string, error) { return s.streamsStartedBy(events.IdeaSuggested) }

func (s *Store) streamsStartedBy(kind events.Type) ([]string, error) {
	rows, err := s.reads.Query(`SELECT stream FROM events WHERE type = ? AND position <= ? ORDER BY position`, kind, s.applied.Load())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

type querier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func queryEvents(db querier, query string, args ...any) ([]events.Event, error) {
	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var found []events.Event
	for rows.Next() {
		var e events.Event
		var at, data string
		if err := rows.Scan(&e.Position, &at, &e.Stream, &e.Actor.Kind, &e.Actor.ID, &e.Type, &e.Correlation, &data); err != nil {
			return nil, err
		}
		if err := decode(&e, at, data); err != nil {
			return nil, err
		}
		found = append(found, e)
	}
	return found, rows.Err()
}

// decode sets an event's time and data from the text stored for them.
func decode(e *events.Event, at, data string) error {
	e.At, _ = time.Parse(time.RFC3339Nano, at)
	var decoded map[string]any
	if err := json.Unmarshal([]byte(data), &decoded); err != nil {
		return err
	}
	e.Data = decoded
	return nil
}

// Log returns everything written to a stream, rejections included: the
// whole record, for people to read.
func (s *Store) Log(name string) ([]events.Event, error) {
	return queryEvents(s.reads, `SELECT `+columns+` FROM events
		WHERE stream = ? AND position <= ? ORDER BY position`, name, s.applied.Load())
}

// LastPosition is the position of the newest event in the model, 0 for an
// empty log.
func (s *Store) LastPosition() int64 { return s.applied.Load() }

// load gives the model each stream's accepted events, streams in the order of
// their first accepted event (tasks in creation order), and returns the log's
// last position. It runs in Open, before applied is set and before any reader
// exists, so it reads everything up to that last position, in one
// transaction for one snapshot.
func load(db *sql.DB, model Model) (int64, error) {
	tx, err := db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var last int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(position), 0) FROM events`).Scan(&last); err != nil {
		return 0, err
	}
	if model == nil {
		return last, nil
	}
	accepted, err := queryEvents(tx, `SELECT `+columns+` FROM events
		WHERE type != ? AND position <= ? ORDER BY position`, events.ProposalRejected, last)
	if err != nil {
		return 0, err
	}
	var streams []string
	byStream := map[string][]events.Event{}
	for _, e := range accepted {
		if _, seen := byStream[e.Stream]; !seen {
			streams = append(streams, e.Stream)
		}
		byStream[e.Stream] = append(byStream[e.Stream], e)
	}
	for _, stream := range streams {
		model.Load(stream, byStream[stream])
	}
	model.Loaded(last)
	return last, nil
}
