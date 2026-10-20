package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"slices"

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
// and the live log, with the task.unarchived the unarchiving adds, and
// deletes the blobs an earlier redaction among them named but couldn't
// delete (removeRedacted). Then it finds the secret in the rows as Redact
// does, so what Redact would refuse for the rows themselves (no row holds
// it, a row is a knowledge version, holds it in a key, can't be rewritten
// leaving the rest as it was or would still hold it once rewritten, or the
// reason holds it) is refused before anything is written. Then it
// unarchives the task (Unarchive), redacts it, with the blobs every
// task.archived among its rows names in "archives", and archives it again
// (Archive), summarise reading the state the redaction refolded; each
// takes the lock for itself, as when called alone, so other events may
// come between them. Last it deletes those blobs. It writes no snapshot,
// as Redact doesn't: its caller writes a new one.
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
//
// Archive puts its blob before it is refused, so when archiving again is
// refused or fails, that blob is removed unless a marker names it: no
// later run would, and after a refused redaction it holds the secret.
// When it was refused because another writer archived the task between
// the steps, a committed redaction is done and comes back with no error,
// unless something after the commit fails: an old blob or the one it put
// won't delete, or the WAL won't empty. A refused one's error says another
// writer archived the task instead of that it is left unarchived.
func (s *Store) RedactArchived(stream, secret string, who events.Actor, why string, summarise func() any, blobs ArchiveBlobs) (events.Event, *events.Rejection, error) {
	return s.redactArchived(stream, secret, nil, who, why, summarise, blobs)
}

// RedactArchivedRows is RedactArchived of only the rows at positions, as
// RedactRows is Redact of them. Archiving keeps each row's position, in
// the blob and back in the log once unarchived, so the positions read
// through the blob name the same rows the redaction rewrites.
func (s *Store) RedactArchivedRows(stream, secret string, positions []int64, who events.Actor, why string, summarise func() any, blobs ArchiveBlobs) (events.Event, *events.Rejection, error) {
	return s.redactArchived(stream, secret, onlyAt(positions), who, why, summarise, blobs)
}

// redactArchived is RedactArchived of the rows named.
func (s *Store) redactArchived(stream, secret string, named rowsNamed, who events.Actor, why string, summarise func() any, blobs ArchiveBlobs) (events.Event, *events.Rejection, error) {
	kept, live, unarchived, rejection, err := s.archivedRows(stream, blobs)
	if rejection != nil || err != nil {
		return events.Event{}, rejection, err
	}
	if err := removeRedacted(kept, live, blobs); err != nil {
		return events.Event{}, nil, fmt.Errorf("redacting %s: %w", stream, err)
	}
	if _, rejection, err := s.checkArchived(stream, append(append(kept, live...), unarchived), secret, named, who, why); rejection != nil || err != nil {
		return events.Event{}, rejection, err
	}
	if _, rejection, err := s.Unarchive(stream, blobs); rejection != nil || err != nil {
		return events.Event{}, rejection, err
	}
	redaction, archives, rejection, err := s.redactUnarchived(stream, secret, named, who, why)
	if redaction.Position == 0 { // nothing committed: the old blobs still hold what the rows do
		return events.Event{}, rejection, errors.Join(err, s.archiveAgain(stream, summarise, blobs))
	}
	again := s.archiveAgain(stream, summarise, blobs)
	var byAnother *archivedByAnother
	if errors.As(again, &byAnother) { // the goal: redacted and archived
		again = byAnother.dropped
	}
	failed := errors.Join(err, again)
	for _, hash := range archives {
		if err := blobs.Remove(hash); err != nil {
			failed = errors.Join(failed, fmt.Errorf("removing the blob %s: %w", hash, err))
		}
	}
	if failed != nil {
		return redaction, nil, fmt.Errorf("%s is redacted, but not all of it is done: %w", stream, failed)
	}
	return redaction, nil, nil
}

