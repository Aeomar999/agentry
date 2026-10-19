package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
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

// LogAfter returns a page of a stream's whole record, as Log has it: up to
// limit events of the given types (every type when none is given) after one
// position and before another, both left out, in order.
func (s *Store) LogAfter(name string, types []events.Type, after, before int64, limit int) ([]events.Event, error) {
	only, args := ofTypes(name, types, after, before, s.applied.Load())
	return queryEvents(s.reads, `SELECT `+columns+` FROM events
		WHERE stream = ? AND position > ? AND position < ? AND position <= ?`+only+`
		ORDER BY position LIMIT ?`, append(args, limit)...)
}

// LogBefore is LogAfter's page from the other end: the last limit events
// between the two positions, still in order.
func (s *Store) LogBefore(name string, types []events.Type, after, before int64, limit int) ([]events.Event, error) {
	only, args := ofTypes(name, types, after, before, s.applied.Load())
	return queryEvents(s.reads, `SELECT * FROM (SELECT `+columns+` FROM events
		WHERE stream = ? AND position > ? AND position < ? AND position <= ?`+only+`
		ORDER BY position DESC LIMIT ?) ORDER BY position`, append(args, limit)...)
}

// ofTypes is the condition that keeps a page to some types, nothing when
// it keeps every type, and the page's arguments with the types after them.
func ofTypes(name string, types []events.Type, after, before, applied int64) (string, []any) {
	args := []any{name, after, before, applied}
	if len(types) == 0 {
		return "", args
	}
	for _, t := range types {
		args = append(args, t)
	}
	return ` AND type IN (?` + strings.Repeat(`, ?`, len(types)-1) + `)`, args
}

// Hash is a fingerprint of the event at position as stored, every column
// of its row: a snapshot names the row it was taken at by it, so a log that
// differs there, such as an older backup restored and grown since, is told
// apart. Found is false when no event has that position.
func (s *Store) Hash(position int64) (hash string, found bool, err error) {
	if position > s.applied.Load() {
		return "", false, nil
	}
	return hashOf(s.reads, position)
}

func hashOf(db querier, position int64) (string, bool, error) {
	rows, err := db.Query(`SELECT `+columns+` FROM events WHERE position = ?`, position)
	if err != nil {
		return "", false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return "", false, rows.Err()
	}
	var at, stream, actorKind, actorID, kind, correlation, data string
	if err := rows.Scan(&position, &at, &stream, &actorKind, &actorID, &kind, &correlation, &data); err != nil {
		return "", false, err
	}
	// A JSON array of the columns, so no two rows' columns run together alike.
	row, err := json.Marshal([]any{position, at, stream, actorKind, actorID, kind, correlation, data})
	if err != nil {
		return "", false, err
	}
	sum := sha256.Sum256(row)
	return hex.EncodeToString(sum[:]), true, nil
}

// Past is the log as Open loads it, up to its last position, read in Open's
// one transaction: what a Resumer checks its copy against.
type Past struct {
	tx        *sql.Tx
	catalogue *events.Catalogue
	last      int64
}

// Last is the log's last position, rejections included.
func (p Past) Last() int64 { return p.last }

// Hash is Store.Hash, in the log as Open loads it.
func (p Past) Hash(position int64) (hash string, found bool, err error) {
	if position > p.last {
		return "", false, nil
	}
	return hashOf(p.tx, position)
}

// Stream is a stream's accepted events at or before through, in order, as
// Store.Stream gives them.
func (p Past) Stream(name string, through int64) ([]events.Event, error) {
	return queryEvents(p.tx, `SELECT `+columns+` FROM events
		WHERE stream = ? AND type != ? AND position <= ? ORDER BY position`, name, events.ProposalRejected, min(through, p.last))
}

// ApplyAfter gives the model every event after since, as Open gives them to
// a model that starts part way.
func (p Past) ApplyAfter(model Model, since int64) error {
	return applyAfter(p.tx, p.catalogue, model, since, p.last)
}

// LastPosition is the position of the newest event in the model, 0 for an
// empty log.
func (s *Store) LastPosition() int64 { return s.applied.Load() }

// load fills the model and returns the log's last position. A Resumer is
// first given the log, to check its saved copy against. A model that holds
// nothing is then given each stream's accepted events, streams in the order
// of their first accepted event (tasks in creation order); one that starts
// part way is given each event after its position, rejections included, as
// Propose gives them. It runs in Open, before applied is set and before any
// reader exists, so it reads everything up to that last position, in one
// transaction for one snapshot.
func load(db *sql.DB, catalogue *events.Catalogue, model Model) (int64, error) {
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
	if resumer, ok := model.(Resumer); ok {
		if err := resumer.Resume(Past{tx: tx, catalogue: catalogue, last: last}); err != nil {
			return 0, err
		}
	}
	if since := model.Since(); since > 0 {
		return last, applyAfter(tx, catalogue, model, since, last)
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

// applyAfter brings a model that starts part way up to the log's last
// position. A model past the end was made from some other log.
func applyAfter(tx *sql.Tx, catalogue *events.Catalogue, model Model, since, last int64) error {
	if since > last {
		return fmt.Errorf("the model is at position %d, past the log's last, %d", since, last)
	}
	tail, err := queryEvents(tx, `SELECT `+columns+` FROM events
		WHERE position > ? AND position <= ? ORDER BY position`, since, last)
	if err != nil {
		return err
	}
	for _, e := range tail {
		before, err := beforeRefold(tx, catalogue, e)
		if err != nil {
			return err
		}
		give(model, catalogue, e, before)
	}
	return nil
}
