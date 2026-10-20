package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Redacting rewrites the rows of a stream that hold a secret, with every
// copy of it replaced by [redacted] and the rest of each row as it was,
// byte for byte, and appends an event.redacted that says who asked, why,
// and which rows and fields changed, never the secret nor its hash
// (docs/design/log-growth.md section 5, "Removing a secret"). The update
// trigger lets exactly the rows it names change, and only their data.
//
// A secret in an archived task is redacted by RedactArchived, which
// unarchives it, redacts it and archives it again; one in an agent's
// knowledge rewrites its versions from the first that holds it as full
// copies (redact_knowledge.go). What the store can't do is left to its
// caller: the copies in other streams, and writing a new snapshot.

// Redacted is what a secret becomes.
const Redacted = "[redacted]"

// redactor writes the redactions, for whoever asked.
var redactor = events.Actor{Kind: events.System, ID: "redactor"}

// Redact rewrites a stream's rows that hold secret, for who, and appends
// event.redacted. First, with no lock held, it reads the stream's rows,
// rejections included, and finds the ones whose text holds the secret: in
// the strings of their data, decoded, so a secret with quotes, backslashes
// or any other text JSON escapes is found as it was written, and in the
// agents stream in the knowledge each version rebuilds, so a patch whose
// text holds it, or whose base does, is written again as a full copy
// (knowledgeRewrites). Only those strings are encoded again, in place, so
// each row stays valid JSON with every other byte as it was, whatever order
// its keys are in. Then, under the lock, it checks the stream took nothing
// since and the rules allow the redaction, and in one transaction appends it
// and updates the rows it names, the old bytes zeroed by the connection's
// secure_delete (Open); the model refolds the stream from the rewritten rows
// once that commits. Last, still under the lock, it empties the WAL
// (giveBackSpace), which still held the old rows. It writes no snapshot: the
// one on disk may still hold the secret, and its caller writes a new one.
//
// It returns the redaction and, when it is refused, the rejection: no row
// holds the secret, a row that does is a knowledge version outside the
// agents stream, holds it in a key or can't be rewritten leaving the rest as
// it was, the reason given holds it, the rules refuse it, or the stream took
// an event since its rows were read.
// A refusal writes nothing, as Archive's does. The error is for the
// database failing. One after the commit comes with the redaction: the
// rows are rewritten, but the old text may stay in the WAL until
// GiveBackSpace runs again.
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
	rows      []rewrittenRow
}

// find is Redact's first half, with no lock held: the stream's rows that
// hold secret, rewritten, and the redaction that names them.
func (s *Store) find(stream, secret string, who events.Actor, why string) (pending, *events.Rejection, error) {
	rows, err := rowsOf(s.reads, stream, s.applied.Load())
	if err != nil {
		return pending{}, nil, err
	}
	return findIn(stream, rows, secret, who, why)
}

// findIn is the rows, of stream, that hold secret, rewritten, and the
// redaction that names them, or why it is refused.
func findIn(stream string, rows []row, secret string, who events.Actor, why string) (pending, *events.Rejection, error) {
	if reason := aSecret(secret); reason != "" {
		return pending{}, &events.Rejection{Check: "redact", Reason: reason}, nil
	}
	if strings.Contains(why, secret) {
		return pending{}, &events.Rejection{Check: "redact", Reason: "the reason holds the secret, which the log would then keep"}, nil
	}
	found, err := rewrittenIn(stream, rows, secret)
	if err != nil {
		return pending{}, nil, fmt.Errorf("redacting %s: %w", stream, err)
	}
	if rejection := redactable(stream, found); rejection != nil {
		return pending{}, rejection, nil
	}
	return pending{redaction: redactionOf(stream, who, why, found), through: rows[len(rows)-1].Position, rows: found}, nil, nil
}

