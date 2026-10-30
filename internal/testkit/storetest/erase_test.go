package storetest

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"modernc.org/sqlite"
)

// Erasing, as the state and the files on disk see it
// (docs/design/log-growth.md section 5, "Removing a whole task"). The
// store's side, the trigger and the marker, is in store's erase_test.go.

func erase(t testing.TB, s *store.Store, id string, blobs store.Remover) events.Event {
	t.Helper()
	event, rejection, err := s.Erase(id, you, "Asked to be forgotten.", blobs)
	if rejection != nil || err != nil {
		t.Fatalf("erasing %s: %v, %v", id, rejection, err)
	}
	return event
}

// planted is in the erased tasks' words, and nowhere else.
const planted = "planted-7f3a9c2e"

// Test 1: once a task is erased, a text planted in it is in no byte of
// the log, its WAL, any blob or a snapshot written after. Two tasks hold
// it: task-1 whole, with a direction long enough to need overflow pages,
// and task-3 archived, so in a blob too. task-2's rows sit among theirs on
// the same pages, so the deleted rows' bytes stay on them unless
// secure_delete zeroes them; and it is still found, so the search reads
// the live rows.
func TestAnErasedTaskIsInNoByteOnDisk(t *testing.T) {
	dir := t.TempDir()
	s, state := Open(t, filepath.Join(dir, "agentry.db"))
	blobs := sharedfiles.Disk{Dir: filepath.Join(dir, "files")}
	const kept = "kept-4b1d"
	for _, p := range append(agents.SeedProposals(nil),
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "One " + planted, "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Two " + kept, "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskDirected, Data: map[string]any{"text": strings.Repeat("Mind the "+planted+". ", 400)}},
		events.Proposal{Stream: "task-3", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Three " + planted, "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Two, renamed " + kept}},
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "One, renamed " + planted}},
	) {
		mustPropose(t, s, p)
	}
	ArchiveIn(t, s, state, "task-3", blobs)
	if holding := FilesHolding(t, dir, planted); len(holding) < 2 {
		t.Fatalf("before erasing, %q is in %v: want the log and the blob", planted, holding)
	}

	erase(t, s, "task-1", blobs)
	erase(t, s, "task-3", blobs)
	if err := current.WriteSnapshot(filepath.Join(dir, "state.snapshot"), state, s, "build-1"); err != nil {
		t.Fatal(err)
	}
	if holding := FilesHolding(t, dir, planted); len(holding) != 0 {
		t.Errorf("after erasing, %q is still in %v", planted, holding)
	}
	if holding := FilesHolding(t, dir, kept); len(holding) == 0 {
		t.Errorf("task-2's %q is in no file: the search can't see the live rows", kept)
	}
}

// Test 2: an erased task takes no event, ever: not through Propose, not
// an unarchiving, an archive or another erasure. Each says why.
func TestAnErasedTaskTakesNoEvent(t *testing.T) {
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	twoTasks(t, s)
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	erase(t, s, "task-1", blobs)
	refused := rules.ErasedRefusal("task-1")
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Back"}},
		{Stream: "task-1", Actor: you, Type: events.TaskReopened, Data: map[string]any{"text": "Once more."}},
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "One again", "repo": "/r", "coordinator": "tech-lead"}},
	} {
		if _, rejection, err := s.Propose(p); err != nil || rejection == nil || rejection.Reason != refused {
			t.Errorf("%s on an erased task: %v, %v, want %q", p.Type, rejection, err, refused)
		}
	}
	if _, rejection, err := s.Unarchive("task-1", blobs); err != nil || rejection == nil {
		t.Errorf("unarchiving an erased task: %v, %v, want refused", rejection, err)
	}
	if _, rejection, err := s.Archive("task-1", summarise(state, "task-1"), blobs); err != nil || rejection == nil || rejection.Reason != refused {
		t.Errorf("archiving an erased task: %v, %v, want %q", rejection, err, refused)
	}
	if _, rejection, err := s.Erase("task-1", you, "Again.", blobs); err != nil || rejection == nil || rejection.Reason != refused {
		t.Errorf("erasing an erased task: %v, %v, want %q", rejection, err, refused)
	}
	if task, err := state.Task("task-1"); task != nil || !errors.Is(err, current.ErrErased) {
		t.Errorf("task-1 in the state: %v, %v, want ErrErased", task, err)
	}
}

// Test 3: erasing an archived task deletes the blob that held its rows,
// and the state keeps it as the stub, not the summary.
func TestErasingAnArchivedTaskDeletesItsBlob(t *testing.T) {
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	twoTasks(t, s)
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	ArchiveIn(t, s, state, "task-1", blobs)
	live, err := s.Log("task-1")
	if err != nil || len(live) != 1 {
		t.Fatalf("task-1 should be its marker alone: %v, %v", live, err)
	}
	hash := live[0].Text("archive")
	if !blobs.Has(hash) {
		t.Fatal("the archive's blob isn't there")
	}
	erased := erase(t, s, "task-1", blobs)
	if blobs.Has(hash) {
		t.Error("the archive's blob is still there")
	}
	if names := erased.List("archives"); len(names) != 1 || names[0] != hash {
		t.Errorf("the marker names %v, want the archive's blob", names)
	}
	if _, archived := state.Summary("task-1"); archived {
		t.Error("task-1 still has its summary")
	}
	if erasure, erased := state.Erased("task-1"); !erased || erasure.ID != "task-1" {
		t.Errorf("task-1's erasure: %+v, %v", erasure, erased)
	}
}

