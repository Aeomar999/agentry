// Package store is the append-only event log, in SQLite.
//
// Triggers make UPDATE and DELETE fail in the database itself, so
// "append-only" does not depend on the code remembering. Propose is the only
// way in: it reads the history, runs the checks and appends the event (or a
// proposal.rejected) in one transaction. The single connection makes
// proposals strictly one at a time, so two can never both pass the order
// check against the same state (proof 05).
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS events (
	position    INTEGER PRIMARY KEY AUTOINCREMENT,
	at          TEXT NOT NULL,
	stream      TEXT NOT NULL,
	actor_kind  TEXT NOT NULL,
	actor_id    TEXT NOT NULL,
	type        TEXT NOT NULL,
	correlation TEXT NOT NULL DEFAULT '',
	data        TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS events_by_stream ON events (stream, position);
CREATE TRIGGER IF NOT EXISTS events_no_update BEFORE UPDATE ON events
	BEGIN SELECT RAISE(ABORT, 'events are append-only'); END;
CREATE TRIGGER IF NOT EXISTS events_no_delete BEFORE DELETE ON events
	BEGIN SELECT RAISE(ABORT, 'events are append-only'); END;
`

// History is what the order check may read: the accepted events of any stream.
type History interface {
	Stream(name string) ([]events.Event, error)
}

// OrderCheck is check 3, order and per-agent who rules: given what happened so
// far, may this proposal happen now? It lives outside the store (package rules).
type OrderCheck func(p events.Proposal, history History) *events.Rejection

type Store struct {
	db      *sql.DB
	check   OrderCheck
	changes *changes
}

func Open(path string, check OrderCheck) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one writer: proposals are decided strictly in order
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating the schema: %w", err)
	}
	return &Store{db: db, check: check, changes: newChanges()}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// Propose validates a proposal and appends the result. It returns the stored
// event and, when the proposal was refused, the rejection (the stored event is
// then the proposal.rejected). The error is for the database failing, not for a refusal.
func (s *Store) Propose(p events.Proposal) (events.Event, *events.Rejection, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return events.Event{}, nil, err
	}
	defer tx.Rollback()

	rejection, err := s.validate(p, txHistory{tx})
	if err != nil {
		return events.Event{}, nil, err
	}
	stored := p
	if rejection != nil {
		stored = rejectionOf(p, rejection)
	}
	event, err := appendEvent(tx, stored)
	if err != nil {
		return events.Event{}, nil, err
	}
	if err := tx.Commit(); err != nil {
		return events.Event{}, nil, err
	}
	s.changes.notify()
	return event, rejection, nil
}

func (s *Store) validate(p events.Proposal, history History) (*events.Rejection, error) {
	if rejection := events.CheckShape(p); rejection != nil {
		return rejection, nil
	}
	if rejection := events.CheckWriter(p); rejection != nil {
		return rejection, nil
	}
	if s.check == nil {
		return nil, nil
	}
	return s.check(p, history), nil
}

func rejectionOf(p events.Proposal, rejection *events.Rejection) events.Proposal {
	return events.Proposal{
		Stream: p.Stream, Actor: events.Actor{Kind: events.System, ID: "validator"},
		Type: events.ProposalRejected, Correlation: p.Correlation,
		Data: map[string]any{"type": p.Type, "by": p.Actor.String(), "check": rejection.Check, "reason": rejection.Reason},
	}
}

func appendEvent(tx *sql.Tx, p events.Proposal) (events.Event, error) {
	data, err := json.Marshal(p.Data)
	if err != nil {
		return events.Event{}, fmt.Errorf("encoding the data of %s: %w", p.Type, err)
	}
	event := events.Event{At: time.Now().UTC(), Proposal: p}
	result, err := tx.Exec(`INSERT INTO events (at, stream, actor_kind, actor_id, type, correlation, data)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		event.At.Format(time.RFC3339Nano), p.Stream, p.Actor.Kind, p.Actor.ID, p.Type, p.Correlation, string(data))
	if err != nil {
		return events.Event{}, err
	}
	event.Position, err = result.LastInsertId()
	return event, err
}
