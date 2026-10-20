package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// Erasing a task through the page (docs/design/log-growth.md section 5,
// "Removing a whole task"). The store's side is in store's and storetest's
// erase tests; these see what the route adds: the turn stopped, the shared
// files, the copies in other tasks and the snapshot.

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
	server    *httptest.Server
	stopped   []string // the sessions ended, and whether each task was erased already
	own, both string   // the blobs of task-2's files
}

func newEraseFixture(t *testing.T) *eraseFixture {
	t.Helper()
	f := &eraseFixture{dir: t.TempDir()}
	f.log, f.state = storetest.Open(t, filepath.Join(f.dir, "agentry.db"))
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
	f.server = httptest.NewServer(NewServer(f.log, f.state, Config{UIToken: token, DeskToken: "desk-secret", DefaultRepo: "/r",
		Blobs: f.blobs, Snapshots: f.snapshots, StopSession: stop}, http.NotFoundHandler(), nil).Handler())
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

// erase erases a task through the route, and is its answer.
func (f *eraseFixture) erase(t *testing.T, id string) erased {
	t.Helper()
	code, body := call(t, f.server, "POST", "/api/tasks/"+id+"/erase", `{"why":"Asked to be forgotten."}`)
	if code != http.StatusOK {
		t.Fatalf("erasing %s: %d %s", id, code, body)
	}
	var answer erased
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatal(err)
	}
	return answer
}

// holdsPlanted fails the test unless some file in the folder holds the
// planted text: the guard that the test searches the right bytes.
func (f *eraseFixture) holdsPlanted(t *testing.T) {
	t.Helper()
	if err := f.snapshots.WriteNow(); err != nil {
		t.Fatal(err)
	}
	holding := storetest.HoldingAny(t, f.dir, plantedHere)
	for _, want := range []string{"state.snapshot", f.own} {
		if !slices.Contains(holding, want) {
			t.Fatalf("before the erasure %s should hold the planted text; these do: %v", want, holding)
		}
	}
}

// erasedFromDisk fails the test if any file in the folder still holds the
// planted text, and checks the blobs: task-2's own file is gone, the one
// task-1 names too is kept.
func (f *eraseFixture) erasedFromDisk(t *testing.T, answer erased) {
	t.Helper()
	if len(answer.Warnings) != 0 {
		t.Errorf("the erasure warned: %q", answer.Warnings)
	}
	if holding := storetest.HoldingAny(t, f.dir, plantedHere); len(holding) != 0 {
		t.Errorf("after the erasure these files still hold the planted text: %v", holding)
	}
	if f.blobs.Has(f.own) {
		t.Error("the blob only task-2 named is still there")
	}
	if !f.blobs.Has(f.both) {
		t.Error("the blob task-1 names too was deleted")
	}
}

// The step's proof for a live task: after the route answers, the planted
// text, from its rows, its own shared file, and its summary's copies in its
// epic and its asker, is in no row, WAL, snapshot or blob.
func TestErasingALiveTaskLeavesNoTraceOnDisk(t *testing.T) {
	f := newEraseFixture(t)
	f.done(t)
	f.holdsPlanted(t)
	answer := f.erase(t, "task-2")
	if answer.ID != "task-2" || time.Since(answer.At) > time.Minute {
		t.Errorf("the answer: %+v", answer)
	}
	f.erasedFromDisk(t, answer)
	for _, id := range []string{"task-1", "task-3"} {
		if task, err := f.state.Task(id); err != nil || task == nil {
			t.Errorf("%s after the erasure: %v, %v", id, task, err)
		}
	}
}

// The same for an archived task, erased as it is, whose asker is archived
// too: its copy is redacted through its blob. Both old archive blobs are
// gone.
func TestErasingAnArchivedTaskLeavesNoTraceOnDisk(t *testing.T) {
	f := newEraseFixture(t)
	f.done(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs)
	storetest.ArchiveIn(t, f.log, f.state, "task-1", f.blobs)
	archives := []string{markerOf(t, f, "task-2"), markerOf(t, f, "task-1")}
	f.holdsPlanted(t)
	f.erasedFromDisk(t, f.erase(t, "task-2"))
	for _, hash := range archives {
		if f.blobs.Has(hash) {
			t.Errorf("the archive blob %s is still there", hash)
		}
	}
	if _, err := f.state.Task("task-1"); !errors.Is(err, current.ErrArchived) {
		t.Errorf("task-1, redacted, should be archived again: %v", err)
	}
}

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

