package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Redacting rewrites the rows of a stream that hold a secret, with every
// copy of it replaced by [redacted] and the rest of each row as it was,
// and appends an event.redacted that says who asked, why, and which rows
// and fields changed, never the secret nor its hash
// (docs/design/log-growth.md section 5, "Removing a secret"). The update
// trigger lets exactly the rows it names change, and only their data.
//
// What the store can't do is left to its caller: a secret in an archived
// task, which is unarchived, redacted and archived again; one in a
// knowledge file, whose versions are rewritten as full copies; the copies
// in other streams; and writing a new snapshot.

// Redacted is what a secret becomes.
const Redacted = "[redacted]"

// redactor writes the redactions, for whoever asked.
var redactor = events.Actor{Kind: events.System, ID: "redactor"}

// Redact rewrites a stream's rows that hold secret, for who, and appends
// event.redacted. First, with no lock held, it reads the stream's rows,
// rejections included, and finds the ones whose text holds the secret: in
// the strings of their data, decoded, so a secret with quotes, backslashes
// or any other text JSON escapes is found as it was written, and encoded
// again, so each row stays valid JSON with the same keys. Then, under the
// lock, it checks the stream took nothing since and the rules allow the
// redaction, and in one transaction appends it and updates the rows it
// names, the old bytes zeroed by the connection's secure_delete (Open);
// the model refolds the stream from the rewritten rows once that commits.
// Last, still under the lock, it empties the WAL (giveBackSpace), which
// still held the old rows. It writes no snapshot: the one on disk may
// still hold the secret, and its caller writes a new one.
//
// It returns the redaction and, when it is refused, the rejection: no row
// holds the secret, a row that does is a knowledge version, the reason
// given holds it, the rules refuse it, or the stream took an event since
// its rows were read. A refusal writes nothing, as Archive's does. The
// error is for the database failing. One after the commit comes with the
// redaction: the rows are rewritten, but the old text may stay in the WAL
// until GiveBackSpace runs again.
func (s *Store) Redact(stream, secret string, who events.Actor, why string) (events.Event, *events.Rejection, error) {
	found, rejection, err := s.find(stream, secret, who, why)
	if rejection != nil || err != nil {
		return events.Event{}, rejection, err
	}
	return s.rewrite(found)
}

// pending is what Redact found to rewrite, with no lock held: the
// redaction, the stream's last row then, and its rows that held the
// secret, rewritten.
type pending struct {
	redaction events.Proposal
	through   int64
	rows      []rewrite
}

// find is Redact's first half, with no lock held: the stream's rows that
// hold secret, rewritten, and the redaction that names them.
func (s *Store) find(stream, secret string, who events.Actor, why string) (pending, *events.Rejection, error) {
	if reason := aSecret(secret); reason != "" {
		return pending{}, &events.Rejection{Check: "redact", Reason: reason}, nil
	}
	if strings.Contains(why, secret) {
		return pending{}, &events.Rejection{Check: "redact", Reason: "the reason holds the secret, which the log would then keep"}, nil
	}
	rows, err := rowsOf(s.reads, stream, s.applied.Load())
	if err != nil {
		return pending{}, nil, err
	}
	found, err := rewritten(rows, secret)
	if err != nil {
		return pending{}, nil, fmt.Errorf("redacting %s: %w", stream, err)
	}
	if rejection := redactable(stream, found); rejection != nil {
		return pending{}, rejection, nil
	}
	return pending{redaction: redactionOf(stream, who, why, found), through: rows[len(rows)-1].Position, rows: found}, nil, nil
}

// rewrite is Redact's second half, under the lock: the checks, the
// commit, the refold and the WAL emptied.
func (s *Store) rewrite(found pending) (events.Event, *events.Rejection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stream := found.redaction.Stream
	if rejection, err := s.unmoved(stream, found.through, "redact", "find its rows again"); rejection != nil || err != nil {
		return events.Event{}, rejection, err
	}
	if rejection := s.validate(found.redaction); rejection != nil {
		return events.Event{}, rejection, nil
	}
	event, before, err := s.commitRedact(found.redaction, found.rows)
	if err != nil {
		return events.Event{}, nil, err
	}
	if s.model != nil {
		give(s.model, s.catalogue, event, before)
	}
	s.applied.Store(event.Position)
	s.changes.notify()
	if err := s.giveBackSpace(); err != nil {
		return event, nil, fmt.Errorf("%s is redacted, but the old text may stay in the WAL for now: %w", stream, err)
	}
	return event, nil, nil
}

