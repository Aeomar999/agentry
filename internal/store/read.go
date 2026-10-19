package store

import (
	"database/sql"
	"encoding/json"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

const columns = `position, at, stream, actor_kind, actor_id, type, correlation, data`

// Stream returns a stream's accepted events in order. Rejections are left out:
// they are in the log for people to read, but they never happened.
func (s *Store) Stream(name string) ([]events.Event, error) {
	return queryEvents(s.db, `SELECT `+columns+` FROM events
		WHERE stream = ? AND type != ? ORDER BY position`, name, events.ProposalRejected)
}

// After returns up to limit events of any stream after a position, rejections
// included. Readers use it to pull the log at their own pace (proof 05).
func (s *Store) After(position int64, limit int) ([]events.Event, error) {
	return queryEvents(s.db, `SELECT `+columns+` FROM events
		WHERE position > ? ORDER BY position LIMIT ?`, position, limit)
}

// TaskIDs returns every task's stream name, oldest first.
func (s *Store) TaskIDs() ([]string, error) { return s.streamsStartedBy(events.TaskCreated) }

// IdeaIDs returns every idea's stream name, oldest first.
func (s *Store) IdeaIDs() ([]string, error) { return s.streamsStartedBy(events.IdeaSuggested) }

func (s *Store) streamsStartedBy(kind events.Type) ([]string, error) {
	rows, err := s.db.Query(`SELECT stream FROM events WHERE type = ? ORDER BY position`, kind)
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

// txHistory reads inside a proposal's transaction, so the checks see exactly
// the state the event will be appended to.
type txHistory struct{ tx *sql.Tx }

func (h txHistory) Stream(name string) ([]events.Event, error) {
	return queryEvents(h.tx, `SELECT `+columns+` FROM events
		WHERE stream = ? AND type != ? ORDER BY position`, name, events.ProposalRejected)
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
		e.At, _ = time.Parse(time.RFC3339Nano, at)
		if err := json.Unmarshal([]byte(data), &e.Data); err != nil {
			return nil, err
		}
		found = append(found, e)
	}
	return found, rows.Err()
}

// Log returns everything written to a stream, rejections included: the
// whole record, for people to read.
func (s *Store) Log(name string) ([]events.Event, error) {
	return queryEvents(s.db, `SELECT `+columns+` FROM events WHERE stream = ? ORDER BY position`, name)
}

// LastPosition is the position of the newest event, 0 for an empty log.
func (s *Store) LastPosition() (int64, error) {
	var position int64
	err := s.db.QueryRow(`SELECT COALESCE(MAX(position), 0) FROM events`).Scan(&position)
	return position, err
}
