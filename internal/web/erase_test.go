package web

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy/hooks"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// Erasing a task drops what web's cache of opened archives keeps of it
// and of the archived tasks whose copies of its words it redacts; the
// redaction route's test reads the cache too. The erase route's other
// tests are in internal/domain/tasks/tasksapi/erase_test.go, whose
// fixture this mirrors.

// plantedHere is in the erased task's words and its own shared file, and,
// as its title and summary, in the copies its epic and its asker keep.
const plantedHere = "planted-5be1d07a"

var (
	you     = events.Actor{Kind: events.Human, ID: "you"}
	loop    = events.Actor{Kind: events.System, ID: tasks.Loop}
	lead    = events.Actor{Kind: events.Agent, ID: "tech-lead"}
	machine = events.Actor{Kind: events.Worker, ID: workers.Local}
)

// eraseFixture is a server on a log in its own folder, with a snapshot
// beside it, where task-2 asked by task-1 and a story of the epic task-3
// has shared two files: one of its own, and one task-1 shares too.
type eraseFixture struct {
	dir       string
	log       *store.Store
	state     *current.State
	blobs     sharedfiles.Disk
	snapshots *current.SnapshotWriter
	web       *Server
	server    *httptest.Server
	hooks     *hooks.Handler // ends turns, as their hooks' calls do
	stopped   []string       // the sessions ended, and whether each task was erased already
	own, both string         // the blobs of task-2's files
}