// Matches is how many of a stream's rows, rejections included, hold
// secret, as Redact would find them: what to show before redacting.
func (s *Store) Matches(stream, secret string) (int, error) {
	if reason := aSecret(secret); reason != "" {
		return 0, fmt.Errorf("%s", reason)
	}
	rows, err := rowsOf(s.reads, stream, s.applied.Load())
	if err != nil {
		return 0, err
	}
	rewrites, err := rewritten(rows, secret)
	return len(rewrites), err
}

// aSecret says what is wrong with a secret to redact: one that is empty,
// or part of what it becomes, would never be gone.
func aSecret(secret string) string {
	if secret == "" || strings.Contains(Redacted, secret) {
		return "a secret to redact is some text, and not part of " + Redacted
	}
	return ""
}

// A rewrite is a row with its secret redacted.
type rewrite struct {
	position int64
	kind     events.Type
	data     string   // the row's data, rewritten
	fields   []string // the JSON paths of the strings that held the secret
	removed  int      // the secret's bytes taken out
}

// rewritten is the rows that hold secret, rewritten, in order.
func rewritten(rows []row, secret string) ([]rewrite, error) {
	var rewrites []rewrite
	for _, r := range rows {
		data, fields, removed, err := redactedData(r.Data, secret)
		if err != nil {
			return nil, fmt.Errorf("the data at %d: %w", r.Position, err)
		}
		if len(fields) > 0 {
			rewrites = append(rewrites, rewrite{position: r.Position, kind: events.Type(r.Type), data: data, fields: fields, removed: removed})
		}
	}
	return rewrites, nil
}

// redactedData is a row's data with secret replaced in every string, keys
// left as they are, and where it was. Numbers are decoded as their text,
// so they are written back as they were; the rest is encoded as appendEvent
// encoded it. No fields means the data doesn't hold the secret.
func redactedData(data, secret string) (string, []string, int, error) {
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return "", nil, 0, err
	}
	var found redaction
	value = found.in(value, "$", secret)
	if len(found.fields) == 0 {
		return "", nil, 0, nil
	}
	encoded, err := json.Marshal(value)
	return string(encoded), found.fields, found.removed, err
}

// A redaction is what redacting one row's data found.
type redaction struct {
	fields  []string
	removed int
}

// in is value at path with secret replaced in its strings, which it
// notes. A map's keys are walked in order, so the paths come out the same
// each time.
func (r *redaction) in(value any, path, secret string) any {
	switch value := value.(type) {
	case string:
		if copies := strings.Count(value, secret); copies > 0 {
			r.fields = append(r.fields, path)
			r.removed += copies * len(secret)
			return strings.ReplaceAll(value, secret, Redacted)
		}
	case map[string]any:
		for _, key := range slices.Sorted(maps.Keys(value)) {
			value[key] = r.in(value[key], path+"."+pathKey(key), secret)
		}
	case []any:
		for i := range value {
			value[i] = r.in(value[i], path+"["+strconv.Itoa(i)+"]", secret)
		}
	}
	return value
}

// plainKey is a key a JSON path can name as it is.
var plainKey = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// pathKey is a key as SQLite's JSON paths write it: quoted unless plain.
func pathKey(key string) string {
	if plainKey.MatchString(key) {
		return key
	}
	return strconv.Quote(key)
}

// redactable refuses a redaction the rows found can't make: none holds the
// secret, or one is a knowledge version, which later versions patch: a
// patch's head and tail would be wrong against a redacted text, so those
// are rewritten as full copies instead.
func redactable(stream string, rewrites []rewrite) *events.Rejection {
	if len(rewrites) == 0 {
		return &events.Rejection{Check: "redact", Reason: "no row of " + stream + " holds the secret"}
	}
	for _, rw := range rewrites {
		if rw.kind == events.AgentLearned {
			return &events.Rejection{Check: "redact", Reason: fmt.Sprintf("the row at %d is a knowledge version, which is redacted by writing it again in full", rw.position)}
		}
	}
	return nil
}

// redactionOf is the event.redacted for the rows rewritten.
func redactionOf(stream string, who events.Actor, why string, rewrites []rewrite) events.Proposal {
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
func (s *Store) commitRedact(redaction events.Proposal, rewrites []rewrite) (events.Event, []events.Event, error) {
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
func rewriteRow(tx *sql.Tx, stream string, rw rewrite) error {
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
