package current

import (
	"bufio"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

const writerBuild = "build-1"

// live is a log opened as serve opens it, with a writer of snapshots
// beside it whose clock is ticks: a tick is the interval passing.
type live struct {
	logPath, path string
	state         *State
	log           *store.Store
	writer        *SnapshotWriter
	ticks         chan time.Time
	writes        atomic.Int64 // writes tried
}

// started restores the snapshot beside the log at logPath, opens the log
// with it and starts a writer that writes every `every` events. wrap, if
// set, wraps how the writer writes a file.
func started(t *testing.T, logPath string, every int64, wrap func(path string, write func(io.Writer) error) error) *live {
	t.Helper()
	l := &live{logPath: logPath, path: filepath.Join(filepath.Dir(logPath), "state.snapshot"), ticks: make(chan time.Time)}
	l.state = Restore(l.path, writerBuild, withIdeas, ideas.Streams())
	log, err := store.Open(logPath, withIdeas, l.state, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { log.Close() })
	l.log = log
	l.writer = NewSnapshotWriter(l.path, writerBuild, l.state, log)
	l.writer.every = every
	l.writer.after = func(time.Duration) <-chan time.Time { return l.ticks }
	if wrap == nil {
		wrap = writeAtomically
	}
	l.writer.atomically = func(path string, write func(io.Writer) error) error {
		l.writes.Add(1)
		return wrap(path, write)
	}
	l.writer.Start()
	return l
}

// tick passes the interval, and returns once the writer has done with it.
func (l *live) tick() {
	l.ticks <- time.Time{}
	l.ticks <- time.Time{} // taken only once the first has been
}

func (l *live) rename(t *testing.T, title string) {
	t.Helper()
	propose(t, l.log, proposal("task-1", lead, events.TaskRenamed, map[string]any{"title": title}))
}

// snapshotPosition is the position the snapshot at path says it is at, 0
// when there is none.
func snapshotPosition(t *testing.T, path string) int64 {
	t.Helper()
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	line, err := bufio.NewReader(file).ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var h header
	if err := json.Unmarshal(line, &h); err != nil {
		t.Fatal(err)
	}
	return h.Position
}

// awaitSnapshot waits for the snapshot at path to reach position.
func awaitSnapshot(t *testing.T, path string, position int64) {
	t.Helper()
	for deadline := time.Now().Add(10 * time.Second); snapshotPosition(t, path) != position; time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the snapshot is at %d, want %d", snapshotPosition(t, path), position)
		}
	}
}

// logged sends slog to a buffer until the test ends, and returns it.
func logged(t *testing.T) *syncBuffer {
	var buffer syncBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buffer, nil)))
	t.Cleanup(func() { slog.SetDefault(previous) })
	return &buffer
}

type syncBuffer struct {
	mu sync.Mutex
	b  strings.Builder
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.b.String()
}

// Target 3: a snapshot is written once `every` events have been applied
// since the last, or once the interval passes with at least one; not when
// it passes with none. The snapshot then loads equal to a replay.
func TestTheWriterWritesAfterEnoughEventsOrTimeWithAnEvent(t *testing.T) {
	logPath, _ := seededLog(t, someProposals()...)
	l := started(t, logPath, 3, nil)
	first := l.state.Position()
	awaitSnapshot(t, l.path, first) // a whole replay is written at once

	for i := range 3 {
		l.rename(t, "Renamed "+string(rune('a'+i)))
	}
	awaitSnapshot(t, l.path, first+3)
	if writes := l.writes.Load(); writes != 2 {
		t.Errorf("%d writes after a replay and 3 events, want 2: one at the start, one for the events", writes)
	}

	l.tick()
	if writes := l.writes.Load(); writes != 2 {
		t.Errorf("the interval passed with no event and the writer wrote: %d writes, want 2", writes)
	}

	l.rename(t, "Once more")
	l.tick()
	awaitSnapshot(t, l.path, first+4)
	l.writer.Close()
	if writes := l.writes.Load(); writes != 3 {
		t.Errorf("%d writes, want 3: Close has nothing new to write", writes)
	}

	state, said := opened(t, logPath, l.path, writerBuild)
	if state.resumed != first+4 || strings.Contains(said, "replayed") {
		t.Errorf("the start went on from %d, want %d; it logged:\n%s", state.resumed, first+4, said)
	}
}

// WriteSoon writes the state as it is now, with fewer events than call for
// a snapshot and the interval not passed, as after an archiving round.
func TestWriteSoonWritesTheStateAsItIs(t *testing.T) {
	logPath, _ := seededLog(t, someProposals()...)
	l := started(t, logPath, 1000, nil)
	first := l.state.Position()
	awaitSnapshot(t, l.path, first)
	l.rename(t, "Renamed")
	l.writer.WriteSoon()
	awaitSnapshot(t, l.path, first+1)
	if writes := l.writes.Load(); writes != 2 {
		t.Errorf("%d writes, want 2: one at the start, one asked for", writes)
	}
}

