package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// countedBlobs is the server's blobs with the blobs opened counted.
type countedBlobs struct {
	sharedfiles.Disk
	opens atomic.Int32
}

func (c *countedBlobs) Open(hash string) (io.ReadCloser, error) {
	c.opens.Add(1)
	return c.Disk.Open(hash)
}

// archiveFixture is a server whose blobs are counted, with task-2 a story
// of the epic task-3 that waited for your answer, took your answer and a
// refusal, and was moved to Done; task-1 is open.
type archiveFixture struct {
	log    *store.Store
	state  *current.State
	blobs  *countedBlobs
	server *httptest.Server
}

func newArchiveFixture(t *testing.T) *archiveFixture {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	blobs := &countedBlobs{Disk: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}}
	app := NewServer(s, state, Config{UIToken: token, DefaultRepo: "/r", Blobs: blobs.Disk}, http.NotFoundHandler(), nil)
	app.archives.blobs = blobs
	server := httptest.NewServer(app.Handler())
	t.Cleanup(server.Close)
	f := &archiveFixture{log: s, state: state, blobs: blobs, server: server}
	call(t, server, "POST", "/api/tasks", `{"title":"One"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Two","kind":"story"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Ship it","stories":["task-2"]}`)
	for _, p := range []events.Proposal{
		{Stream: "task-2", Actor: events.Actor{Kind: events.Worker, ID: workers.Local}, Type: events.WorktreeReady,
			Data: map[string]any{"path": "/w/task-2", "branch": "b", "port": 7902}},
		{Stream: "task-2", Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"}, Type: events.TaskNeedsYou,
			Data: map[string]any{"question": "Which layout?"}},
	} {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	call(t, server, "POST", "/api/tasks/task-2/answer", `{"answer":"The wide one"}`)
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-2", Actor: events.Actor{Kind: events.Human, ID: "you"}, Type: events.TaskCreated,
		Data: map[string]any{"title": "Two again", "repo": "/r", "coordinator": "tech-lead"}}); rejection == nil || err != nil {
		t.Fatal("a second task.created should be refused:", rejection, err)
	}
	call(t, server, "POST", "/api/tasks/task-2/direct", `{"to":"done"}`)
	return f
}

// get is a read's status and body, decoded into v when it is 200.
func (f *archiveFixture) get(t *testing.T, path string, v any) int {
	t.Helper()
	code, body := call(t, f.server, "GET", path, "")
	if code == http.StatusOK {
		if err := json.Unmarshal([]byte(body), v); err != nil {
			t.Fatalf("%s: %v in %s", path, err, body)
		}
	}
	return code
}

// detail is a task's detail as JSON, without what only an archived one
// says.
func (f *archiveFixture) detail(t *testing.T, id string) (detail, archived map[string]any) {
	t.Helper()
	if code := f.get(t, "/api/tasks/"+id, &detail); code != http.StatusOK {
		t.Fatalf("%s's detail: %d", id, code)
	}
	archived = map[string]any{"archived": detail["archived"], "archivedAt": detail["archivedAt"]}
	delete(detail, "archived")
	delete(detail, "archivedAt")
	return detail, archived
}

// An archived task's detail is its detail before it was archived, saying
// it is archived and when; its events are as before, then its marker. An
// epic archived after its story still lists it as before.
func TestAnArchivedTaskReadsAsBeforeArchiving(t *testing.T) {
	f := newArchiveFixture(t)
	call(t, f.server, "POST", "/api/tasks/task-3/direct", `{"to":"done"}`)
	epicBefore, _ := f.detail(t, "task-3")
	if stories, _ := epicBefore["storyList"].([]any); len(stories) != 1 {
		t.Fatalf("task-3 should list task-2 before archiving: %v", epicBefore["storyList"])
	}
	before, archived := f.detail(t, "task-2")
	if archived["archived"] != nil || archived["archivedAt"] != nil {
		t.Errorf("a live task says it is archived: %v", archived)
	}
	marker := f.log.LastPosition() + 1
	pages := []string{"", "?limit=2", "?after=3&limit=3", fmt.Sprintf("?before=%d&limit=2", marker), "?type=task.needs_you&type=proposal.rejected"}
	eventsBefore := map[string]eventsPage{}
	for _, page := range pages {
		var read eventsPage
		f.get(t, "/api/tasks/task-2/events"+page, &read)
		eventsBefore[page] = read
	}
	if all := eventsBefore[""].Events; len(all) < 6 || !strings.Contains(fmt.Sprint(all), string(events.ProposalRejected)) {
		t.Fatalf("task-2 should have its events and a rejection before archiving: %+v", all)
	}
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)

	after, archived := f.detail(t, "task-2")
	if !reflect.DeepEqual(after, before) {
		t.Errorf("the archived task's detail:\n%v\nwant as before archiving:\n%v", after, before)
	}
	at, _ := archived["archivedAt"].(string)
	if archived["archived"] != true || at == "" {
		t.Errorf("the archived task says %v, want archived and when", archived)
	}
	for _, page := range pages {
		var read eventsPage
		if code := f.get(t, "/api/tasks/task-2/events"+page, &read); code != http.StatusOK {
			t.Fatalf("events%s: %d", page, code)
		}
		want := eventsBefore[page]
		if page == "" {
			want.Events = append(want.Events, eventEntry{})
			if last := read.Events[len(read.Events)-1]; last.Position == marker && last.Type == events.TaskArchived {
				want.Events[len(want.Events)-1] = last
			}
		}
		if !reflect.DeepEqual(read, want) {
			t.Errorf("events%s of the archived task:\n%+v\nwant\n%+v", page, read, want)
		}
	}

	storetest.ArchiveIn(t, f.log, f.state, "task-3", f.blobs.Disk)
	epicAfter, archived := f.detail(t, "task-3")
	if !reflect.DeepEqual(epicAfter, epicBefore) {
		t.Errorf("the archived epic's detail:\n%v\nwant as before archiving:\n%v", epicAfter, epicBefore)
	}
	if archived["archived"] != true {
		t.Errorf("the archived epic says %v, want archived", archived)
	}
}