// Test 4: a restart replays the log after an erasure to the state the
// store kept live, stub and all: a whole task and an archived one erased.
// (Open's own cleanup checks the same of a replay and of a snapshot at
// every position.)
func TestAnErasureReplaysAsItWasLive(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, live := Open(t, path)
	twoTasks(t, s)
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	ArchiveIn(t, s, live, "task-2", blobs)
	erase(t, s, "task-1", blobs)
	erase(t, s, "task-2", blobs)

	_, restarted := Open(t, path)
	for _, id := range []string{"task-1", "task-2"} {
		before, _ := live.Erased(id)
		after, erased := restarted.Erased(id)
		if !erased || !reflect.DeepEqual(after, before) {
			t.Errorf("%s after a restart: %+v, %v, want %+v", id, after, erased, before)
		}
	}
	if ids, again := live.TaskIDs(), restarted.TaskIDs(); !reflect.DeepEqual(ids, again) || len(ids) != 2 {
		t.Errorf("the task ids live %v, after a restart %v", ids, again)
	}
	if next, again := live.NextTask(), restarted.NextTask(); next != "task-3" || again != next {
		t.Errorf("the next task live %s, after a restart %s, want task-3", next, again)
	}
}

// Test 5: an erased task's id is never taken again, before a restart or
// after: the next task follows it, though its task.created is gone.
func TestAnErasedTasksIDIsNeverTakenAgain(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, state := Open(t, path)
	twoTasks(t, s)
	erase(t, s, "task-2", sharedfiles.Disk{Dir: t.TempDir()})
	newTask := events.Proposal{Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Next", "repo": "/r", "coordinator": "tech-lead"}}
	if made, rejection, err := s.Create(events.TaskPrefix, newTask); err != nil || rejection != nil || made.Stream != "task-3" {
		t.Errorf("Create after erasing task-2: %s, %v, %v, want task-3", made.Stream, rejection, err)
	}
	if next := state.NextTask(); next != "task-4" {
		t.Errorf("the state names %s next, want task-4", next)
	}
	// The first store stays open, so the restart only reads: a write
	// through it would leave the first state behind the log.
	_, restarted := Open(t, path)
	if next := restarted.NextTask(); next != "task-4" {
		t.Errorf("the restarted state names %s next, want task-4", next)
	}
}

// archivedAndKept seeds the roster, task-1 with planted in its title and
// task-2 with kept in its, both on the same pages as the agents' rows.
func archivedAndKept(t *testing.T, s *store.Store, kept string) {
	t.Helper()
	for _, p := range append(agents.SeedProposals(nil),
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "One " + planted, "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Two " + kept, "repo": "/r", "coordinator": "tech-lead"}},
	) {
		mustPropose(t, s, p)
	}
}

// checkErased fails the test if planted is in any file under dir, or kept
// in none, so the search reads the live rows.
func checkErased(t *testing.T, dir, kept string) {
	t.Helper()
	if holding := FilesHolding(t, dir, planted); len(holding) != 0 {
		t.Errorf("after erasing, %q is still in %v", planted, holding)
	}
	if holding := FilesHolding(t, dir, kept); len(holding) == 0 {
		t.Errorf("task-2's %q is in no file: the search can't see the live rows", kept)
	}
}

// Test 6: a task archived and then erased is in no byte on disk. The
// archive deleted its rows before the erasure, from pages that task-2's
// and the agents' rows keep in use, so only secure_delete on for the
// archive too zeroes them: the erasure zeroes only the marker.
func TestAnArchivedTaskErasedIsInNoByteOnDisk(t *testing.T) {
	dir := t.TempDir()
	s, state := Open(t, filepath.Join(dir, "agentry.db"))
	blobs := sharedfiles.Disk{Dir: filepath.Join(dir, "files")}
	const kept = "kept-5c2e"
	archivedAndKept(t, s, kept)
	ArchiveIn(t, s, state, "task-1", blobs)
	erase(t, s, "task-1", blobs)
	checkErased(t, dir, kept)
}

// Test 7: the same once the task has been archived, unarchived, reopened
// and archived again: each archive and the unarchiving deleted rows.
func TestATaskArchivedTwiceErasedIsInNoByteOnDisk(t *testing.T) {
	dir := t.TempDir()
	s, state := Open(t, filepath.Join(dir, "agentry.db"))
	blobs := sharedfiles.Disk{Dir: filepath.Join(dir, "files")}
	const kept = "kept-8a61"
	archivedAndKept(t, s, kept)
	ArchiveIn(t, s, state, "task-1", blobs)
	unarchive(t, s, "task-1", blobs)
	reopen(t, s, "task-1")
	ArchiveIn(t, s, state, "task-1", blobs)
	erase(t, s, "task-1", blobs)
	checkErased(t, dir, kept)
}