// archivedRows is an archived stream's rows, kept in its marker's blob and
// live, and the task.unarchived row the unarchiving will add, or why it
// can't be redacted: it is erased, so nothing is left, or it isn't
// archived, and Redact redacts it as it is.
func (s *Store) archivedRows(stream string, blobs Opener) (kept, live []row, unarchived row, rejection *events.Rejection, err error) {
	logged, err := s.Log(stream)
	if err != nil {
		return nil, nil, row{}, nil, err
	}
	if erased(logged) {
		return nil, nil, row{}, erasedRedaction(stream), nil
	}
	marker, archived := MarkerOf(logged)
	if !archived {
		return nil, nil, row{}, &events.Rejection{Check: "redact", Reason: stream + " isn't archived: it is redacted as it is"}, nil
	}
	if kept, err = keptBy(marker, blobs); err != nil {
		return nil, nil, row{}, nil, unreadable(stream, err)
	}
	through := s.applied.Load()
	if live, err = rowsOf(s.reads, stream, through); err != nil {
		return nil, nil, row{}, nil, err
	}
	unarchived, err = unarchivedRow(marker, through)
	return kept, live, unarchived, nil, err
}

// unreadable is the error of an archived stream whose blob can't be read,
// as a preview or a redaction answers it: it names the task, but not the
// blob's path on the server, which goes to the server's log.
func unreadable(stream string, err error) error {
	slog.Warn("store: an archived task's rows can't be read", "task", stream, "err", err)
	if errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("%s's archived rows can't be read: its blob is missing", stream)
	}
	return fmt.Errorf("%s's archived rows can't be read: the server's log says why", stream)
}

// unarchivedRow is the task.unarchived row Unarchive writes for marker, at
// the position after through, which it takes when nothing is appended
// first. The redaction rewrites that row too, as it holds the marker's
// position and blob, so the checks before the unarchiving see it.
func unarchivedRow(marker events.Event, through int64) (row, error) {
	unarchived := unarchivedBy(marker)
	data, err := json.Marshal(unarchived.Data)
	if err != nil {
		return row{}, fmt.Errorf("encoding the data of %s: %w", unarchived.Type, err)
	}
	return row{Position: through + 1, Stream: unarchived.Stream, ActorKind: string(unarchived.Actor.Kind),
		ActorID: unarchived.Actor.ID, Type: string(unarchived.Type), Data: string(data)}, nil
}

// checkArchived is what Redact would find in an archived stream's rows
// named, or what it would refuse in them, before anything is written.
func (s *Store) checkArchived(stream string, rows []row, secret string, named rowsNamed, who events.Actor, why string) (pending, *events.Rejection, error) {
	found, rejection, err := findIn(stream, rows, secret, named, who, why)
	if rejection != nil || err != nil {
		return pending{}, rejection, err
	}
	return found, s.rewritable(stream, found.rows), nil
}

// PreviewArchived is Preview for RedactArchived: the number of an archived
// task's rows, read through its marker's blob and the live log, with the
// task.unarchived the unarchiving adds, that it would rewrite or, when it
// would refuse them before unarchiving the task, its rejection and none.
// It writes nothing: not even the blobs an earlier redaction couldn't
// delete, which RedactArchived deletes first. The rules see the redaction
// only once the task is unarchived, so they aren't asked.
func (s *Store) PreviewArchived(stream, secret string, blobs Opener) (int, *events.Rejection, error) {
	kept, live, unarchived, rejection, err := s.archivedRows(stream, blobs)
	if rejection != nil || err != nil {
		return 0, rejection, err
	}
	found, rejection, err := s.checkArchived(stream, append(append(kept, live...), unarchived), secret, nil, redactor, "")
	if rejection != nil || err != nil {
		return 0, rejection, err
	}
	return len(found.rows), nil, nil
}

