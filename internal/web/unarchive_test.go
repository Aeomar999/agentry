package web

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// Reopening an archived task unarchives it first, in the same request
// (docs/design/log-growth.md section 4, "Reopening an archived task").

// openedThen is the server's blobs, with something done as a blob is
// opened: between Unarchive reading the marker and taking the lock.
type openedThen struct {
	sharedfiles.Disk
	then func()
}

func (o openedThen) Open(hash string) (io.ReadCloser, error) {
	o.then()
	return o.Disk.Open(hash)
}

// putThen is the server's blobs, with something done after an archive's
// blob is put: between Archive reading the rows and taking the lock.
type putThen struct {
	sharedfiles.Disk
	then func()
}

func (p putThen) Put(r io.Reader) (string, int64, error) {
	hash, size, err := p.Disk.Put(r)
	p.then()
	return hash, size, err
}

// lastTwo are the types of the stream's last two events.
func lastTwo(t *testing.T, f *archiveFixture, id string) []events.Type {
	t.Helper()
	log, err := f.log.Log(id)
	if err != nil || len(log) < 2 {
		t.Fatalf("%s's log: %v, %v", id, log, err)
	}
	return []events.Type{log[len(log)-2].Type, log[len(log)-1].Type}
}

func stillArchived(t *testing.T, f *archiveFixture, id string) {
	t.Helper()
	if _, err := f.state.Task(id); !errors.Is(err, current.ErrArchived) {
		t.Errorf("%s in the state: %v, want still archived", id, err)
	}
}

// The step's proof, end to end: an archived task moved back to work is
// unarchived and reopened by the one request, and the state is a replay
// of the log, its restored rows and the new events.
func TestMovingAnArchivedTaskBackToWorkReopensIt(t *testing.T) {
	for _, column := range []string{"in-progress", "todo"} {
		t.Run(column, func(t *testing.T) {
			f := newArchiveFixture(t)
			storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
			if code, body := call(t, f.server, "POST", "/api/tasks/task-2/direct", `{"to":"`+column+`"}`); code != http.StatusOK {
				t.Fatalf("moving the archived task-2 to %s: %d %s", column, code, body)
			}
			task, err := f.state.Task("task-2")
			if err != nil || task == nil || task.IsDone || task.Title != "Two" || task.Epic != "task-3" {
				t.Fatalf("task-2 should be whole and back at work: %+v, %v", task, err)
			}
			log, err := f.log.Log("task-2")
			if err != nil {
				t.Fatal(err)
			}
			var unarchived, reopened int
			for _, e := range log {
				switch e.Type {
				case events.TaskUnarchived:
					unarchived++
				case events.TaskReopened:
					reopened++
				}
			}
			if unarchived != 1 || reopened != 1 {
				t.Errorf("task-2 has %d task.unarchived and %d task.reopened, want one each", unarchived, reopened)
			}
			storetest.CheckEqual(t, f.log, f.state)
		})
	}
}