// Test 8: a folder whose task was archived by a build before secure_delete,
// at schema version 1, opened by this one: the full VACUUM before the
// migration clears the cells the archive freed, so the erasure leaves
// nothing behind.
func TestAnErasureClearsWhatAnOlderArchiveLeft(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentry.db")
	blobs := sharedfiles.Disk{Dir: filepath.Join(dir, "files")}
	const kept = "kept-e90d"
	withoutSecureDelete(t, func() {
		s, state := openUnchecked(t, path, events.Core())
		archivedAndKept(t, s, kept)
		ArchiveIn(t, s, state, "task-1", blobs)
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	})
	if holding := FilesHolding(t, dir, planted); !slices.Contains(holding, "agentry.db") {
		t.Fatalf("before the upgrade, %q is in %v: want the log, in the archive's freed cells", planted, holding)
	}
	atVersionOne(t, path)

	s, _ := Open(t, path)
	erase(t, s, "task-1", blobs)
	checkErased(t, dir, kept)
}

// withoutSecureDelete runs body with the stores it opens opening as the
// builds before secure_delete did: without it.
func withoutSecureDelete(t *testing.T, body func()) {
	t.Helper()
	name := fmt.Sprintf("sqlite-insecure-%d", drivers.Add(1))
	sql.Register(name, insecureDriver{})
	previous := store.Driver
	store.Driver = name
	defer func() { store.Driver = previous }()
	body()
}

type insecureDriver struct{}

func (insecureDriver) Open(name string) (driver.Conn, error) {
	return (&sqlite.Driver{}).Open(strings.Replace(name, "&_pragma=secure_delete(ON)", "", 1))
}

// atVersionOne takes the file at path back to schema version 1, before
// erasing: migration 2's and 3's indexes dropped, so they run again. Their
// triggers stay, which they replace anyway.
func atVersionOne(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open(store.Driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP INDEX events_erased; DROP INDEX events_redacted; PRAGMA user_version = 1`); err != nil {
		t.Fatal(err)
	}
}

// Test 9: the full VACUUM before the migration runs once. A folder made at
// schema version 2, with a task archived, reopens without one, so a large
// log doesn't copy itself on every start; taken back to version 1, it
// gets exactly one, and the reopen after it none.
func TestReopeningAVersionTwoLogRunsNoVacuum(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentry.db")
	blobs := sharedfiles.Disk{Dir: filepath.Join(dir, "files")}
	s, state := openUnchecked(t, path, events.Core())
	archivedAndKept(t, s, "kept-3f07")
	ArchiveIn(t, s, state, "task-1", blobs)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	if vacuums := vacuumsOpening(t, path); vacuums != 0 {
		t.Errorf("reopening a version-2 log sent %d VACUUMs, want none", vacuums)
	}
	atVersionOne(t, path)
	if vacuums := vacuumsOpening(t, path); vacuums != 1 {
		t.Errorf("opening a version-1 log with an archived task sent %d VACUUMs, want 1", vacuums)
	}
	if vacuums := vacuumsOpening(t, path); vacuums != 0 {
		t.Errorf("reopening it after its VACUUM sent %d more, want none", vacuums)
	}
}

// vacuumsOpening opens and closes the store at path, and says how many full
// VACUUMs it sent: PRAGMA incremental_vacuum isn't one.
func vacuumsOpening(t *testing.T, path string) int64 {
	t.Helper()
	var vacuums atomic.Int64
	name := fmt.Sprintf("sqlite-vacuums-%d", drivers.Add(1))
	sql.Register(name, vacuumCountingDriver{&vacuums})
	previous := store.Driver
	store.Driver = name
	defer func() { store.Driver = previous }()
	s, _ := openUnchecked(t, path, events.Core())
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	return vacuums.Load()
}

type vacuumCountingDriver struct{ vacuums *atomic.Int64 }

func (d vacuumCountingDriver) Open(name string) (driver.Conn, error) {
	c, err := (&sqlite.Driver{}).Open(name)
	if err != nil {
		return nil, err
	}
	return vacuumCountingConn{c.(sqliteConn), d.vacuums}, nil
}

type vacuumCountingConn struct {
	sqliteConn
	vacuums *atomic.Int64
}

func (c vacuumCountingConn) count(query string) {
	if words := strings.Fields(query); len(words) > 0 && strings.EqualFold(words[0], "VACUUM") {
		c.vacuums.Add(1)
	}
}

func (c vacuumCountingConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	c.count(query)
	return c.sqliteConn.PrepareContext(ctx, query)
}

func (c vacuumCountingConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	result, err := c.sqliteConn.ExecContext(ctx, query, args)
	if err != driver.ErrSkip { // skipped, database/sql prepares it instead
		c.count(query)
	}
	return result, err
}

func (c vacuumCountingConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	rows, err := c.sqliteConn.QueryContext(ctx, query, args)
	if err != driver.ErrSkip {
		c.count(query)
	}
	return rows, err
}
