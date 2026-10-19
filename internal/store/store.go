// Package store is the append-only event log, in SQLite.
//
// Triggers make UPDATE and DELETE fail in the database itself, so
// "append-only" does not depend on the code remembering; the one delete
// allowed is of a task's rows that its task.archived marker covers (see
// migrations). Propose is the way in for every event but the markers,
// which only Archive and Unarchive write: it runs the checks against the model, appends
// the event (or a proposal.rejected) and applies it to the model. A mutex
// makes proposals strictly one at a time, from the check to the apply, so
// two can never both pass the order check against the same state (proof
// 05). Reads of the log go through a separate pool of read-only
// connections, so they never queue behind a write: in WAL mode a reader sees
// every committed event and none in flight.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sync"
	"sync/atomic"
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

// migrations bring a log made by an older build up to this one's schema:
// migrations[i] takes a file at PRAGMA user_version i to i+1, and a new
// file starts at 0 and runs them all. The schema's version is the list's
// length, so a new migration is only a new entry.
var migrations = []string{
	// 1: archiving (docs/design/log-growth.md section 4). A delete is
	// allowed only for rows a task.archived marker in the same stream
	// covers, and only until a task.unarchived follows that marker. The
	// marker counts inside its own uncommitted transaction, so the archiver
	// inserts it and deletes the rows in one. The trigger checks every row
	// deleted: the index of markers keeps that a lookup, where a scan of the
	// stream per row took 4.6 s to delete 8,000.
	`CREATE INDEX events_archived ON events (stream, position) WHERE type = 'task.archived';
DROP TRIGGER IF EXISTS events_no_delete;
CREATE TRIGGER events_no_delete BEFORE DELETE ON events
WHEN NOT EXISTS (
	SELECT 1 FROM events m
	WHERE m.stream = OLD.stream AND m.type = 'task.archived'
	  AND OLD.position <= json_extract(m.data, '$.through')
	  AND NOT EXISTS (SELECT 1 FROM events u WHERE u.stream = m.stream
	                  AND u.type = 'task.unarchived' AND u.position > m.position))
BEGIN SELECT RAISE(ABORT, 'events are append-only'); END;`,
}

// A Model is state kept from the log. The store fills it at Open and
// applies each event after it commits, before any reader of the log can
// read that event.
//
// A model may start part way through the log, as one read from a snapshot
// would: Since says where it is, and Open applies only the events after it
// instead of loading every stream.
//
// An event that changes what came before it in its stream (the catalogue's
// Refolds) is given to Refold in place of Apply, with the stream's accepted
// events before it, since the model can't read the log itself.
type Model interface {
	Since() int64                                                // the last position the model holds, 0 when it holds nothing
	Load(stream string, accepted []events.Event)                 // at Open, once per stream, when Since is 0
	Loaded(position int64)                                       // at Open, the log's last position, rejections included, when Since is 0
	Apply(e events.Event)                                        // after each commit, and at Open each event after Since
	Refold(stream string, before []events.Event, e events.Event) // in place of Apply, for an event that refolds its stream
}

// A Resumer is a Model that may start from a saved copy of itself, such as
// a snapshot, which may not fit the log it is opened on. Open calls Resume
// first, with the log as it loads it: the model checks its copy against the
// log, finishes it and brings it up to the log's last position, or empties
// itself to be loaded whole. Since then says which. The error is for the
// database failing.
type Resumer interface {
	Resume(log Past) error
}

// OrderCheck is check 3, order and per-agent who rules: given what happened so
// far, may this proposal happen now? It lives outside the store (package
// rules) and reads the model it was made with.
type OrderCheck func(p events.Proposal) *events.Rejection

type Store struct {
	db        *sql.DB // the one connection that writes
	reads     *sql.DB // read-only connections for the reads in read.go
	catalogue *events.Catalogue
	model     Model // nil when nothing keeps state from the log
	check     OrderCheck
	changes   *changes

	mu      sync.Mutex   // one proposal at a time, from its check to its apply
	applied atomic.Int64 // the last position in the model; no read of the log goes past it
}

// Driver names the database/sql driver Open uses: modernc's SQLite. A test
// can name a wrapper around it instead, to count the statements sent.
var Driver = "sqlite"

// readConnections is how many reads of the log can run at once.
const readConnections = 4

