package current

import (
	"io"
	"log/slog"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/store"
)

// How often a SnapshotWriter writes: the most a snapshot falls behind the
// log (docs/design/log-growth.md, section 2, "When it is written").
const (
	snapshotEvery    = 10_000           // events applied since the last snapshot
	snapshotInterval = 10 * time.Minute // with at least one event applied in it
)

// A SnapshotWriter keeps the snapshot at a path close behind a state, as the
// store applies events to it, so a start has only a short tail to apply. It
// writes on its own goroutine, never on the request path: it holds the
// state's read lock only to copy it, and no lock of the store's, so
// proposals go on while it encodes and writes. One write runs at a time, and
// a failed one is logged and leaves the old file.
type SnapshotWriter struct {
	path, build string
	state       *State
	log         *store.Store

	every      int64                                                // events applied that call for a snapshot
	interval   time.Duration                                        // time that calls for one, if an event was applied
	after      func(time.Duration) <-chan time.Time                 // the clock
	atomically func(path string, write func(io.Writer) error) error // how a file is written

	tried   int64 // the position of the last snapshot written or tried
	written int64 // the position of the snapshot on disk, 0 for none of this state's
	stop    chan struct{}
	done    chan struct{}
}

// NewSnapshotWriter is a writer of snapshots of state, which the store log
// keeps, to path, for the code named build. Start starts it. A state Open
// went on from a snapshot starts with that snapshot on disk; any other one
// is written as soon as the writer starts.
func NewSnapshotWriter(path, build string, state *State, log *store.Store) *SnapshotWriter {
	resumed := state.Resumed()
	return &SnapshotWriter{path: path, build: build, state: state, log: log,
		every: snapshotEvery, interval: snapshotInterval, after: time.After, atomically: writeAtomically,
		tried: resumed, written: resumed, stop: make(chan struct{}), done: make(chan struct{})}
}

// Start writes snapshots in the background until Close.
func (w *SnapshotWriter) Start() { go w.run() }

// Close stops the writer, waits for a write under way, and then writes the
// state as it is now, unless the snapshot on disk already holds it: the
// snapshot a clean shutdown leaves, which the next start goes on from.
func (w *SnapshotWriter) Close() {
	close(w.stop)
	<-w.done
	if w.state.Position() > w.written {
		w.write()
	}
}

func (w *SnapshotWriter) run() {
	defer close(w.done)
	if w.written == 0 {
		w.write() // after a whole replay, so the next start needn't do one
	}
	timer := w.after(w.interval)
	for {
		select {
		case <-w.stop:
			return
		case <-w.state.moved:
			if w.state.Position()-w.tried < w.every {
				continue
			}
		case <-timer:
			if w.state.Position() == w.tried {
				timer = w.after(w.interval)
				continue
			}
		}
		w.write()
		timer = w.after(w.interval)
	}
}

// write writes a snapshot of the state as it is now. A new state, which
// has no event yet, has nothing to write.
func (w *SnapshotWriter) write() {
	held := w.state.copy()
	if held.position == 0 {
		return
	}
	w.tried = held.position
	err := w.atomically(w.path, func(file io.Writer) error { return writeCopy(file, held, w.log, w.build) })
	if err != nil {
		slog.Warn("current: the snapshot can't be written, so the old one stays", "path", w.path, "position", held.position, "err", err)
		return
	}
	w.written = held.position
}
