package current

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// A start with a snapshot goes:
//
//	RemoveLeftovers(path)
//	state := Restore(path, build, catalogue, owned...)
//	log, err := store.Open(dbPath, catalogue, state, check) // calls state.Resume
//
// and a snapshot is written with WriteSnapshot(path, state, log, build).
// Every way a snapshot can't be used ends in a whole replay, which is
// logged with why.

// Restore is a state read from the snapshot at path, for store.Open to
// resume from (Resume). build names the running code: a snapshot written by
// other code is not used. When there is no snapshot it can use, Restore logs
// why and returns a new state, which Open loads whole; a torn or corrupt
// file is kept as path.bad, the last one only, for a person to look at.
func Restore(path, build string, catalogue *events.Catalogue, owned ...*features.Streams) *State {
	s := New(catalogue, owned...)
	err := s.readFile(path, build)
	if err == nil {
		return s
	}
	s.empty()
	switch {
	case errors.Is(err, fs.ErrNotExist):
		slog.Info("current: there is no snapshot, so the whole log is replayed", "path", path)
	case errors.Is(err, errOtherCode):
		slog.Info("current: the snapshot is from other code, so the whole log is replayed", "path", path, "why", err)
	default:
		slog.Warn("current: the snapshot is torn or corrupt, so the whole log is replayed", "path", path, "why", err, "kept", path+".bad")
		if err := os.Rename(path, path+".bad"); err != nil {
			slog.Warn("current: the corrupt snapshot can't be kept", "path", path, "err", err)
		}
	}
	return s
}

func (s *State) readFile(path, build string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return s.readSnapshot(file, build)
}

// tail is what Resume learns as it applies the events after a snapshot.
type tail struct {
	broke error // the first stream the events broke
}

// Resume makes s a store.Resumer. A state Restore read from a snapshot is
// checked against the log: its anchor must be the log's event at its
// position. The streams the snapshot holds no state for, broken and owned
// ones, are rebuilt from their events up to that position, and the events
// after it are applied in order. If the snapshot doesn't fit the log, or
// those events break a stream, Resume logs why and empties the state, which
// Open then loads whole. A state not read from a snapshot is left as it is.
func (s *State) Resume(log store.Past) error {
	at := s.snapshot
	if at == nil {
		return nil
	}
	s.snapshot = nil
	why, err := s.resumeFrom(at, log)
	if err != nil {
		return err
	}
	if why != "" {
		slog.Warn("current: the snapshot doesn't fit the log, so the whole log is replayed", "position", at.position, "why", why)
		s.empty()
		return nil
	}
	s.resumed = at.position
	return nil
}

// resumeFrom brings a restored state up to the log's last position, or
// says why it can't.
func (s *State) resumeFrom(at *snapshotAt, log store.Past) (why string, err error) {
	if at.position > log.Last() {
		return fmt.Sprintf("it is at position %d, past the log's last, %d", at.position, log.Last()), nil
	}
	anchor, found, err := log.Hash(at.position)
	if err != nil {
		return "", err
	}
	if !found {
		return fmt.Sprintf("the log has no event at its position, %d", at.position), nil
	}
	if anchor != at.anchor {
		return fmt.Sprintf("the log's event at its position, %d, is not the one it was written at", at.position), nil
	}
	for _, stream := range s.rebuilt(at) {
		accepted, err := log.Stream(stream, at.position)
		if err != nil {
			return "", err
		}
		s.mu.Lock()
		s.fold(stream, accepted)
		s.mu.Unlock()
	}
	s.tail = &tail{}
	err = log.ApplyAfter(s, at.position)
	broke := s.tail.broke
	s.tail = nil
	if err != nil {
		return "", err
	}
	if broke != nil {
		return "an event after it broke a stream: " + broke.Error(), nil
	}
	s.Loaded(log.Last())
	return "", nil
}

// rebuilt is the streams a snapshot holds no state for: the broken ones,
// which break again with their reason, and every owned one, whose state is
// a feature's own and has no saved form.
func (s *State) rebuilt(at *snapshotAt) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	streams := slices.Clone(at.broken)
	for _, order := range s.ownedOrder {
		for _, c := range order {
			streams = append(streams, c.id)
		}
	}
	slices.Sort(streams)
	return slices.Compact(streams)
}

// WriteSnapshot writes the state, as it is now, to the snapshot at path,
// replacing it whole or not at all. log is the store the state is kept
// from, and build names the running code. A failed write leaves the old
// snapshot as it was.
func WriteSnapshot(path string, s *State, log *store.Store, build string) error {
	return writeAtomically(path, func(w io.Writer) error { return writeSnapshot(w, s, log, build) })
}

// leftovers matches the temporary files of writes to path that never
// finished.
func leftovers(path string) string { return path + ".tmp-*" }

// writeAtomically writes a file with write, so that path is either the old
// file or the whole new one, even after a power cut: a temporary file
// beside it, synced, renamed over it, and the folder synced so the rename
// lasts too. The file is readable by its owner alone, as it holds what the
// log does.
func writeAtomically(path string, write func(io.Writer) error) (err error) {
	dir, pattern := filepath.Split(leftovers(path))
	temporary, err := os.CreateTemp(dir, pattern) // mode 0600
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			temporary.Close()
			os.Remove(temporary.Name())
		}
	}()
	if err := write(temporary); err != nil {
		return err
	}
	if err := temporary.Sync(); err != nil {
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary.Name(), path); err != nil {
		return err
	}
	return syncFolder(filepath.Dir(path))
}

// syncFolder makes a rename in a folder last through a power cut.
func syncFolder(dir string) error {
	folder, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer folder.Close()
	return folder.Sync()
}

// RemoveLeftovers deletes the temporary files of snapshot writes to path
// that never finished, as after a crash mid-write. Nothing else is touched.
func RemoveLeftovers(path string) error {
	found, err := filepath.Glob(leftovers(path))
	if err != nil {
		return err
	}
	var errs []error
	for _, file := range found {
		errs = append(errs, os.Remove(file))
	}
	return errors.Join(errs...)
}