// Open opens the log at path, migrates it to this build's schema and loads
// it into model, if there is one. Every proposal must be of a type in
// catalogue, in a stream it belongs in.
//
// auto_vacuum is set first, so a new file gives back the pages archiving
// frees. An existing file switches mode only with a full VACUUM, which
// Open does not run: the archiver runs it once, after its first round,
// when the done tasks are gone and the copy is cheapest (log-growth.md
// section 4, "VACUUM").
func Open(path string, catalogue *events.Catalogue, model Model, check OrderCheck) (*Store, error) {
	db, err := sql.Open(Driver, path+"?_pragma=auto_vacuum(INCREMENTAL)&_pragma=journal_mode(WAL)&_pragma=synchronous(NORMAL)&_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one connection, which writes; Store.mu orders the proposals
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("creating the schema: %w", err)
	}
	if err := migrate(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrating the schema: %w", err)
	}
	last, err := load(db, catalogue, model)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("loading the log: %w", err)
	}
	// Opened after the schema, which made the file and switched it to WAL.
	reads, err := sql.Open(Driver, path+"?_pragma=query_only(1)&_pragma=busy_timeout(5000)")
	if err != nil {
		db.Close()
		return nil, err
	}
	reads.SetMaxOpenConns(readConnections)
	// sql.Open connects lazily: a bad path or pragma fails here, not at the first read.
	if err := reads.Ping(); err != nil {
		reads.Close()
		db.Close()
		return nil, fmt.Errorf("opening the read pool: %w", err)
	}
	s := &Store{db: db, reads: reads, catalogue: catalogue, model: model, check: check, changes: newChanges()}
	s.applied.Store(last)
	return s, nil
}

// migrate runs the migrations the file hasn't had, and records the new
// version, in one transaction: a failure leaves the file as it was.
func migrate(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var version int
	if err := tx.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return err
	}
	if version > len(migrations) {
		return fmt.Errorf("the log is at schema version %d, newer than this build's %d", version, len(migrations))
	}
	if version == len(migrations) {
		return nil
	}
	for i, migration := range migrations[version:] {
		if _, err := tx.Exec(migration); err != nil {
			return fmt.Errorf("to version %d: %w", version+i+1, err)
		}
	}
	// PRAGMA takes no parameters; the number is the list's length.
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, len(migrations))); err != nil {
		return err
	}
	return tx.Commit()
}

// Catalogue is the event types the log was opened with.
func (s *Store) Catalogue() *events.Catalogue { return s.catalogue }

func (s *Store) Close() error { return errors.Join(s.reads.Close(), s.db.Close()) }

// Propose validates a proposal and appends the result. It returns the stored
// event and, when the proposal was refused, the rejection (the stored event is
// then the proposal.rejected). The error is for the database failing, not for a refusal.
//
// The order matters: the model takes the event only once it is committed,
// and readers of the log see it only once the model has it.
func (s *Store) Propose(p events.Proposal) (events.Event, *events.Rejection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.propose(p)
}

// Create is Propose for the event that starts a new stream, such as
// task.created or idea.suggested. The stream is named prefix and the next
// number, counted under the same lock, so two made at once never take the
// same name, and never an archived task's (see made).
func (s *Store) Create(prefix string, p events.Proposal) (events.Event, *events.Rejection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	made, err := s.made(p.Type)
	if err != nil {
		return events.Event{}, nil, err
	}
	p.Stream = events.Numbered(prefix, made)
	return s.propose(p)
}

// made counts the events of a type in the log, rejections left out, so the
// next stream of its kind follows every one made, even one whose stream
// can't be read.
//
// For tasks it counts task.created rows and task.archived markers
// together, since archiving deletes a task's task.created row: each marker
// at the task.created position it stands for (its "created"), so a task
// counts once whether archived, unarchived (the row is back, at that
// position) or archived twice (two markers, one "created"). The markers are
// gated on that position, not their own: between the archive's commit and
// applied moving past its marker, a gate on the marker would hide the task
// from both halves (log-growth.md section 4, "How it fits").
func (s *Store) made(kind events.Type) (int, error) {
	query, args := `SELECT COUNT(*) FROM events WHERE type = ? AND position <= ?`, []any{kind, s.applied.Load()}
	if kind == events.TaskCreated {
		query = `SELECT COUNT(*) FROM (
			SELECT stream, position FROM events WHERE type = ?1 AND position <= ?2
			UNION
			SELECT stream, json_extract(data, '$.created') FROM events
			WHERE type = ?3 AND json_extract(data, '$.created') <= ?2)`
		args = append(args, events.TaskArchived)
	}
	var made int
	err := s.db.QueryRow(query, args...).Scan(&made)
	return made, err
}