// removeRedacted deletes the blobs an event.redacted among an archived
// stream's rows names in "archives", unless a live marker names it: the
// redaction deleted them once it committed, and one that wouldn't delete
// then still holds the secret, which no later run would find in the rows.
// A blob already gone is no error.
func removeRedacted(kept, live []row, blobs Remover) error {
	named, err := archivesIn(live)
	if err != nil {
		return err
	}
	var failed error
	for _, r := range append(slices.Clip(kept), live...) {
		if r.Type != string(events.EventRedacted) {
			continue
		}
		var redaction struct {
			Archives []string `json:"archives"`
		}
		if err := json.Unmarshal([]byte(r.Data), &redaction); err != nil {
			return fmt.Errorf("the redaction at %d: %w", r.Position, err)
		}
		for _, hash := range redaction.Archives {
			// A safety net: blobs are named by their content, so none made
			// after a redaction can be one it named.
			if slices.Contains(named, hash) {
				continue
			}
			if err := blobs.Remove(hash); err != nil {
				failed = errors.Join(failed, fmt.Errorf("removing the blob %s an earlier redaction deleted: %w", hash, err))
			}
		}
	}
	return failed
}

// redactUnarchived is Redact of the rows named of a stream just
// unarchived, its redaction naming the blobs every task.archived among its
// rows names, which are returned to delete once it commits.
func (s *Store) redactUnarchived(stream, secret string, named rowsNamed, who events.Actor, why string) (events.Event, []string, *events.Rejection, error) {
	rows, err := rowsOf(s.reads, stream, s.applied.Load())
	if err != nil {
		return events.Event{}, nil, nil, err
	}
	archives, err := archivesIn(rows)
	if err != nil {
		return events.Event{}, nil, nil, fmt.Errorf("redacting %s: %w", stream, err)
	}
	found, rejection, err := findIn(stream, rows, secret, named, who, why)
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

// archiveAgain archives a stream RedactArchived unarchived. When it can't,
// it removes the blob its Archive put, if any, unless a marker names it
// (dropUnnamed), and says the stream is left unarchived or, if its latest
// marker is a task.archived, that another writer archived it again
// (archivedByAnother).
func (s *Store) archiveAgain(stream string, summarise func() any, blobs ArchiveBlobs) error {
	puts := &recordingPuts{Blobs: blobs}
	_, rejection, err := s.Archive(stream, summarise, puts)
	if rejection == nil && err == nil {
		return nil
	}
	if rejection != nil {
		err = errors.New("archiving it again was refused: " + rejection.Reason)
	}
	dropped := s.dropUnnamed(stream, puts.put, blobs)
	live, read := s.Log(stream)
	if _, archived := MarkerOf(live); archived && read == nil {
		return &archivedByAnother{stream: stream, refused: err, dropped: dropped}
	}
	return errors.Join(fmt.Errorf("%s is left unarchived: %w", stream, err), read, dropped)
}

// archivedByAnother is archiveAgain refused because another writer
// archived the stream again: refused is why, and dropped is removing the
// blob it put failing, if it did.
type archivedByAnother struct {
	stream           string
	refused, dropped error
}

func (a *archivedByAnother) Error() string {
	return errors.Join(fmt.Errorf("%s was archived again by another writer: %w", a.stream, a.refused), a.dropped).Error()
}

func (a *archivedByAnother) Unwrap() []error {
	return []error{a.refused, a.dropped}
}

// recordingPuts is blobs that keep the hash of the last blob put.
type recordingPuts struct {
	Blobs
	put string
}

func (r *recordingPuts) Put(blob io.Reader) (string, int64, error) {
	hash, size, err := r.Blobs.Put(blob)
	if err == nil {
		r.put = hash
	}
	return hash, size, err
}

// dropUnnamed removes the blob hash a refused Archive of stream put, unless
// a marker among the stream's rows names it: another writer's Archive of
// the same rows puts the same blob. Archive puts before taking the lock,
// so nothing else would delete it, and it holds the rows it read, the
// secret too when the redaction was refused. The check and the removal
// hold the lock, so no marker naming the blob commits between them. hash
// is empty when nothing was put.
func (s *Store) dropUnnamed(stream, hash string, blobs Remover) error {
	if hash == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := rowsOf(s.reads, stream, s.applied.Load())
	if err != nil {
		return err
	}
	named, err := archivesIn(rows)
	if err != nil {
		return fmt.Errorf("removing the blob archiving %s again put: %w", stream, err)
	}
	if slices.Contains(named, hash) {
		return nil
	}
	if err := blobs.Remove(hash); err != nil {
		return fmt.Errorf("removing the blob %s archiving %s again put: %w", hash, stream, err)
	}
	return nil
}