// A live story whose epic is archived still names its epic.
func TestAStoryOfAnArchivedEpicKeepsItsTitle(t *testing.T) {
	f := newArchiveFixture(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-3", f.blobs.Disk)
	story, _ := f.detail(t, "task-2")
	if story["epicTitle"] != "Ship it" {
		t.Errorf("the story's epicTitle is %q, want the archived epic's %q", story["epicTitle"], "Ship it")
	}
}

// A second read of an archived task, its detail or its events, opens no
// blob; another archived task's opens its own.
func TestASecondReadOfAnArchivedTaskIsCached(t *testing.T) {
	f := newArchiveFixture(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
	storetest.ArchiveIn(t, f.log, f.state, "task-1", f.blobs.Disk)
	var read any
	for _, path := range []string{"/api/tasks/task-2", "/api/tasks/task-2", "/api/tasks/task-2/events", "/api/tasks/task-2/events?limit=1"} {
		if code := f.get(t, path, &read); code != http.StatusOK || f.blobs.opens.Load() != 1 {
			t.Errorf("%s: %d, %d blobs opened; want 200 and task-2's opened once", path, code, f.blobs.opens.Load())
		}
	}
	if code := f.get(t, "/api/tasks/task-1", &read); code != http.StatusOK || f.blobs.opens.Load() != 2 {
		t.Errorf("task-1: %d, %d blobs opened; want 200 and task-1's opened too", code, f.blobs.opens.Load())
	}
}

// A second read of an archived task neither gunzips nor folds again: it
// is handed the task the first read folded.
func TestASecondReadOfAnArchivedTaskFoldsOnce(t *testing.T) {
	f := newArchiveFixture(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
	a := &archives{blobs: f.blobs}
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
	a := &archives{blobs: f.blobs}
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

// Reads of archived tasks at once each get their task.
func TestArchivedTasksReadAtOnce(t *testing.T) {
	f := newArchiveFixture(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
	storetest.ArchiveIn(t, f.log, f.state, "task-1", f.blobs.Disk)
	var wg sync.WaitGroup
	for i := range 16 {
		id := []string{"task-1", "task-2"}[i%2]
		wg.Go(func() {
			code, body := call(t, f.server, "GET", "/api/tasks/"+id, "")
			if code != http.StatusOK || !strings.Contains(body, `"id":"`+id+`"`) {
				t.Errorf("%s: %d %s", id, code, body)
			}
		})
	}
	wg.Wait()
}

// An archived task whose blob is missing or isn't an archive can't be
// read: 500, saying which task. A task that never was is still 404.
func TestAnArchivedTaskWithoutItsBlobFails(t *testing.T) {
	f := newArchiveFixture(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
	live, err := f.log.Log("task-2")
	if err != nil {
		t.Fatal(err)
	}
	hash := live[0].Text("archive")
	path := filepath.Join(f.blobs.Dir, hash[:2], hash)
	if err := os.WriteFile(path, []byte("not gzip"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, read := range []string{"/api/tasks/task-2", "/api/tasks/task-2/events"} {
		if code, body := call(t, f.server, "GET", read, ""); code != http.StatusInternalServerError || !strings.Contains(body, "task-2") {
			t.Errorf("%s with a corrupt blob: %d %s", read, code, body)
		}
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if code, body := call(t, f.server, "GET", "/api/tasks/task-2", ""); code != http.StatusInternalServerError || !strings.Contains(body, "task-2") {
		t.Errorf("with the blob missing: %d %s", code, body)
	}
	if code, _ := call(t, f.server, "GET", "/api/tasks/task-9", ""); code != http.StatusNotFound {
		t.Errorf("a task that never was: %d, want 404", code)
	}
}

// Reading an archived task doesn't make it take events: the routes that
// act on a task still refuse it, reopening too, until it is unarchived
// (step 9). Moving it answers 404, as it did before it could be read.
func TestAnArchivedTaskReadBackStillTakesNoEvents(t *testing.T) {
	f := newArchiveFixture(t)
	storetest.ArchiveIn(t, f.log, f.state, "task-2", f.blobs.Disk)
	var read any
	if code := f.get(t, "/api/tasks/task-2", &read); code != http.StatusOK {
		t.Fatalf("task-2's detail: %d", code)
	}
	for _, request := range []struct{ method, path, body string }{
		{"POST", "/api/tasks/task-2/answer", `{"answer":"Again"}`},
		{"POST", "/api/tasks/task-2/direct", `{"to":"todo"}`},
		{"PUT", "/api/tasks/task-2/labels", `{"labels":["ui"]}`},
		{"PUT", "/api/tasks/task-2/rank", `{"rank":1}`},
	} {
		if code, body := call(t, f.server, request.method, request.path, request.body); code == http.StatusOK || code == http.StatusInternalServerError {
			t.Errorf("%s %s on the archived task: %d %s", request.method, request.path, code, body)
		}
	}
	if _, err := f.state.Task("task-2"); !errors.Is(err, current.ErrArchived) {
		t.Errorf("task-2 in the state: %v, want still archived", err)
	}
}
