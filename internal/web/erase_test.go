package web

import (
	"cmp"
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
	"gitlab.com/dev.chempa/agentry/internal/coordinator"
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
	web       *Server
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
	f.web = NewServer(f.log, f.state, Config{UIToken: token, DeskToken: "desk-secret", DefaultRepo: "/r",
		Blobs: f.blobs, Snapshots: f.snapshots, StopSession: stop}, http.NotFoundHandler(), nil)
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

// A task that can't be read, here the asker whose archive blob is lost,
// doesn't keep the copies in the tasks after it: the epic's are redacted,
// and the answer warns that copies may stay, naming the one passed over.
func TestErasingRedactsPastATaskThatCantBeRead(t *testing.T) {
	f := newEraseFixture(t)
	f.done(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-1", f.blobs)
	if err := f.blobs.Remove(markerOf(t, f, "task-1")); err != nil {
		t.Fatal(err)
	}
	answer := f.erase(t, "task-2")
	if !slices.ContainsFunc(answer.Warnings, func(w string) bool {
		return strings.Contains(w, "its words may stay") && strings.Contains(w, "task-1:")
	}) {
		t.Errorf("the warnings %q should say task-1 can't be read", answer.Warnings)
	}
	for _, kind := range []events.Type{events.StoryDone, events.StoryAdded} {
		for _, field := range []string{"summary", "title"} {
			for _, text := range textIn(t, f, "task-3", kind, field) {
				if strings.Contains(text, plantedHere) {
					t.Errorf("the epic's %s %s still holds the erased task's words: %q", kind, field, text)
				}
			}
		}
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
func (f *eraseFixture) opened() []string {
	f.web.archives.mu.Lock()
	defer f.web.archives.mu.Unlock()
	var streams []string
	for _, o := range f.web.archives.recent {
		streams = append(streams, o.stream)
	}
	return streams
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

// A story whose title starts its summary leaves no part of the summary
// behind, whether only the story_added of an epic it joined once done
// holds the summary or the epic and the asker heard it finish.
func TestErasingATitleInsideItsSummaryLeavesNoneOfIt(t *testing.T) {
	const title, summary = "Add tests", "Add tests for X"
	for _, archived := range []bool{false, true} {
		for _, heard := range []bool{false, true} {
			name := map[bool]string{false: "live", true: "archived"}[archived] + map[bool]string{false: ", joined once done", true: ", heard done"}[heard]
			t.Run(name, func(t *testing.T) {
				f := newEraseFixture(t)
				f.done(t)
				f.propose(t,
					events.Proposal{Stream: "task-4", Actor: lead, Type: events.TaskCreated, Data: map[string]any{"title": title, "repo": "/r",
						"coordinator": "tech-lead", "parent": "task-1"}},
					events.Proposal{Stream: "task-1", Actor: lead, Type: events.TaskRequested, Data: map[string]any{"child": "task-4", "team": "Software",
						"request": "Write them", "coordinator": "tech-lead"}},
				)
				done := events.Proposal{Stream: "task-4", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": summary, "moved": true}}
				if !heard {
					f.propose(t, done)
				}
				if code, body := call(t, f.server, "POST", "/api/tasks", `{"title":"Tests","stories":["task-4"]}`); code != http.StatusOK {
					t.Fatalf("creating the epic: %d %s", code, body)
				}
				if heard {
					f.propose(t, done,
						events.Proposal{Stream: "task-5", Actor: loop, Type: events.StoryDone, Data: map[string]any{"story": "task-4", "summary": summary, "coordinator": "tech-lead"}},
						events.Proposal{Stream: "task-1", Actor: loop, Type: events.RequestAnswered, Data: map[string]any{"child": "task-4", "summary": summary, "coordinator": "tech-lead"}},
					)
				}
				if archived {
					storetest.ArchiveIn(t, f.log, f.state, "task-5", f.blobs)
					storetest.ArchiveIn(t, f.log, f.state, "task-1", f.blobs)
				}
				if answer := f.erase(t, "task-4"); len(answer.Warnings) != 0 {
					t.Errorf("the erasure warned: %q", answer.Warnings)
				}
				if got := textIn(t, f, "task-5", events.TaskCreated, "title"); !slices.Equal(got, []string{"Tests"}) {
					t.Errorf("the epic's own title: %q, want it kept", got)
				}
				for _, stream := range []string{"task-5", "task-1"} {
					history, err := f.log.History(stream, store.FromBlobs(f.blobs))
					if err != nil {
						t.Fatal(err)
					}
					for _, e := range history {
						data, _ := json.Marshal(e.Data)
						if strings.Contains(string(data), " for X") || strings.Contains(string(data), title) {
							t.Errorf("%s's %s still holds some of task-4's words: %s", stream, e.Type, data)
						}
					}
				}
			})
		}
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

// An erased task can never be done, so neither the list, a detail nor the
// epic's story list shows it blocking the story after it; one that isn't
// done still does.
func TestAnErasedTaskBlocksNoStory(t *testing.T) {
	f := newEraseFixture(t)
	f.propose(t, events.Proposal{Stream: "task-4", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "After both", "repo": "/r",
		"coordinator": "tech-lead", "after": []string{"task-1", "task-2"}}})
	if code, body := call(t, f.server, "PUT", "/api/tasks/task-4/epic", `{"epic":"task-3"}`); code != http.StatusOK {
		t.Fatalf("task-4 into the epic: %d %s", code, body)
	}
	f.erase(t, "task-2")

	var list []taskSummary
	_, body := call(t, f.server, "GET", "/api/tasks", "")
	json.Unmarshal([]byte(body), &list)
	var detail, epic taskDetail
	_, body = call(t, f.server, "GET", "/api/tasks/task-4", "")
	json.Unmarshal([]byte(body), &detail)
	_, body = call(t, f.server, "GET", "/api/tasks/task-3", "")
	json.Unmarshal([]byte(body), &epic)
	isFour := func(summary taskSummary) bool { return summary.ID == "task-4" }
	for _, view := range []struct {
		name      string
		blockedBy func() []string
	}{
		{"the list", func() []string { return list[slices.IndexFunc(list, isFour)].BlockedBy }},
		{"task-4's detail", func() []string { return detail.BlockedBy }},
		{"the epic's story list", func() []string { return epic.StoryList[slices.IndexFunc(epic.StoryList, isFour)].BlockedBy }},
	} {
		if got := view.blockedBy(); !slices.Equal(got, []string{"task-1"}) {
			t.Errorf("%s has task-4 blocked by %v, want only task-1", view.name, got)
		}
	}
}

// A route that acts on a task answers an erased one 410, with the day, as
// the reads do: the board's move, the limits, the repo, the shared files
// and the Workspace. A task there never was is still 404.
func TestTheRoutesOnAnErasedTaskSayItIsGone(t *testing.T) {
	f := newEraseFixture(t)
	answer := f.erase(t, "task-2")
	want := "task-2 was erased on " + answer.At.Format("2 January 2006")
	repo, _ := json.Marshal(map[string]string{"repo": gitRepo(t)})
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/tasks/{task}/direct", `{"to":"todo"}`},
		{"PUT", "/api/tasks/{task}/budget", `{"budget":5}`},
		{"PUT", "/api/tasks/{task}/turns", `{"maxTurns":5}`},
		{"PUT", "/api/tasks/{task}/repo", string(repo)},
		{"GET", "/api/tasks/{task}/shared", ""},
		{"GET", "/api/tasks/{task}/shared/file?name=mine.md", ""},
		{"POST", "/api/tasks/{task}/shared?name=new.md", "notes"},
		{"GET", "/api/tasks/{task}/files", ""},
		{"GET", "/api/tasks/{task}/file?path=README.md", ""},
		{"GET", "/api/tasks/{task}/commits", ""},
	} {
		erased := strings.ReplaceAll(c.path, "{task}", "task-2")
		if code, body := call(t, f.server, c.method, erased, c.body); code != http.StatusGone || strings.TrimSpace(body) != want {
			t.Errorf("%s %s: %d %q, want 410 %q", c.method, erased, code, body, want)
		}
		unknown := strings.ReplaceAll(c.path, "{task}", "task-9")
		if code, body := call(t, f.server, c.method, unknown, c.body); code != http.StatusNotFound {
			t.Errorf("%s %s: %d %q, want 404", c.method, unknown, code, body)
		}
	}
}

// A turn that runs is stopped while the task is still there, so the runner
// can find the machine it runs on; so is one whose turn has ended, as its
// session may still be there, idle, until the loop stops it. An archived
// task has none.
func TestErasingStopsTheTurnThatRunsFirst(t *testing.T) {
	f := newEraseFixture(t)
	f.propose(t, events.Proposal{Stream: "task-1", Actor: machine, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/task-1", "branch": "b", "port": 7901}})
	f.propose(t, events.Proposal{Stream: "task-1", Actor: machine, Type: events.TurnStarted, Correlation: "turn-1",
		Data: map[string]any{"agent": "tech-lead", "engine": "claude", "hash": "h", "worker": workers.Local}})
	f.propose(t, events.Proposal{Stream: "task-3", Actor: machine, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/task-3", "branch": "b", "port": 7902}})
	for _, kind := range []events.Type{events.TurnStarted, events.TurnEnded} {
		f.propose(t, events.Proposal{Stream: "task-3", Actor: machine, Type: kind, Correlation: "turn-1",
			Data: map[string]any{"agent": "tech-lead", "engine": "claude", "hash": "h", "worker": workers.Local, "report": "Waiting."}})
	}
	f.done(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs)
	f.erase(t, "task-1")
	f.erase(t, "task-2")
	f.erase(t, "task-3")
	if want := []string{"task-1", "task-3"}; !slices.Equal(f.stopped, want) {
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

// An erased story is taken out of its epic first, so the epic doesn't wait
// on it for ever: task-3's other story task-4 is done, and task-2, live and
// not done or archived once done, is erased. Its coordinator may finish,
// is told to when it waits, has no story left to wake, and the card counts
// the one story left. Closing it from the page answers 200.
func TestErasingAStoryTakesItOutOfItsEpic(t *testing.T) {
	for _, archived := range []bool{false, true} {
		t.Run(map[bool]string{false: "live", true: "archived"}[archived], func(t *testing.T) {
			f := newEraseFixture(t)
			f.propose(t, events.Proposal{Stream: "task-4", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Other", "repo": "/r", "coordinator": "tech-lead"}})
			if code, body := call(t, f.server, "PUT", "/api/tasks/task-4/epic", `{"epic":"task-3"}`); code != http.StatusOK {
				t.Fatalf("task-4 into the epic: %d %s", code, body)
			}
			f.propose(t,
				events.Proposal{Stream: "task-4", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "Other done.", "moved": true}},
				events.Proposal{Stream: "task-3", Actor: loop, Type: events.StoryDone, Data: map[string]any{"story": "task-4", "summary": "Other done.", "coordinator": "tech-lead"}},
			)
			if archived {
				f.done(t)
				storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs)
			}
			f.erase(t, "task-2")

			epic, err := f.state.Task("task-3")
			if err != nil || epic == nil {
				t.Fatalf("the epic: %v", err)
			}
			if !slices.Equal(epic.Stories, []string{"task-4"}) || !slices.Equal(epic.StoriesDone, []string{"task-4"}) {
				t.Errorf("the epic's stories %v, done %v, want task-4 alone in both", epic.Stories, epic.StoriesDone)
			}
			if got := textIn(t, f, "task-3", events.StoryRemoved, "story"); !slices.Equal(got, []string{"task-2"}) {
				t.Errorf("the epic's task.story_removed: %q, want task-2's", got)
			}
			if last := epic.Trail[len(epic.Trail)-1]; strings.Contains(last.Text, plantedHere) {
				t.Errorf("the epic's trail names the erased story's words: %q", last.Text)
			}
			for _, c := range []struct {
				report string
				want   coordinator.MoveKind
				says   string
			}{
				{`{"action": "done", "summary": "Shipped."}`, coordinator.ApplyDecision, ""},
				{`{"action": "wait"}`, coordinator.RaiseIssue, "every story of this epic is done"},
			} {
				decided := epic.Clone()
				for _, e := range []events.Event{
					{Proposal: events.Proposal{Stream: "task-3", Actor: machine, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/task-3", "branch": "b", "port": 7901}}},
					{Proposal: events.Proposal{Stream: "task-3", Actor: machine, Type: events.TurnStarted, Correlation: "turn-9", Data: map[string]any{"agent": "tech-lead", "hash": "h"}}},
					{Proposal: events.Proposal{Stream: "task-3", Actor: machine, Type: events.TurnEnded, Correlation: "turn-9", Data: map[string]any{"agent": "tech-lead", "report": c.report}}},
				} {
					decided.Apply(e)
				}
				if move := coordinator.Next(decided, f.state.Roster()); move.Kind != c.want || !strings.Contains(move.Reason, c.says) {
					t.Errorf("%s: got %s (%q), want %s with %q", c.report, move.Kind, move.Reason, c.want, c.says)
				}
			}
			if summary, _ := f.state.Summary("task-3"); summary.Stories != 1 || summary.StoriesDone != 1 {
				t.Errorf("the card counts %d of %d done, want 1 of 1", summary.StoriesDone, summary.Stories)
			}
			if code, body := call(t, f.server, "POST", "/api/tasks/task-3/direct", `{"to":"done"}`); code != http.StatusOK {
				t.Errorf("closing the epic: %d %s, want 200", code, body)
			}
		})
	}
}

// A done epic that still lists an erased story takes it out as it is
// reopened, before its task.reopened, by either route that reopens it: no
// open story is left to keep it from finishing. It lists one erased while
// the epic was archived, as leaveEpic leaves an archived epic as it is,
// and a live one lists one when a clean-up failed part way, as here, where
// the store erased the story past the route.
func TestAReopenedEpicDropsItsErasedStories(t *testing.T) {
	erasings := []struct {
		name  string
		erase func(t *testing.T, f *eraseFixture)
	}{
		{"while archived", func(t *testing.T, f *eraseFixture) {
			storetest.ArchiveIn(t, f.log, f.state, "task-3", f.blobs)
			f.erase(t, "task-2")
		}},
		{"while live", func(t *testing.T, f *eraseFixture) {
			if _, rejection, err := f.log.Erase("task-2", you, "Gone.", f.blobs); rejection != nil || err != nil {
				t.Fatalf("erasing task-2 in the store: %v, %v", rejection, err)
			}
		}},
	}
	routes := []struct{ name, path, body string }{
		{"moved to To do", "/api/tasks/task-3/direct", `{"to":"todo"}`},
		{"moved to In progress", "/api/tasks/task-3/direct", `{"to":"in-progress"}`},
		{"told", "/api/tasks/task-3/message", `{"text":"One more thing."}`},
	}
	for _, erasing := range erasings {
		for _, c := range routes {
			t.Run(erasing.name+", "+c.name, func(t *testing.T) {
				f := newEraseFixture(t)
				if code, body := call(t, f.server, "POST", "/api/tasks/task-3/direct", `{"to":"done"}`); code != http.StatusOK {
					t.Fatalf("closing the epic: %d %s", code, body)
				}
				erasing.erase(t, f)
				if code, body := call(t, f.server, "POST", c.path, c.body); code != http.StatusOK {
					t.Fatalf("reopening the epic: %d %s", code, body)
				}
				f.reopenedWithoutStories(t)
			})
		}
	}
}

// Two reopens that both read the done epic before either took its erased
// story out, as a double drag does, both get past taking it out: to the
// second, the story already gone counts as taken out.
func TestTwoReopensTakeAnErasedStoryOutOnce(t *testing.T) {
	f := newEraseFixture(t)
	if code, body := call(t, f.server, "POST", "/api/tasks/task-3/direct", `{"to":"done"}`); code != http.StatusOK {
		t.Fatalf("closing the epic: %d %s", code, body)
	}
	if _, rejection, err := f.log.Erase("task-2", you, "Gone.", f.blobs); rejection != nil || err != nil {
		t.Fatalf("erasing task-2 in the store: %v, %v", rejection, err)
	}
	read, err := f.state.Task("task-3") // what both requests read
	if err != nil {
		t.Fatal(err)
	}
	for i := range 2 {
		w := httptest.NewRecorder()
		if !f.web.dropErasedStories(w, read) {
			t.Fatalf("reopen %d: %d %s", i+1, w.Code, w.Body)
		}
	}
	if got := textIn(t, f, "task-3", events.StoryRemoved, "story"); !slices.Equal(got, []string{"task-2"}) {
		t.Errorf("the epic's task.story_removed: %q, want task-2's once", got)
	}
}

// reopenedWithoutStories fails the test unless task-3 is open again with
// no stories, and took task-2 out before it was reopened.
func (f *eraseFixture) reopenedWithoutStories(t *testing.T) {
	t.Helper()
	epic, err := f.state.Task("task-3")
	if err != nil || epic == nil || epic.IsDone {
		t.Fatalf("the epic should be open again: %v, %v", epic, err)
	}
	if len(epic.Stories) != 0 {
		t.Errorf("the epic's stories %v, want none", epic.Stories)
	}
	live, err := f.log.Log("task-3")
	if err != nil {
		t.Fatal(err)
	}
	removed := slices.IndexFunc(live, func(e events.Event) bool { return e.Type == events.StoryRemoved && e.Text("story") == "task-2" })
	reopened := slices.IndexFunc(live, func(e events.Event) bool { return e.Type == events.TaskReopened })
	if removed < 0 || removed > reopened {
		t.Errorf("task.story_removed at %d, task.reopened at %d: want the removal first", removed, reopened)
	}
	if summary, _ := f.state.Summary("task-3"); summary.Stories != 0 {
		t.Errorf("the card counts %d stories, want none", summary.Stories)
	}
}

// A story of an epic you closed, not archived, is erased: the done epic
// lets go of it, and lists no story after.
func TestErasingAStoryOfAClosedEpic(t *testing.T) {
	f := newEraseFixture(t)
	if code, body := call(t, f.server, "POST", "/api/tasks/task-3/direct", `{"to":"done"}`); code != http.StatusOK {
		t.Fatalf("closing the epic: %d %s", code, body)
	}
	f.erase(t, "task-2")
	if epic, err := f.state.Task("task-3"); err != nil || !epic.IsDone || len(epic.Stories) != 0 {
		t.Errorf("the epic should be done with no stories: %v, %v", epic, err)
	}
}

// An epic that still lists an erased story, as when the erasure got past
// the epic some other way, closes from the page: there is nothing of the
// story left to park.
func TestClosingAnEpicSkipsAnErasedStory(t *testing.T) {
	f := newEraseFixture(t)
	if _, rejection, err := f.log.Erase("task-2", you, "Gone.", f.blobs); rejection != nil || err != nil {
		t.Fatalf("erasing task-2 in the store: %v, %v", rejection, err)
	}
	if epic, err := f.state.Task("task-3"); err != nil || !slices.Contains(epic.Stories, "task-2") {
		t.Fatalf("the epic should still list the erased story: %v", err)
	}
	if code, body := call(t, f.server, "POST", "/api/tasks/task-3/direct", `{"to":"done"}`); code != http.StatusOK {
		t.Errorf("closing the epic: %d %s, want 200", code, body)
	}
}

// A story an erasure took out of its epic before it was refused can be
// filed again, out of any epic or back under the same one. The erasure
// leaves it out on both sides; one from before the story wrote its
// task.epic_left too, or a story archived when it was taken out and
// unarchived since, leaves only the epic's task.story_removed, and filing
// it lets go of the epic the story still names.
func TestAStoryLeftByARefusedErasureFilesAgain(t *testing.T) {
	for _, c := range []struct {
		name  string
		leave func(t *testing.T, f *eraseFixture)
	}{
		{"out on both sides", func(t *testing.T, f *eraseFixture) {
			if refused, err := f.web.leaveEpic("task-2"); refused != "" || err != nil {
				t.Fatalf("leaving the epic: %q, %v", refused, err)
			}
			f.filed(t, "", nil)
		}},
		{"out of the epic alone", func(t *testing.T, f *eraseFixture) {
			f.propose(t, events.Proposal{Stream: "task-3", Actor: you, Type: events.StoryRemoved, Data: map[string]any{"story": "task-2"}})
		}},
	} {
		for _, epic := range []string{"", "task-3"} {
			t.Run(c.name+", then into "+cmp.Or(epic, "none"), func(t *testing.T) {
				f := newEraseFixture(t)
				c.leave(t, f)
				if code, body := call(t, f.server, "PUT", "/api/tasks/task-2/epic", `{"epic":"`+epic+`"}`); code != http.StatusOK {
					t.Fatalf("filing task-2 into %q: %d %s", epic, code, body)
				}
				f.filed(t, epic, map[string][]string{"": nil, "task-3": {"task-2"}}[epic])
			})
		}
	}
}

// A story of an erased epic leaves it, though the epic can't take its
// task.story_removed: the route refuses to erase an epic with stories, but
// an erasure from before it did left them.
func TestAStoryLeavesAnErasedEpic(t *testing.T) {
	f := newEraseFixture(t)
	if _, rejection, err := f.log.Erase("task-3", you, "Gone.", f.blobs); rejection != nil || err != nil {
		t.Fatalf("erasing task-3 in the store: %v, %v", rejection, err)
	}
	if code, body := call(t, f.server, "PUT", "/api/tasks/task-2/epic", `{"epic":""}`); code != http.StatusOK {
		t.Fatalf("taking task-2 out: %d %s", code, body)
	}
	if story, err := f.state.Task("task-2"); err != nil || story.Epic != "" {
		t.Errorf("task-2's epic: %v, want none", err)
	}
}

// An epic isn't erased while a story still names it, as the story would
// then point at a task that is gone: you erase the stories, or move them
// out, first. An archived epic is erased while its stories are archived
// too. A refusal writes nothing.
func TestAnEpicWithStoriesIsntErased(t *testing.T) {
	refused := "task-3 has stories (task-2); erase or move them out of the epic first"
	for _, c := range []struct {
		name   string
		before func(t *testing.T, f *eraseFixture)
		want   string // the refusal, or "" for an erasure
	}{
		{"a live story", func(*testing.T, *eraseFixture) {}, refused},
		{"an archived story", func(t *testing.T, f *eraseFixture) {
			f.done(t)
			storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs)
		}, refused},
		{"an archived epic's live story", func(t *testing.T, f *eraseFixture) {
			storetest.ArchiveIn(t, f.log, f.state, "task-3", f.blobs)
		}, refused},
		{"an archived epic's archived story", func(t *testing.T, f *eraseFixture) {
			f.done(t)
			storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs)
			storetest.ArchiveIn(t, f.log, f.state, "task-3", f.blobs)
		}, ""},
		{"a story moved out", func(t *testing.T, f *eraseFixture) {
			if code, body := call(t, f.server, "PUT", "/api/tasks/task-2/epic", `{"epic":""}`); code != http.StatusOK {
				t.Fatalf("taking task-2 out: %d %s", code, body)
			}
		}, ""},
		{"an erased story", func(t *testing.T, f *eraseFixture) { f.erase(t, "task-2") }, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := newEraseFixture(t)
			c.before(t, f)
			last := f.log.LastPosition()
			code, body := call(t, f.server, "POST", "/api/tasks/task-3/erase", `{"why":"Gone."}`)
			switch {
			case c.want == "" && code != http.StatusOK:
				t.Errorf("erasing the epic: %d %s, want 200", code, body)
			case c.want != "" && (code != http.StatusConflict || strings.TrimSpace(body) != c.want):
				t.Errorf("erasing the epic: %d %s, want 409 %q", code, body, c.want)
			case c.want != "" && f.log.LastPosition() != last:
				t.Errorf("the refusal wrote up to %d from %d", f.log.LastPosition(), last)
			}
		})
	}
}

// filed fails the test unless task-2 names epic, and task-3 lists stories.
func (f *eraseFixture) filed(t *testing.T, epic string, stories []string) {
	t.Helper()
	story, err := f.state.Task("task-2")
	if err != nil {
		t.Fatal(err)
	}
	if story.Epic != epic {
		t.Errorf("task-2's epic: %q, want %q", story.Epic, epic)
	}
	task, err := f.state.Task("task-3")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(task.Stories, stories) {
		t.Errorf("task-3's stories: %v, want %v", task.Stories, stories)
	}
}