// rewrite is Redact's second half, under the lock: the checks, the
// commit, the refold and the WAL emptied. The rules come before
// rewritable, so a row is refused for what it is before how it is written.
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
	if rejection := rewritable(found.rows); rejection != nil {
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
	rewrites, err := rewrittenIn(stream, rows, secret)
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

// A rewrittenRow is a row with its secret redacted.
type rewrittenRow struct {
	position int64
	kind     events.Type
	data     string   // the row's data, rewritten
	fields   []string // the JSON paths of the strings that held the secret
	removed  int      // the secret's bytes taken out
	refused  string   // why the row can't be rewritten, if it can't
	inKey    bool     // a key holds the secret, so the row has no fields
}

// holds says whether the row held the secret: in a string it rewrote, or
// where it refuses to rewrite it.
func (rw rewrittenRow) holds() bool { return len(rw.fields) > 0 || rw.refused != "" }

// rewritten is the rows that hold secret, rewritten, in order.
func rewritten(rows []row, secret string) ([]rewrittenRow, error) {
	var rewrites []rewrittenRow
	for _, r := range rows {
		rw, err := redactedData(r.Data, secret)
		if err != nil {
			return nil, fmt.Errorf("the data at %d: %w", r.Position, err)
		}
		if rw.holds() {
			rw.position, rw.kind = r.Position, events.Type(r.Type)
			rewrites = append(rewrites, rw)
		}
	}
	return rewrites, nil
}

// redactedData is a row's data with secret replaced in every string value,
// and where it was. Only the strings that held it are
// written again, escaped as appendEvent's json.Marshal escapes them, so a
// rewritten string looks like one the log wrote; every other byte is
// copied as it was: the keys in their order, which a struct's fields set,
// the numbers and the other strings. A string the log would have escaped
// otherwise, which it never writes, would change by more than the secret,
// so its row is refused, as is one whose secret would still show next to
// [redacted]. A row with the secret in a key is refused with its data as it
// was and no fields, whose paths would hold the secret too. A row that
// doesn't hold the secret comes back empty.
func redactedData(data, secret string) (rewrittenRow, error) {
	var found finding
	escapedOtherwise, inKey := false, false
	rewritten, err := eachString(data, func(text, raw, path string) (string, bool) {
		replaced, ok := found.in(text, path, secret)
		if !ok {
			return "", false
		}
		escapedOtherwise = escapedOtherwise || encoded(text) != raw
		return encoded(replaced), true
	}, func(key string) {
		inKey = inKey || strings.Contains(key, secret)
	})
	if err != nil {
		return rewrittenRow{}, err
	}
	if inKey {
		return rewrittenRow{data: data, refused: "holds the secret in a key, which redacting doesn't rewrite", inKey: true}, nil
	}
	if len(found.fields) == 0 {
		return rewrittenRow{}, nil
	}
	rw := rewrittenRow{data: rewritten, fields: found.fields, removed: found.removed}
	switch {
	case escapedOtherwise:
		rw.refused = "isn't written as the log writes data: a string holding the secret is escaped otherwise, so rewriting it would change more than the secret"
	case found.survives:
		rw.refused = "would still show the secret next to " + Redacted
	}
	return rw, nil
}

// eachString is data with each string value, not a key, put through
// rewrite: given its text, decoded, its raw bytes, quotes included, and
// its JSON path, rewrite says what to write in its place, or false to keep
// it. Every other byte is copied as it was. Keys are only shown to key,
// decoded, and kept as they are: no event type has a map whose keys a user
// writes, and a row whose key holds the secret is refused rather than left
// half-redacted.
func eachString(data string, rewrite func(text, raw, path string) (string, bool), key func(text string)) (string, error) {
	decoder := json.NewDecoder(strings.NewReader(data))
	decoder.UseNumber()
	var rewritten strings.Builder
	var at cursor
	copied, read := 0, 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		end := int(decoder.InputOffset())
		if text, ok := token.(string); ok && at.keyNext() {
			key(text)
		} else if ok {
			// Before a string are only spaces, a comma or a colon, none a quote.
			start := read + strings.IndexByte(data[read:end], '"')
			if replaced, ok := rewrite(text, data[start:end], at.path()); ok {
				rewritten.WriteString(data[copied:start])
				rewritten.WriteString(replaced)
				copied = end
			}
		}
		at.step(token)
		read = end
	}
	rewritten.WriteString(data[copied:])
	return rewritten.String(), nil
}