// Target 6: the writer holds no lock of the store's. A proposal commits,
// and the state takes it, while a write is held part way through.
func TestAProposalCommitsWhileTheWriterIsMidWrite(t *testing.T) {
	logPath, _ := seededLog(t, someProposals()...)
	writing, release := make(chan struct{}), make(chan struct{})
	l := started(t, logPath, 1, func(path string, write func(io.Writer) error) error {
		return writeAtomically(path, func(file io.Writer) error {
			return write(heldWriter{file, writing, release})
		})
	})
	select {
	case <-writing: // the write at the start, after the state was copied and encoded
	case <-time.After(10 * time.Second):
		t.Fatal("the writer wrote nothing at the start")
	}
	before := l.state.Position()
	committed := make(chan error, 1)
	go func() {
		_, _, err := l.log.Propose(proposal("task-1", lead, events.TaskRenamed, map[string]any{"title": "While writing"}))
		committed <- err
	}()
	select {
	case err := <-committed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the proposal waited for the writer")
	}
	if l.state.Position() != before+1 {
		t.Errorf("the state is at %d, want %d", l.state.Position(), before+1)
	}
	close(release)
	awaitSnapshot(t, l.path, before+1) // the event moved it past `every`
	l.writer.Close()
}

// heldWriter is a file whose writes wait for release, once writing has
// been told.
type heldWriter struct {
	io.Writer
	writing, release chan struct{}
}

func (w heldWriter) Write(p []byte) (int, error) {
	select {
	case w.writing <- struct{}{}:
	case <-w.release:
	}
	<-w.release
	return w.Writer.Write(p)
}

// A clean shutdown writes the state as it is, and the next start goes on
// from that snapshot with no fallback, equal to a replay.
func TestCloseWritesASnapshotTheNextStartGoesOnFrom(t *testing.T) {
	logPath, _ := seededLog(t, someProposals()...)
	l := started(t, logPath, 1_000, nil)
	awaitSnapshot(t, l.path, l.state.Position())
	l.rename(t, "Before shutdown")
	l.rename(t, "Just before shutdown")
	l.writer.Close()
	last := l.state.Position()
	if got := snapshotPosition(t, l.path); got != last {
		t.Fatalf("the snapshot after Close is at %d, want %d", got, last)
	}
	l.log.Close()

	state, said := opened(t, logPath, l.path, writerBuild)
	if state.resumed != last || strings.Contains(said, "replayed") {
		t.Errorf("the start went on from %d, want %d; it logged:\n%s", state.resumed, last, said)
	}
}

// A write that fails is logged and leaves the old snapshot; the writer
// goes on, and its next write lands.
func TestAFailedWriteIsLoggedAndTheWriterGoesOn(t *testing.T) {
	said := logged(t)
	logPath, _ := seededLog(t, someProposals()...)
	var failing atomic.Bool
	l := started(t, logPath, 1, func(path string, write func(io.Writer) error) error {
		if failing.Load() {
			return writeAtomically(path, func(io.Writer) error { return errors.New("the disk is full") })
		}
		return writeAtomically(path, write)
	})
	first := l.state.Position()
	awaitSnapshot(t, l.path, first)
	failing.Store(true)
	l.rename(t, "Not written")
	for deadline := time.Now().Add(10 * time.Second); !strings.Contains(said.String(), "the disk is full"); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("the failed write wasn't logged:\n%s", said)
		}
	}
	if got := snapshotPosition(t, l.path); got != first {
		t.Errorf("after a failed write the snapshot is at %d, want the old one's %d", got, first)
	}
	failing.Store(false)
	l.rename(t, "Written")
	awaitSnapshot(t, l.path, first+2)
	l.writer.Close()
}

// A start that had to replay the whole log, here for a corrupt snapshot,
// writes a fresh one straight away, with no event and no tick.
func TestAStartAfterAFallbackWritesAFreshSnapshot(t *testing.T) {
	logPath, _ := seededLog(t, someProposals()...)
	path := filepath.Join(filepath.Dir(logPath), "state.snapshot")
	if err := os.WriteFile(path, []byte("not a snapshot\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	l := started(t, logPath, 1_000, nil)
	if l.state.Resumed() != 0 {
		t.Fatal("the corrupt snapshot was used")
	}
	awaitSnapshot(t, l.path, l.state.Position())
	l.writer.Close()
	if _, err := os.Stat(path + ".bad"); err != nil {
		t.Errorf("the corrupt snapshot wasn't kept: %v", err)
	}
}

// A state Open went on from starts with its snapshot on disk: the writer
// writes nothing until there is something new.
func TestAResumedStartWritesNothingNew(t *testing.T) {
	logPath, _ := seededLog(t, someProposals()...)
	first := started(t, logPath, 1_000, nil)
	awaitSnapshot(t, first.path, first.state.Position())
	first.writer.Close()
	first.log.Close()

	again := started(t, logPath, 1_000, nil)
	if again.state.Resumed() == 0 {
		t.Fatal("the second start replayed the whole log")
	}
	again.tick()
	again.writer.Close()
	if writes := again.writes.Load(); writes != 0 {
		t.Errorf("%d writes with nothing new, want 0", writes)
	}
}
