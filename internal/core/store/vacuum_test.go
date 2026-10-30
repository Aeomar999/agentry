package store

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
)

// pragma reads one number from the log's file, such as its page_count.
func pragma(t *testing.T, s *Store, name string) int64 {
	t.Helper()
	var n int64
	if err := s.db.QueryRow(`PRAGMA ` + name).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// walSize is the size of the log's WAL file, 0 when it has none.
func walSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path + "-wal")
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

// bigTask gives task-1 a few hundred kilobytes of renames, so archiving it
// frees pages, and makes it done.
func bigTask(t *testing.T, s *Store) {
	t.Helper()
	acceptAll(t, s, newTask("task-1"))
	for range 100 {
		acceptAll(t, s, events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": strings.Repeat("a long title ", 300)}})
	}
}

// After an archive, GiveBackSpace shrinks the file by the pages it freed
// and empties the WAL, which held them too.
func TestGiveBackSpaceShrinksTheFileAndTheWAL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	bigTask(t, s)
	if _, rejection, err := s.Archive("task-1", summary, sharedfiles.Disk{Dir: t.TempDir()}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	pages, free, wal := pragma(t, s, "page_count"), pragma(t, s, "freelist_count"), walSize(t, path)
	if free < 50 || wal == 0 {
		t.Fatalf("archiving should leave free pages and a WAL: %d of %d pages free, a WAL of %d bytes", free, pages, wal)
	}
	if err := s.GiveBackSpace(); err != nil {
		t.Fatal(err)
	}
	if now, freeNow, walNow := pragma(t, s, "page_count"), pragma(t, s, "freelist_count"), walSize(t, path); freeNow != 0 || now != pages-free || walNow != 0 {
		t.Errorf("after giving back the space: %d pages, %d free, a WAL of %d bytes; want %d, 0 and 0", now, freeNow, walNow, pages-free)
	}
}

// oldFile is a log made before incremental mode: its file's auto_vacuum
// is NONE, which opening it with the pragma doesn't change.
func oldFile(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agentry.db")
	db, err := sql.Open(Driver, path+"?_pragma=journal_mode(WAL)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	return path
}

// An older file switches to incremental mode with one VACUUM, once a task
// is archived; never before, and never again, after a restart too.
func TestVacuumOnceSwitchesAnOldFileOnce(t *testing.T) {
	path := oldFile(t)
	s, _ := openAt(t, path)
	if mode := pragma(t, s, "auto_vacuum"); mode != 0 {
		t.Fatalf("the old file's auto_vacuum is %d, want 0 (NONE)", mode)
	}
	bigTask(t, s)
	if ran, err := s.VacuumOnce(); ran || err != nil {
		t.Errorf("before any archiving: ran %v, %v; want nothing done", ran, err)
	}
	if _, rejection, err := s.Archive("task-1", summary, sharedfiles.Disk{Dir: t.TempDir()}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	pages := pragma(t, s, "page_count")
	if ran, err := s.VacuumOnce(); !ran || err != nil {
		t.Fatalf("after the first archive: ran %v, %v; want the VACUUM run", ran, err)
	}
	if mode, now := pragma(t, s, "auto_vacuum"), pragma(t, s, "page_count"); mode != incremental || now >= pages {
		t.Errorf("after the VACUUM: auto_vacuum %d, %d pages; want %d and fewer than %d", mode, now, incremental, pages)
	}
	if ran, err := s.VacuumOnce(); ran || err != nil {
		t.Errorf("a second time: ran %v, %v; want nothing done", ran, err)
	}
	s.Close()

	s, _ = openAt(t, path)
	if ran, err := s.VacuumOnce(); ran || err != nil || pragma(t, s, "auto_vacuum") != incremental {
		t.Errorf("after a restart: ran %v, %v, auto_vacuum %d; want nothing done, and still incremental", ran, err, pragma(t, s, "auto_vacuum"))
	}
}

// A new file is incremental from the start, so it never needs the VACUUM.
func TestANewFileNeverNeedsTheVacuum(t *testing.T) {
	s := openTestStore(t)
	bigTask(t, s)
	if _, rejection, err := s.Archive("task-1", summary, sharedfiles.Disk{Dir: t.TempDir()}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if ran, err := s.VacuumOnce(); ran || err != nil || pragma(t, s, "auto_vacuum") != incremental {
		t.Errorf("ran %v, %v, auto_vacuum %d; want nothing done on an incremental file", ran, err, pragma(t, s, "auto_vacuum"))
	}
}