// A cursor is where a walk of JSON tokens is: the objects and arrays it is
// in, the innermost last.
type cursor []level

// A level is an object or an array a cursor is in.
type level struct {
	object  bool
	key     string // in an object, the key of the value read now
	index   int    // in an array, the index of the value read now
	keyNext bool   // in an object, the next string is a key
}

// keyNext says whether the next string is a key.
func (c cursor) keyNext() bool { return len(c) > 0 && c[len(c)-1].keyNext }

// path is the JSON path of the value read now.
func (c cursor) path() string {
	path := "$"
	for _, l := range c {
		if l.object {
			path += "." + pathKey(l.key)
		} else {
			path += "[" + strconv.Itoa(l.index) + "]"
		}
	}
	return path
}

// step moves the cursor past token.
func (c *cursor) step(token json.Token) {
	switch token {
	case json.Delim('{'):
		*c = append(*c, level{object: true, keyNext: true})
	case json.Delim('['):
		*c = append(*c, level{})
	case json.Delim('}'), json.Delim(']'):
		*c = (*c)[:len(*c)-1]
		c.pastValue()
	default:
		if key, ok := token.(string); ok && c.keyNext() {
			top := &(*c)[len(*c)-1]
			top.key, top.keyNext = key, false
			return
		}
		c.pastValue()
	}
}

// pastValue moves the cursor past a value: to an array's next index, or
// an object's next key.
func (c cursor) pastValue() {
	if len(c) == 0 {
		return
	}
	top := &c[len(c)-1]
	if top.object {
		top.keyNext = true
	} else {
		top.index++
	}
}

// A finding is what redacting one row's data found. Its fields are in the
// order the row holds them: for data appendEvent wrote, each object's keys
// sorted.
type finding struct {
	fields   []string
	removed  int
	survives bool // the secret is still in a string once replaced
}

// in is text, the string at path, with secret replaced, which it notes,
// or false when the text doesn't hold the secret.
func (f *finding) in(text, path, secret string) (string, bool) {
	copies := strings.Count(text, secret)
	if copies == 0 {
		return "", false
	}
	f.fields = append(f.fields, path)
	f.removed += copies * len(secret)
	replaced := strings.ReplaceAll(text, secret, Redacted)
	f.survives = f.survives || strings.Contains(replaced, secret)
	return replaced, true
}

// encoded is text as a JSON string, as appendEvent's json.Marshal writes it.
func encoded(text string) string {
	b, _ := json.Marshal(text) // a string always encodes
	return string(b)
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
// secret; one is a knowledge version outside the agents stream, whose
// versions rewrittenIn doesn't follow; or one holds it in a key, which is
// refused here, before the rules would refuse its redaction for naming no
// fields.
func redactable(stream string, rewrites []rewrittenRow) *events.Rejection {
	if len(rewrites) == 0 {
		return &events.Rejection{Check: "redact", Reason: "no row of " + stream + " holds the secret"}
	}
	for _, rw := range rewrites {
		if rw.kind == events.AgentLearned && stream != events.AgentsStream {
			return &events.Rejection{Check: "redact", Reason: fmt.Sprintf("the row at %d is a knowledge version, which is redacted by writing it again in full", rw.position)}
		}
	}
	for _, rw := range rewrites {
		if rw.inKey {
			return &events.Rejection{Check: "redact", Reason: fmt.Sprintf("the row at %d %s", rw.position, rw.refused)}
		}
	}
	return nil
}

// rewritable refuses a redaction with a row redactedData can't rewrite
// leaving the rest as it was.
func rewritable(rewrites []rewrittenRow) *events.Rejection {
	for _, rw := range rewrites {
		if rw.refused != "" {
			return &events.Rejection{Check: "redact", Reason: fmt.Sprintf("the row at %d %s", rw.position, rw.refused)}
		}
	}
	return nil
}

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