// byTheStore are the types the store writes itself, through Archive and
// Unarchive: Propose refuses them from anyone, so no marker goes into the
// log without its rows moving with it.
var byTheStore = []events.Type{events.TaskArchived, events.TaskUnarchived}

// propose is Propose, under Store.mu.
func (s *Store) propose(p events.Proposal) (events.Event, *events.Rejection, error) {
	rejection := s.proposable(p)
	stored := p
	if rejection != nil {
		stored = rejectionOf(p, rejection)
	}
	event, before, err := s.commit(stored)
	if err != nil {
		return events.Event{}, nil, err
	}
	if s.model != nil {
		give(s.model, s.catalogue, event, before)
	}
	s.applied.Store(event.Position)
	s.changes.notify()
	return event, rejection, nil
}

// proposable is validate, for a proposal: the types only the store writes
// are refused first.
func (s *Store) proposable(p events.Proposal) *events.Rejection {
	if slices.Contains(byTheStore, p.Type) {
		return &events.Rejection{Check: "who", Reason: fmt.Sprintf("only the store writes %s", p.Type)}
	}
	return s.validate(p)
}

func (s *Store) validate(p events.Proposal) *events.Rejection {
	if rejection := s.catalogue.CheckShape(p); rejection != nil {
		return rejection
	}
	if rejection := s.catalogue.CheckWriter(p); rejection != nil {
		return rejection
	}
	if s.check == nil {
		return nil
	}
	return s.check(p)
}

// commit appends one event in its own transaction. For an event that
// refolds its stream, it also returns the stream's accepted events before
// it, read in the same transaction: a read that fails then undoes the
// event, rather than leave it in the log and out of the model.
func (s *Store) commit(p events.Proposal) (events.Event, []events.Event, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return events.Event{}, nil, err
	}
	defer tx.Rollback()
	event, err := appendEvent(tx, p)
	if err != nil {
		return events.Event{}, nil, err
	}
	before, err := beforeRefold(tx, s.catalogue, event)
	if err != nil {
		return events.Event{}, nil, err
	}
	return event, before, tx.Commit()
}

// beforeRefold is the stream's accepted events before e, when e refolds
// its stream; nil otherwise. It never reads past e: at Open the log holds
// the rest of the tail too.
func beforeRefold(db querier, catalogue *events.Catalogue, e events.Event) ([]events.Event, error) {
	if !catalogue.Refolds(e.Type) {
		return nil, nil
	}
	return queryEvents(db, `SELECT `+columns+` FROM events
		WHERE stream = ? AND type != ? AND position < ? ORDER BY position`, e.Stream, events.ProposalRejected, e.Position)
}

// give hands the model a committed event: to Refold, with the stream's
// events before it, when it refolds its stream, and to Apply otherwise.
func give(model Model, catalogue *events.Catalogue, e events.Event, before []events.Event) {
	if catalogue.Refolds(e.Type) {
		model.Refold(e.Stream, before, e)
		return
	}
	model.Apply(e)
}

func rejectionOf(p events.Proposal, rejection *events.Rejection) events.Proposal {
	return events.Proposal{
		Stream: p.Stream, Actor: events.Actor{Kind: events.System, ID: "validator"},
		Type: events.ProposalRejected, Correlation: p.Correlation,
		Data: map[string]any{"type": p.Type, "by": p.Actor.String(), "check": rejection.Check, "reason": rejection.Reason},
	}
}

// appendEvent returns the event as a reader reads it back, not as proposed:
// Data decoded from the JSON written (an []string{} or an int64 comes back as
// replay sees it) and At parsed from the text written (without the monotonic
// reading). So the model, applying this, equals a replay of the log.
func appendEvent(tx *sql.Tx, p events.Proposal) (events.Event, error) {
	data, err := json.Marshal(p.Data)
	if err != nil {
		return events.Event{}, fmt.Errorf("encoding the data of %s: %w", p.Type, err)
	}
	at := time.Now().UTC().Format(time.RFC3339Nano)
	result, err := tx.Exec(`INSERT INTO events (at, stream, actor_kind, actor_id, type, correlation, data)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		at, p.Stream, p.Actor.Kind, p.Actor.ID, p.Type, p.Correlation, string(data))
	if err != nil {
		return events.Event{}, err
	}
	event := events.Event{Proposal: p}
	if event.Position, err = result.LastInsertId(); err != nil {
		return events.Event{}, err
	}
	return event, decode(&event, at, string(data))
}