// Only the rows that copy an erased task's words are redacted: its epic's
// task.story_added and task.story_done for it, and its asker's
// task.request_answered. The same words in the epic's and the asker's own
// rows, here their titles, are theirs and kept, live and archived.
func TestErasingKeepsTheSameWordsOutsideTheCopies(t *testing.T) {
	for _, archived := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "archived"}[archived], func(t *testing.T) {
			f := newEraseFixture(t)
			f.done(t)
			// task-4, asked by task-1 and the one story of the epic task-5,
			// shares the epic's title, and is done with the asker's title.
			f.propose(t,
				events.Proposal{Stream: "task-4", Actor: lead, Type: events.TaskCreated, Data: map[string]any{"title": "Tests", "repo": "/r",
					"coordinator": "tech-lead", "parent": "task-1"}},
				events.Proposal{Stream: "task-1", Actor: lead, Type: events.TaskRequested, Data: map[string]any{"child": "task-4", "team": "Software",
					"request": "Write them", "coordinator": "tech-lead"}},
			)
			if code, body := call(t, f.server, "POST", "/api/tasks", `{"title":"Tests","stories":["task-4"]}`); code != http.StatusOK {
				t.Fatalf("creating the epic: %d %s", code, body)
			}
			f.propose(t,
				events.Proposal{Stream: "task-4", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "Asker", "moved": true}},
				events.Proposal{Stream: "task-5", Actor: loop, Type: events.StoryDone, Data: map[string]any{"story": "task-4", "summary": "Asker", "coordinator": "tech-lead"}},
				events.Proposal{Stream: "task-1", Actor: loop, Type: events.RequestAnswered, Data: map[string]any{"child": "task-4", "summary": "Asker", "coordinator": "tech-lead"}},
			)
			if archived {
				storetest.ArchiveIn(t, f.log, f.state, "task-5", f.blobs)
				storetest.ArchiveIn(t, f.log, f.state, "task-1", f.blobs)
			}
			if answer := f.erase(t, "task-4"); len(answer.Warnings) != 0 {
				t.Errorf("the erasure warned: %q", answer.Warnings)
			}
			for _, c := range []struct {
				stream string
				kind   events.Type
				field  string
				want   string
			}{
				{"task-5", events.TaskCreated, "title", "Tests"},
				{"task-5", events.StoryAdded, "title", store.Redacted},
				{"task-5", events.StoryDone, "summary", store.Redacted},
				{"task-1", events.TaskCreated, "title", "Asker"},
				{"task-1", events.RequestAnswered, "summary", store.Redacted},
			} {
				if got := textIn(t, f, c.stream, c.kind, c.field); !slices.Contains(got, c.want) {
					t.Errorf("%s's %s %s: %q, want %q among them", c.stream, c.kind, c.field, got, c.want)
				}
			}
			if got := textIn(t, f, "task-1", events.RequestAnswered, "summary"); slices.Contains(got, "Asker") {
				t.Errorf("task-1's task.request_answered for task-4 still holds its summary: %q", got)
			}
		})
	}
}

// textIn is field of each event of kind in stream's whole record, an
// archived task's read through its blob.
func textIn(t *testing.T, f *eraseFixture, stream string, kind events.Type, field string) []string {
	t.Helper()
	history, err := f.log.History(stream, store.FromBlobs(f.blobs))
	if err != nil {
		t.Fatal(err)
	}
	var texts []string
	for _, e := range history {
		if e.Type == kind {
			texts = append(texts, e.Text(field))
		}
	}
	return texts
}

// A turn that runs is stopped while the task is still there, so the runner
// can find the machine it runs on; a task with no turn running isn't.
func TestErasingStopsTheTurnThatRunsFirst(t *testing.T) {
	f := newEraseFixture(t)
	f.propose(t, events.Proposal{Stream: "task-1", Actor: machine, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/task-1", "branch": "b", "port": 7901}})
	f.propose(t, events.Proposal{Stream: "task-1", Actor: machine, Type: events.TurnStarted, Correlation: "turn-1",
		Data: map[string]any{"agent": "tech-lead", "engine": "claude", "hash": "h", "worker": workers.Local}})
	f.erase(t, "task-1")
	f.erase(t, "task-2")
	if want := []string{"task-1"}; !slices.Equal(f.stopped, want) {
		t.Errorf("sessions stopped: %q, want %q", f.stopped, want)
	}
}

// An erased task reads as erased, with the day, not as no such task; the
// list, the board and the epic leave it out without failing.
func TestAnErasedTaskReadsAsGone(t *testing.T) {
	f := newEraseFixture(t)
	answer := f.erase(t, "task-2")
	want := "task-2 was erased on " + answer.At.Format("2 January 2006")
	for _, path := range []string{"/api/tasks/task-2", "/api/tasks/task-2/events"} {
		if code, body := call(t, f.server, "GET", path, ""); code != http.StatusGone || strings.TrimSpace(body) != want {
			t.Errorf("GET %s: %d %q, want 410 %q", path, code, body, want)
		}
	}
	if code, body := call(t, f.server, "POST", "/api/tasks/task-2/message", `{"text":"Hello?"}`); code != http.StatusGone {
		t.Errorf("a message to the erased task: %d %s, want 410", code, body)
	}
	var list []map[string]any
	code, body := call(t, f.server, "GET", "/api/tasks", "")
	if code != http.StatusOK || json.Unmarshal([]byte(body), &list) != nil || len(list) != 2 || list[0]["id"] == "task-2" || list[1]["id"] == "task-2" {
		t.Errorf("the list: %d %s, want task-1 and task-3 alone", code, body)
	}
	for _, path := range []string{"/api/features", "/api/tasks/task-3", "/api/tasks/task-1"} {
		if code, body := call(t, f.server, "GET", path, ""); code != http.StatusOK {
			t.Errorf("GET %s: %d %s", path, code, body)
		}
	}
}

// A refused erasure, as a second one, writes nothing and answers 409; a
// task never made is 404; and the desk can't erase.
func TestARefusedErasureWritesNothing(t *testing.T) {
	f := newEraseFixture(t)
	f.erase(t, "task-2")
	last := f.log.LastPosition()
	if code, body := call(t, f.server, "POST", "/api/tasks/task-2/erase", `{"why":"Again."}`); code != http.StatusConflict || !strings.Contains(body, "task-2 is erased") {
		t.Errorf("a second erasure: %d %s, want 409", code, body)
	}
	if code, _ := call(t, f.server, "POST", "/api/tasks/task-9/erase", `{}`); code != http.StatusNotFound {
		t.Errorf("erasing no such task: %d, want 404", code)
	}
	if code, _ := callAs(t, f.server, "desk-secret", "POST", "/api/tasks/task-1/erase"); code != http.StatusUnauthorized {
		t.Errorf("the desk erasing: %d, want 401", code)
	}
	if f.log.LastPosition() != last {
		t.Errorf("the refusals wrote up to %d from %d", f.log.LastPosition(), last)
	}
}
