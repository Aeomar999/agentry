package tasksapi

import (
	"io"
	"path/filepath"
	"sync/atomic"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// countedBlobs is the log's blobs with the blobs opened counted; it
// mirrors internal/web/archives_test.go's.
type countedBlobs struct {
	sharedfiles.Disk
	opens atomic.Int32
}

func (c *countedBlobs) Open(hash string) (io.ReadCloser, error) {
	c.opens.Add(1)
	return c.Disk.Open(hash)
}

// archiveFixture is a log whose blobs are counted, with task-2 done, ready
// to archive, and task-1 open.
type archiveFixture struct {
	log   *store.Store
	state *current.State
	blobs *countedBlobs
}

func newArchiveFixture(t *testing.T) *archiveFixture {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	for _, title := range []string{"One", "Two"} {
		if _, rejection, err := s.Create("task-", events.Proposal{Actor: httpapi.You, Type: events.TaskCreated,
			Data: map[string]any{"title": title, "repo": "/r", "coordinator": "tech-lead"}}); err != nil || rejection != nil {
			t.Fatal(title, rejection, err)
		}
	}
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-2", Actor: httpapi.You, Type: events.TaskDone,
		Data: map[string]any{"summary": "The user moved it to Done, as it was.", "moved": true}}); err != nil || rejection != nil {
		t.Fatal(rejection, err)
	}
	return &archiveFixture{log: s, state: state, blobs: &countedBlobs{Disk: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}}}
}

// A second read of an archived task neither gunzips nor folds again: it
// is handed the task the first read folded.
func TestASecondReadOfAnArchivedTaskFoldsOnce(t *testing.T) {
	f := newArchiveFixture(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
	a := NewArchives(f.blobs)
	read := func() *tasks.Task {
		t.Helper()
		history, err := a.history(f.log, "task-2")
		if err != nil {
			t.Fatal(err)
		}
		marker, _ := store.MarkerOf(history)
		task, err := a.task(marker, history)
		if err != nil || task == nil {
			t.Fatalf("task-2: %v, %v", task, err)
		}
		return task
	}
	if first, second := read(), read(); first != second {
		t.Error("the second read folded task-2 again, want the first read's task")
	}
}

// The cache keeps a stream's events at one marker: another marker of the
// stream is read afresh. It keeps the last openedMax read, and drops the
// one used longest ago.
func TestTheCacheKeepsTheLastFewMarkers(t *testing.T) {
	f := newArchiveFixture(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
	live, err := f.log.Log("task-2")
	if err != nil {
		t.Fatal(err)
	}
	marker := live[0]
	a := NewArchives(f.blobs)
	read := func(position int64) {
		t.Helper()
		at := marker
		at.Position = position
		if found, err := a.events(at); err != nil || len(found) == 0 {
			t.Fatalf("at %d: %d events, %v", position, len(found), err)
		}
	}
	read(marker.Position)
	read(marker.Position)
	if opens := f.blobs.opens.Load(); opens != 1 {
		t.Errorf("the same marker twice opened %d blobs, want 1", opens)
	}
	read(marker.Position + 1)
	if opens := f.blobs.opens.Load(); opens != 2 {
		t.Errorf("another marker of the stream opened %d blobs in all, want 2", opens)
	}
	for i := range int64(openedMax - 1) { // the first marker is now the one used longest ago
		read(marker.Position + 2 + i)
	}
	opens := f.blobs.opens.Load()
	read(marker.Position + 1)
	if f.blobs.opens.Load() != opens {
		t.Errorf("a marker among the last %d opened its blob again", openedMax)
	}
	read(marker.Position)
	if f.blobs.opens.Load() != opens+1 {
		t.Errorf("the marker used longest ago was still kept")
	}
}