func newEraseFixture(t *testing.T) *eraseFixture {
	t.Helper()
	f := &eraseFixture{dir: t.TempDir()}
	f.log, f.state = storetest.OpenWith(t, filepath.Join(f.dir, "agentry.db"), storetest.WithExtensions())
	for _, p := range agents.SeedProposals(nil) {
		f.log.Propose(p)
	}
	f.blobs = sharedfiles.Disk{Dir: filepath.Join(f.dir, "files")}
	f.snapshots = current.NewSnapshotWriter(filepath.Join(f.dir, "state.snapshot"), "build-1", f.state, f.log)
	f.snapshots.Start()
	t.Cleanup(f.snapshots.Close) // before the log closes
	stop := func(task string) {
		if _, err := f.state.Task(task); errors.Is(err, current.ErrErased) {
			task += ", already erased"
		}
		f.stopped = append(f.stopped, task)
	}
	f.hooks = hooks.NewHandler(f.log, f.state, nil)
	f.web = NewServer(f.log, f.state, Config{UIToken: token, DeskToken: "desk-secret", DefaultRepo: "/r",
		Blobs: f.blobs, Snapshots: f.snapshots, StopSession: stop, Redactions: f.hooks.Redactions(), Extensions: storetest.Installed()}, http.NotFoundHandler(), nil)
	f.server = httptest.NewServer(f.web.Handler())
	t.Cleanup(f.server.Close)

	f.own, f.both = f.put(t, "task-2's own notes, "+plantedHere), f.put(t, "notes both tasks share")
	f.propose(t,
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Asker", "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-1", Actor: you, Type: events.FileAdded, Data: map[string]any{"name": "shared.md", "hash": f.both, "size": 21, "by": "you"}},
		events.Proposal{Stream: "task-2", Actor: lead, Type: events.TaskCreated, Data: map[string]any{"title": "Asked " + plantedHere, "repo": "/r",
			"coordinator": "tech-lead", "parent": "task-1"}},
		events.Proposal{Stream: "task-1", Actor: lead, Type: events.TaskRequested, Data: map[string]any{"child": "task-2", "team": "Software",
			"request": "Asked", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.FileAdded, Data: map[string]any{"name": "mine.md", "hash": f.own, "size": 40, "by": "you"}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.FileAdded, Data: map[string]any{"name": "shared.md", "hash": f.both, "size": 21, "by": "you"}},
	)
	if code, body := call(t, f.server, "POST", "/api/tasks", `{"title":"Ship it","stories":["task-2"]}`); code != http.StatusOK {
		t.Fatalf("creating the epic: %d %s", code, body)
	}
	return f
}

// done finishes task-2, and has its epic and its asker hear its summary,
// as the coordinator loop has them hear it.
func (f *eraseFixture) done(t *testing.T) {
	t.Helper()
	summary := "Laid out, " + plantedHere + "."
	f.propose(t,
		events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": summary, "moved": true}},
		events.Proposal{Stream: "task-3", Actor: loop, Type: events.StoryDone, Data: map[string]any{"story": "task-2", "summary": summary, "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-1", Actor: loop, Type: events.RequestAnswered, Data: map[string]any{"child": "task-2", "summary": summary, "coordinator": "tech-lead"}},
	)
}

func (f *eraseFixture) put(t *testing.T, content string) string {
	t.Helper()
	hash, _, err := f.blobs.Put(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func (f *eraseFixture) propose(t *testing.T, proposals ...events.Proposal) {
	t.Helper()
	for _, p := range proposals {
		if _, rejection, err := f.log.Propose(p); rejection != nil || err != nil {
			t.Fatalf("%s in %s: %v, %v", p.Type, p.Stream, rejection, err)
		}
	}
}

// erase erases a task through the route.
func (f *eraseFixture) erase(t *testing.T, id string) {
	t.Helper()
	if code, body := call(t, f.server, "POST", "/api/tasks/"+id+"/erase", `{"why":"Asked to be forgotten."}`); code != http.StatusOK {
		t.Fatalf("erasing %s: %d %s", id, code, body)
	}
}

// An archived task opened from the page before it is erased is dropped
// from the cache of opened archives, which held its words in memory; its
// asker, opened too, is kept.
func TestErasingForgetsAnOpenedArchivedTask(t *testing.T) {
	f := newEraseFixture(t)
	f.done(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs)
	storetest.ArchiveIn(t, f.log, f.state, "task-1", f.blobs)
	for _, id := range []string{"task-2", "task-1"} {
		if code, body := call(t, f.server, "GET", "/api/tasks/"+id, ""); code != http.StatusOK {
			t.Fatalf("opening %s: %d %s", id, code, body)
		}
	}
	if got := f.opened(); !slices.Equal(got, []string{"task-2", "task-1"}) {
		t.Fatalf("opened archives %q, want task-2 and task-1", got)
	}
	f.erase(t, "task-2")
	if got := f.opened(); slices.Contains(got, "task-2") {
		t.Errorf("opened archives after the erasure %q, want no task-2", got)
	}
}

// An archived asker opened from the page before the task it asked is
// erased has its copy redacted through its blob, and the cache keeps none
// of the words it held before: its page, read again, holds no copy.
func TestErasingForgetsTheOpenedArchivesItRedacts(t *testing.T) {
	f := newEraseFixture(t)
	f.done(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs)
	storetest.ArchiveIn(t, f.log, f.state, "task-1", f.blobs)
	if code, body := call(t, f.server, "GET", "/api/tasks/task-1/events", ""); code != http.StatusOK || !strings.Contains(body, plantedHere) {
		t.Fatalf("opening task-1 before the erasure: %d %s", code, body)
	}
	f.erase(t, "task-2")
	if got := f.opened(); slices.Contains(got, "task-1") {
		t.Errorf("opened archives after the erasure %q, want no task-1", got)
	}
	if code, body := call(t, f.server, "GET", "/api/tasks/task-1/events", ""); code != http.StatusOK || strings.Contains(body, plantedHere) {
		t.Errorf("task-1's events after the erasure: %d %s", code, body)
	}
}

// opened is the streams the cache of opened archives keeps, oldest first.
func (f *eraseFixture) opened() []string { return f.web.archives.Opened() }

func markerOf(t *testing.T, f *eraseFixture, id string) string {
	t.Helper()
	live, err := f.log.Log(id)
	if err != nil {
		t.Fatal(err)
	}
	marker, archived := store.MarkerOf(live)
	if !archived {
		t.Fatalf("%s isn't archived", id)
	}
	return marker.Text("archive")
}
