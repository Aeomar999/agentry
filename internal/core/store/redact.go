package store

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
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
// It returns the redaction and, when it is refused, the rejection: the
// stream is erased or archived, which comes before what its rows hold, no
// row holds the secret, a row that does is a knowledge version outside the
// agents stream, holds it in a key, can't be rewritten leaving the rest as
// it was or would still hold it once rewritten, the reason given holds it,
// the rules refuse it, or the stream took an event since its rows were
// read. A refusal writes nothing, as Archive's does. The error is for the
// database failing. One after the commit comes with the redaction: the
// rows are rewritten, but the old text may stay in the WAL until
// GiveBackSpace runs again.
func (s *Store) Redact(stream, secret string, who events.Actor, why string) (events.Event, *events.Rejection, error) {
	return s.redact(stream, secret, nil, AnyCount, who, why)
}

// RedactCounted is Redact of the rows the user counted: when rows is not
// AnyCount, it is refused unless that many rows hold the secret as it
// commits. The user confirms a redaction after Preview counted the rows,
// and a row written since, such as a message holding the text, would
// otherwise be rewritten without their seeing it. The count is checked
// under the lock, with the stream's rows as the commit finds them.
func (s *Store) RedactCounted(stream, secret string, rows int, who events.Actor, why string) (events.Event, *events.Rejection, error) {
	return s.redact(stream, secret, nil, rows, who, why)
}

// RedactRows is Redact of only the rows of stream at positions, such as
// the rows of an epic that copy an erased story's words: a short text
// such as a title may be in its other rows too, written there by the epic
// itself. Every check Redact makes is made of those rows; one that doesn't
// hold the secret is left as it is, and when none does, it is refused as
// Redact is when no row does.
func (s *Store) RedactRows(stream, secret string, positions []int64, who events.Actor, why string) (events.Event, *events.Rejection, error) {
	return s.redact(stream, secret, onlyAt(positions), AnyCount, who, why)
}

// redact is Redact of the rows named, refused unless counted of them hold
// the secret.
func (s *Store) redact(stream, secret string, named rowsNamed, counted int, who events.Actor, why string) (events.Event, *events.Rejection, error) {
	found, rejection, err := s.find(stream, secret, named, who, why)
	if rejection != nil || err != nil {
		return events.Event{}, rejection, err
	}
	found.counted = counted
	return s.rewrite(found)
}

// AnyCount (0) asks for no check of how many rows hold the secret: as a
// redaction always rewrites at least one row, 0 can't be a real count.
const AnyCount = 0

// rowsNamed is the rows a redaction may rewrite, by position: every row of
// its stream when nil.
type rowsNamed map[int64]bool

// onlyAt names the rows at positions, and only those: none when there are
// none.
func onlyAt(positions []int64) rowsNamed {
	named := rowsNamed{}
	for _, position := range positions {
		named[position] = true
	}
	return named
}

// of is the rows found that are named.
func (named rowsNamed) of(found []rewrittenRow) []rewrittenRow {
	if named == nil {
		return found
	}
	return slices.DeleteFunc(found, func(rw rewrittenRow) bool { return !named[rw.position] })
}

// pending is what Redact found to rewrite, with no lock held: the
// redaction, the stream's last row then, its rows that held the secret,
// rewritten, and how many the user counted.
type pending struct {
	redaction events.Proposal
	through   int64
	rows      []rewrittenRow
	counted   int
}

// recounted refuses a redaction whose rows aren't the number the user
// counted, unless they counted none (AnyCount).
func (found pending) recounted() *events.Rejection {
	held := len(found.rows)
	if found.counted == AnyCount || held == found.counted {
		return nil
	}
	return &events.Rejection{Check: CountCheck, Reason: fmt.Sprintf("%s this text now, not %d: count them again", RowsHolding(held), found.counted)}
}

// CountCheck is the check of a redaction refused as its rows aren't the
// number the user counted.
const CountCheck = "count"

// Miscounted says whether rejection is a redaction's refusal because the
// rows that hold the secret aren't the number the user counted, which
// counting them again and confirming that count would redact.
func Miscounted(rejection *events.Rejection) bool {
	return rejection != nil && rejection.Check == CountCheck
}

// RowsHolding is how many rows hold a text, in words.
func RowsHolding(rows int) string {
	if rows == 1 {
		return "1 row holds"
	}
	return strconv.Itoa(rows) + " rows hold"
}

// find is Redact's first half, with no lock held: the stream's rows named
// that hold secret, rewritten, and the redaction that names them. An
// erased or archived stream is refused for that first, whatever its live
// rows hold.
func (s *Store) find(stream, secret string, named rowsNamed, who events.Actor, why string) (pending, *events.Rejection, error) {
	logged, err := s.Log(stream)
	if err != nil {
		return pending{}, nil, err
	}
	if rejection := asItIs(stream, logged); rejection != nil {
		return pending{}, rejection, nil
	}
	rows, err := rowsOf(s.reads, stream, s.applied.Load())
	if err != nil {
		return pending{}, nil, err
	}
	return findIn(stream, rows, secret, named, who, why)
}

// findIn is the rows named, of stream, that hold secret, rewritten, and
// the redaction that names them, or why it is refused.
func findIn(stream string, rows []row, secret string, named rowsNamed, who events.Actor, why string) (pending, *events.Rejection, error) {
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
	found = named.of(found)
	if rejection := redactable(stream, found); rejection != nil {
		return pending{}, rejection, nil
	}
	return pending{redaction: redactionOf(stream, who, why, found), through: rows[len(rows)-1].Position, rows: found}, nil, nil
}

// rewrite is Redact's second half, under the lock: the checks, the count
// among them, the commit, the refold and the WAL emptied. As the stream
// took nothing since its rows were read, they are the rows the commit
// rewrites.
func (s *Store) rewrite(found pending) (events.Event, *events.Rejection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	stream := found.redaction.Stream
	if rejection, err := s.unmoved(stream, found.through, "redact", "find its rows again"); rejection != nil || err != nil {
		return events.Event{}, rejection, err
	}
	if rejection := found.recounted(); rejection != nil {
		return events.Event{}, rejection, nil
	}
	if rejection := s.refusal(found); rejection != nil {
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

// refusal is why rewritable or the rules refuse what Redact found, under
// the lock. rewritable comes first: a row it can't rewrite, such as one
// holding the secret only in a number, has no fields in the redaction,
// and the rules would refuse that for naming none instead of saying why.
func (s *Store) refusal(found pending) *events.Rejection {
	if rejection := s.rewritable(found.redaction.Stream, found.rows); rejection != nil {
		return rejection
	}
	return s.validate(found.redaction)
}

// Preview is what Redact would do with secret in stream, writing nothing:
// the number of rows it would rewrite or, when it would refuse, its
// rejection and none. It is what the page shows before the user confirms.
// The rules are asked as Redact asks them, for a redaction with no reason
// yet, by redactor, as who asks doesn't change what they allow; a reason
// that holds the secret is refused when the redaction is asked for. The
// error is for the database failing.
func (s *Store) Preview(stream, secret string) (int, *events.Rejection, error) {
	found, rejection, err := s.find(stream, secret, nil, redactor, "")
	if rejection != nil || err != nil {
		return 0, rejection, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if rejection := s.refusal(found); rejection != nil {
		return 0, rejection, nil
	}
	return len(found.rows), nil, nil
}
