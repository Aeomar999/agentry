package store

import (
	"encoding/json"
	"errors"
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// An archived task's rows are in a blob, so its secret is redacted by
// putting them back, redacting them and archiving them again in a new blob
// (docs/design/log-growth.md section 5, "Removing a secret", item 5). The
// blobs that held them before still hold the secret, so they are deleted,
// and the event.redacted names them in "archives".

// ArchiveBlobs is where archived tasks' rows are kept, read and deleted:
// sharedfiles.Disk.
type ArchiveBlobs interface {
	Blobs
	Remover
}

// RedactArchived rewrites an archived task's rows that hold secret, for
// who. First, with no lock held, it reads the rows from its marker's blob
// and the live log, and finds the secret in them as Redact does, so what
// Redact would refuse for the rows themselves (no row holds it, a row is a
// knowledge version, holds it in a key or can't be rewritten leaving the
// rest as it was, or the reason holds it) is refused before anything is
// written. Then it unarchives the task (Unarchive), redacts it, with the
// blobs every task.archived among its rows names in "archives", and
// archives it again (Archive), summarise reading the state the redaction
// refolded; each takes the lock for itself, as when called alone, so other
// events may come between them. Last it deletes those blobs. It writes no
// snapshot, as Redact doesn't: its caller writes a new one.
//
// It returns the redaction or, when nothing is redacted, the rejection.
// Refused before the unarchiving, or by Unarchive, nothing is written.
// Refused after it, by the rules or because the stream took an event, the
// task is archived again in a new blob and the old blobs are kept, since
// its rows still hold the secret; when that archive fails, an error comes
// with the rejection saying the task is left unarchived. The error is
// otherwise for the database or the blobs failing, the same way. Once the
// redaction commits, an error comes with it: the task is redacted and its
// old blobs deleted, but it stays unarchived when archiving it again fails
// or is refused, and some of the secret may stay in the WAL or a blob that
// won't delete, as with Erase.
func (s *Store) RedactArchived(stream, secret string, who events.Actor, why string, summarise func() any, blobs ArchiveBlobs) (events.Event, *events.Rejection, error) {
	if rejection, err := s.checkArchived(stream, secret, who, why, blobs); rejection != nil || err != nil {
		return events.Event{}, rejection, err
	}
	if _, rejection, err := s.Unarchive(stream, blobs); rejection != nil || err != nil {
		return events.Event{}, rejection, err
	}
	redaction, archives, rejection, err := s.redactUnarchived(stream, secret, who, why)
	if redaction.Position == 0 { // nothing committed: the old blobs still hold what the rows do
		return events.Event{}, rejection, errors.Join(err, s.archiveAgain(stream, summarise, blobs))
	}
	failed := errors.Join(err, s.archiveAgain(stream, summarise, blobs))
	for _, hash := range archives {
		if err := blobs.Remove(hash); err != nil {
			failed = errors.Join(failed, err)
		}
	}
	if failed != nil {
		return redaction, nil, fmt.Errorf("%s is redacted, but not all of it is done: %w", stream, failed)
	}
	return redaction, nil, nil
}

// checkArchived refuses, before anything is written, what Redact would
// refuse in an archived stream's rows, read from its marker's blob and the
// live log, or a stream that isn't archived, which Redact redacts as it is.
func (s *Store) checkArchived(stream, secret string, who events.Actor, why string, blobs Opener) (*events.Rejection, error) {
	live, err := s.Log(stream)
	if err != nil {
		return nil, err
	}
	marker, archived := MarkerOf(live)
	if !archived {
		return &events.Rejection{Check: "redact", Reason: stream + " isn't archived: it is redacted as it is"}, nil
	}
	kept, err := keptBy(marker, blobs)
	if err != nil {
		return nil, fmt.Errorf("redacting %s: %w", stream, err)
	}
	rest, err := rowsOf(s.reads, stream, s.applied.Load())
	if err != nil {
		return nil, err
	}
	found, rejection, err := findIn(stream, append(kept, rest...), secret, who, why)
	if rejection != nil || err != nil {
		return rejection, err
	}
	return rewritable(found.rows), nil
}

// redactUnarchived is Redact of a stream just unarchived, its redaction
// naming the blobs every task.archived among its rows names, which are
// returned to delete once it commits.
func (s *Store) redactUnarchived(stream, secret string, who events.Actor, why string) (events.Event, []string, *events.Rejection, error) {
	rows, err := rowsOf(s.reads, stream, s.applied.Load())
	if err != nil {
		return events.Event{}, nil, nil, err
	}
	archives, err := archivesIn(rows)
	if err != nil {
		return events.Event{}, nil, nil, fmt.Errorf("redacting %s: %w", stream, err)
	}
	found, rejection, err := findIn(stream, rows, secret, who, why)
	if rejection != nil || err != nil {
		return events.Event{}, nil, rejection, err
	}
	found.redaction.Data["archives"] = archives
	redaction, rejection, err := s.rewrite(found)
	return redaction, archives, rejection, err
}

// archivesIn is the blobs the task.archived rows among rows name.
func archivesIn(rows []row) ([]string, error) {
	archives := []string{}
	for _, r := range rows {
		if r.Type != string(events.TaskArchived) {
			continue
		}
		var marker struct {
			Archive string `json:"archive"`
		}
		if err := json.Unmarshal([]byte(r.Data), &marker); err != nil {
			return nil, fmt.Errorf("the marker at %d: %w", r.Position, err)
		}
		archives = append(archives, marker.Archive)
	}
	return archives, nil
}

// archiveAgain archives a stream RedactArchived unarchived, and says, when
// it can't, that the stream is left unarchived.
func (s *Store) archiveAgain(stream string, summarise func() any, blobs Blobs) error {
	_, rejection, err := s.Archive(stream, summarise, blobs)
	switch {
	case err != nil:
		return fmt.Errorf("%s is left unarchived: %w", stream, err)
	case rejection != nil:
		return fmt.Errorf("%s is left unarchived: archiving it again was refused: %s", stream, rejection.Reason)
	}
	return nil
}