// A move to Shipping is refused while the task's team has no agent that
// ships work, and an archived task is refused that before it is unarchived:
// nothing is written, and it stays archived.
func TestMovingAnArchivedTaskToShippingWithNoShipperUnarchivesNothing(t *testing.T) {
	f := newArchiveFixture(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
	if code, body := call(t, f.server, "DELETE", "/api/agents/deployer", ""); code != http.StatusOK {
		t.Fatalf("removing the deployer: %d %s", code, body)
	}
	last := f.log.LastPosition()
	const refused = "Software has no agent that ships work\n"
	if code, body := call(t, f.server, "POST", "/api/tasks/task-2/direct", `{"to":"shipping"}`); code != http.StatusConflict || body != refused {
		t.Errorf("moving the archived task-2 to Shipping: %d %q, want 409 %q", code, body, refused)
	}
	if f.log.LastPosition() != last {
		t.Errorf("the refused move wrote events: the log moved from %d to %d", last, f.log.LastPosition())
	}
	stillArchived(t, f, "task-2")
}

// What you say to an archived task reopens it too, unarchived first; saying
// nothing is refused before anything is written.
func TestSayingSomethingToAnArchivedTaskReopensIt(t *testing.T) {
	f := newArchiveFixture(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
	last := f.log.LastPosition()
	if code, _ := call(t, f.server, "POST", "/api/tasks/task-2/message", `{"text":"  "}`); code != http.StatusBadRequest || f.log.LastPosition() != last {
		t.Errorf("saying nothing: %d, position %d, want 400 and nothing written", code, f.log.LastPosition())
	}
	stillArchived(t, f, "task-2")

	if code, body := call(t, f.server, "POST", "/api/tasks/task-2/message", `{"text":"One more thing."}`); code != http.StatusOK {
		t.Fatalf("saying something to the archived task-2: %d %s", code, body)
	}
	if got := lastTwo(t, f, "task-2"); got[0] != events.TaskUnarchived || got[1] != events.TaskReopened {
		t.Errorf("task-2's last events are %v, want task.unarchived then task.reopened", got)
	}
	if task, err := f.state.Task("task-2"); err != nil || task == nil || task.IsDone {
		t.Errorf("task-2 should be reopened: %+v, %v", task, err)
	}
	storetest.CheckEqual(t, f.log, f.state)
}

// When the task can't be unarchived, the move answers why and writes
// nothing: its blob missing is a failure, 500, and a marker that changed
// since it was read is a refusal, 409.
func TestAReopenThatCantUnarchiveWritesNothing(t *testing.T) {
	t.Run("the blob missing", func(t *testing.T) {
		f := newArchiveFixture(t)
		storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
		live, err := f.log.Log("task-2")
		if err != nil {
			t.Fatal(err)
		}
		hash := live[0].Text("archive")
		if err := os.Remove(filepath.Join(f.blobs.Dir, hash[:2], hash)); err != nil {
			t.Fatal(err)
		}
		last := f.log.LastPosition()
		if code, body := call(t, f.server, "POST", "/api/tasks/task-2/direct", `{"to":"in-progress"}`); code != http.StatusInternalServerError || !strings.Contains(body, "task-2") {
			t.Errorf("reopening with the blob missing: %d %s, want 500", code, body)
		}
		if f.log.LastPosition() != last {
			t.Errorf("the log moved to %d from %d", f.log.LastPosition(), last)
		}
		stillArchived(t, f, "task-2")
	})
	t.Run("archived again since", func(t *testing.T) {
		f := newArchiveFixture(t)
		storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
		var last int64
		f.app.archives.blobs = openedThen{Disk: f.blobs.Disk, then: func() {
			if _, rejection, err := f.log.Unarchive("task-2", f.blobs.Disk); rejection != nil || err != nil {
				t.Fatal(rejection, err)
			}
			storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
			last = f.log.LastPosition()
		}}
		code, body := call(t, f.server, "POST", "/api/tasks/task-2/direct", `{"to":"in-progress"}`)
		if code != http.StatusConflict || !strings.Contains(body, "took an event since its marker was read") {
			t.Errorf("reopening as the marker changed: %d %s, want 409", code, body)
		}
		if f.log.LastPosition() != last {
			t.Errorf("the log moved to %d from %d", f.log.LastPosition(), last)
		}
		stillArchived(t, f, "task-2")
	})
}

// Two reopens at once: the one that unarchives second finds the task
// whole, so it isn't refused, and the task is unarchived once.
func TestAReopenFindsTheTaskUnarchivedByAnother(t *testing.T) {
	f := newArchiveFixture(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
	f.app.archives.blobs = openedThen{Disk: f.blobs.Disk, then: func() {
		if _, rejection, err := f.log.Unarchive("task-2", f.blobs.Disk); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}}
	if code, body := call(t, f.server, "POST", "/api/tasks/task-2/direct", `{"to":"in-progress"}`); code != http.StatusOK {
		t.Fatalf("reopening a task unarchived by another: %d %s", code, body)
	}
	if got := lastTwo(t, f, "task-2"); got[0] != events.TaskUnarchived || got[1] != events.TaskReopened {
		t.Errorf("task-2's last events are %v, want one task.unarchived, then task.reopened", got)
	}
	storetest.CheckEqual(t, f.log, f.state)
}

// A reopen while the archiver archives the task: the archive read its rows
// before the reopen, so Store.Archive's check of the stream refuses it,
// and the task stays whole and at work.
func TestAReopenDuringAnArchiveRefusesTheArchive(t *testing.T) {
	f := newArchiveFixture(t)
	blobs := putThen{Disk: f.blobs.Disk, then: func() {
		if code, body := call(t, f.server, "POST", "/api/tasks/task-2/direct", `{"to":"in-progress"}`); code != http.StatusOK {
			t.Errorf("reopening task-2: %d %s", code, body)
		}
	}}
	summary := func() any {
		task, err := f.state.Task("task-2")
		if err != nil || task == nil {
			return nil
		}
		return tasks.SummaryOf(task, f.state.Roster())
	}
	_, rejection, err := f.log.Archive("task-2", summary, blobs)
	if err != nil || rejection == nil || !strings.Contains(rejection.Reason, "took an event since its rows were read") {
		t.Fatalf("archiving as task-2 was reopened: %v, %v; want refused", rejection, err)
	}
	if task, err := f.state.Task("task-2"); err != nil || task == nil || task.IsDone {
		t.Errorf("task-2 should be whole and reopened: %+v, %v", task, err)
	}
}

// A done epic takes no stories, archived or not, so a story made under an
// archived one is made but not filed, with the words a done epic gives,
// and the epic stays archived: unarchiving it would change nothing. Once
// you reopen it, which unarchives it, it takes the story. An archived task
// that isn't an epic says so.
func TestAStoryUnderAnArchivedEpicWaitsForItsReopening(t *testing.T) {
	f := newArchiveFixture(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
	storetest.ArchiveIn(t, f.log, f.state, "task-3", f.blobs.Disk)

	var made struct{ NotFiled []string }
	code, body := call(t, f.server, "POST", "/api/tasks", `{"title":"Three more","epic":"task-3"}`)
	if code != http.StatusOK {
		t.Fatalf("a story under the archived task-3: %d %s", code, body)
	}
	if err := json.Unmarshal([]byte(body), &made); err != nil || !slices.Equal(made.NotFiled, []string{"task-3 is done; reopen it to add stories"}) {
		t.Errorf("not filed: %q, %v", made.NotFiled, err)
	}
	stillArchived(t, f, "task-3")
	if code, body := call(t, f.server, "PUT", "/api/tasks/task-4/epic", `{"epic":"task-2"}`); code != http.StatusConflict || body != "task-2 isn't an epic\n" {
		t.Errorf("filing under the archived story task-2: %d %q", code, body)
	}

	if code, body := call(t, f.server, "POST", "/api/tasks/task-3/direct", `{"to":"in-progress"}`); code != http.StatusOK {
		t.Fatalf("reopening the archived task-3: %d %s", code, body)
	}
	if code, body := call(t, f.server, "PUT", "/api/tasks/task-4/epic", `{"epic":"task-3"}`); code != http.StatusOK {
		t.Fatalf("filing task-4 under the reopened task-3: %d %s", code, body)
	}
	epic, err := f.state.Task("task-3")
	if err != nil || epic == nil || !slices.Equal(epic.Stories, []string{"task-2", "task-4"}) {
		t.Errorf("task-3 should list task-2 and task-4: %+v, %v", epic, err)
	}
	stillArchived(t, f, "task-2")
	storetest.CheckEqual(t, f.log, f.state)
}
