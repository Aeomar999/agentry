package current

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks/taskstest"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/settings"
)

// erasedLog is archivedLog with every task erased, by a marker appended
// as Store.Erase appends it: task-1 archived, task-2 whole and task-3
// broken. With rows, each marker's rows are deleted too, as Store.Erase
// deletes them, so each marker starts its stream; without, they stay
// beside it.
func erasedLog(t *testing.T, rows bool) string {
	t.Helper()
	path := archivedLog(t)
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		own := streamAt(t, path, id)
		marker := taskstest.Erasure(own)
		appendRow(t, path, marker)
		if rows {
			deleteRows(t, path, id, int64(marker.Number("through")))
		}
	}
	return path
}

// deleteRows deletes a stream's rows up to through from the closed log at
// path, past the delete trigger, which a test's marker appended by hand
// gets no say in.
func deleteRows(t *testing.T, path, stream string, through int64) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`DROP TRIGGER events_no_delete; DELETE FROM events WHERE stream = ? AND position <= ?`, stream, through); err != nil {
		t.Fatal(err)
	}
}

// An erased task is a stub, whether its rows are gone or still beside its
// marker, and whatever it was: whole, archived or broken. The state equals
// a replay, as does a snapshot at every position; the task can't be had,
// isn't listed, keeps its place in the order and its id, and says when it
// went. With its rows beside it, a snapshot from before the first marker
// takes task-3's break in its tail, which replays the whole log (Resume),
// so the cuts start at that marker: the tail then erases task-3 broken.
func TestAnErasedTaskIsAStub(t *testing.T) {
	for _, c := range []struct {
		name string
		rows bool
	}{{"its rows deleted", true}, {"its rows beside it", false}} {
		t.Run(c.name, func(t *testing.T) {
			logPath := erasedLog(t, c.rows)
			state := New(withIdeas, ideas.Streams())
			s, err := store.Open(logPath, withIdeas, state, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := Equal(s, state); err != nil {
				t.Error(err)
			}
			for _, id := range []string{"task-1", "task-2", "task-3"} {
				if task, err := state.Task(id); task != nil || !errors.Is(err, ErrErased) {
					t.Errorf("%s: %v, %v, want ErrErased", id, task, err)
				}
				own, err := s.Stream(id)
				if err != nil {
					t.Fatal(err)
				}
				if erasure, erased := state.Erased(id); !erased || erasure.ID != id || !erasure.At.Equal(own[len(own)-1].At) {
					t.Errorf("%s's erasure: %+v, %v, want its marker's time", id, erasure, erased)
				}
			}
			if ids := state.TaskIDs(); !slices.Equal(ids, []string{"task-1", "task-2", "task-3"}) {
				t.Errorf("the task ids are %v, want every erased task's still there", ids)
			}
			if next := state.NextTask(); next != "task-4" {
				t.Errorf("the next task is %s, want task-4", next)
			}
			if listed, summaries := state.Tasks(), state.Summaries(); len(listed) != 0 || len(summaries) != 0 {
				t.Errorf("an erased task is listed: %v, %v", listed, summaries)
			}
			all, err := everyEvent(s)
			if err != nil {
				t.Fatal(err)
			}
			s.Close()

			var cuts []int64
			for _, e := range all {
				if c.rows || e.Position >= firstMarker(all) {
					cuts = append(cuts, e.Position)
				}
			}
			if err := EqualThroughSnapshots(logPath, withIdeas, t.TempDir(), cuts, ideas.Streams()); err != nil {
				t.Error(err)
			}
			restored := Restore(snapshotted(t, logPath, cuts[len(cuts)-1], nil), "build-1", withIdeas, ideas.Streams())
			if restored.snapshot == nil {
				t.Fatal("the snapshot wasn't restored")
			}
			if !reflect.DeepEqual(restored.erased, state.erased) {
				t.Errorf("restored erasures %+v, want %+v", restored.erased, state.erased)
			}
		})
	}
}

// firstMarker is the position of the first task.erased in log.
func firstMarker(log []events.Event) int64 {
	for _, e := range log {
		if e.Type == events.TaskErased {
			return e.Position
		}
	}
	return 0
}

// beforeErasures is a snapshot's first chunk before the state kept
// erasures: savedState without Erased.
type beforeErasures struct {
	Position    int64
	Streams     map[string]int64
	Created     []savedPlace
	TasksMade   int
	OwnedOrder  map[string][]savedPlace
	Broken      []string
	Archived    map[string]tasks.Summary
	Roster      agents.Saved
	Workers     workers.Saved
	Connections connections.Saved
	Settings    settings.Settings
	Prefixes    map[string]Unknown
	Types       map[events.Type]Unknown
}

// A snapshot from before the state kept erasures is never read as one
// with them, as TestASnapshotFromBeforeSummariesReplays checks for
// summaries: older code's is another build's, and the old shape under
// this build's tag fails to decode. Either way the whole log replays, the
// erasures included.
func TestASnapshotFromBeforeErasuresReplays(t *testing.T) {
	logPath := erasedLog(t, true)
	log, err := store.Open(logPath, withIdeas, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	last := log.LastPosition()
	anchor, _, err := log.Hash(last)
	log.Close()
	if err != nil {
		t.Fatal(err)
	}
	full := New(withIdeas, ideas.Streams())
	loaded, err := store.Open(logPath, withIdeas, full, nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Close()
	old := full.saved()
	oldShape := func(t *testing.T, path string) string {
		var body bytes.Buffer
		if err := writeChunk(&body, beforeErasures{Position: old.Position, Streams: old.Streams, Created: old.Created, TasksMade: old.TasksMade,
			OwnedOrder: old.OwnedOrder, Broken: old.Broken, Archived: old.Archived, Roster: old.Roster, Workers: old.Workers,
			Connections: old.Connections, Settings: old.Settings, Prefixes: old.Prefixes, Types: old.Types}); err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(body.Bytes())
		line, err := json.Marshal(header{Format: snapshotFormat, Build: "build-1", Position: last, Anchor: anchor,
			Size: int64(body.Len()), SHA256: hex.EncodeToString(sum[:])})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(append(line, '\n'), body.Bytes()...), 0o600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	for _, c := range []struct {
		name   string
		damage func(t *testing.T, path string) string
		build  string
		want   string
	}{
		{"older code", nil, "build-2", "from other code"},
		{"the old shape under this build", oldShape, "build-1", "torn or corrupt"},
	} {
		t.Run(c.name, func(t *testing.T) {
			path := snapshotted(t, logPath, last, nil)
			if c.damage != nil {
				path = c.damage(t, path)
			}
			state, logged := opened(t, logPath, path, c.build)
			if state.resumed != 0 || !strings.Contains(logged, c.want) {
				t.Errorf("went on from %d, logged %q, want %q in it", state.resumed, logged, c.want)
			}
			if _, erased := state.Erased("task-2"); !erased {
				t.Error("after the replay task-2 isn't erased")
			}
		})
	}
}
