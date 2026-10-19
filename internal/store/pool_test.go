package store

import (
	"database/sql/driver"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"modernc.org/sqlite"
)

// hold is what test_hold() runs; a trigger calls it to stop a write inside
// its transaction.
var hold func()

func init() {
	sqlite.MustRegisterScalarFunction("test_hold", 0, func(*sqlite.FunctionContext, []driver.Value) (driver.Value, error) {
		hold()
		return nil, nil
	})
}

// While a proposal holds the write connection inside its transaction, reads
// of the log go through the read pool: they return, and without the event
// being written. With one connection for both, they would wait for the
// commit.
func TestReadsDoNotQueueBehindAWrite(t *testing.T) {
	s, _ := openAt(t, filepath.Join(t.TempDir(), "agentry.db"))
	s.Propose(newTask("task-1"))
	started, release := make(chan struct{}), make(chan struct{})
	hold = func() {
		close(started)
		<-release
	}
	var released sync.Once
	t.Cleanup(func() { released.Do(func() { close(release) }) }) // before Close, which runs after
	if _, err := s.db.Exec(`CREATE TRIGGER hold_task_2 AFTER INSERT ON events WHEN NEW.stream = 'task-2'
		BEGIN SELECT test_hold(); END`); err != nil {
		t.Fatal(err)
	}
	proposed := make(chan error, 1)
	go func() {
		_, _, err := s.Propose(newTask("task-2"))
		proposed <- err
	}()
	within(t, started, "the write to start")

	reads := map[string]func() ([]events.Event, error){
		"Log":    func() ([]events.Event, error) { return s.Log("task-2") },
		"Stream": func() ([]events.Event, error) { return s.Stream("task-2") },
		"After":  func() ([]events.Event, error) { return s.After(1, 10) },
	}
	for name, read := range reads {
		done := make(chan error, 1)
		go func() {
			found, err := read()
			if err == nil && len(found) != 0 {
				t.Errorf("%s shows an event not yet committed: %+v", name, found)
			}
			done <- err
		}()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("%s: %v", name, err)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("%s waited for the write to finish", name)
		}
	}

	released.Do(func() { close(release) })
	if err := within(t, proposed, "the write to commit"); err != nil {
		t.Fatal(err)
	}
	if log, err := s.Log("task-2"); err != nil || len(log) != 1 {
		t.Errorf("once committed, Log should return the event: %+v, %v", log, err)
	}
}

// within is what c gives, failing the test if that takes over two seconds,
// so a write that never starts or ends fails instead of hanging the run.
func within[T any](t *testing.T, c <-chan T, what string) T {
	t.Helper()
	select {
	case v := <-c:
		return v
	case <-time.After(2 * time.Second):
		t.Fatalf("waited over two seconds for %s", what)
		var zero T
		return zero
	}
}
