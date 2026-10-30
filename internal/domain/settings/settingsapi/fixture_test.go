package settingsapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy/hooks"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

// The redaction tests' fixture: what of web's erase fixture they use, each
// piece mirroring internal/web/erase_test.go's (or the file named).

// plantedHere is in task-2's words and its own shared file, and, as its
// title, in the copy its epic keeps.
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
	hooks     *hooks.Handler // ends turns, as their hooks' calls do
	both      string         // the blob task-1 and task-2 share
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
	f.hooks = hooks.NewHandler(f.log, f.state, nil)
	f.server = httptest.NewServer(web.NewServer(f.log, f.state, web.Config{UIToken: token, DeskToken: "desk-secret", DefaultRepo: "/r",
		Blobs: f.blobs, Snapshots: f.snapshots, Redactions: f.hooks.Redactions(), Extensions: storetest.Installed()}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(f.server.Close)

	own := f.put(t, "task-2's own notes, "+plantedHere)
	f.both = f.put(t, "notes both tasks share")
	f.propose(t,
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Asker", "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-1", Actor: you, Type: events.FileAdded, Data: map[string]any{"name": "shared.md", "hash": f.both, "size": 21, "by": "you"}},
		events.Proposal{Stream: "task-2", Actor: lead, Type: events.TaskCreated, Data: map[string]any{"title": "Asked " + plantedHere, "repo": "/r",
			"coordinator": "tech-lead", "parent": "task-1"}},
		events.Proposal{Stream: "task-1", Actor: lead, Type: events.TaskRequested, Data: map[string]any{"child": "task-2", "team": "Software",
			"request": "Asked", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.FileAdded, Data: map[string]any{"name": "mine.md", "hash": own, "size": 40, "by": "you"}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.FileAdded, Data: map[string]any{"name": "shared.md", "hash": f.both, "size": 21, "by": "you"}},
	)
	if code, body := call(t, f.server, "POST", "/api/tasks", `{"title":"Ship it","stories":["task-2"]}`); code != http.StatusOK {
		t.Fatalf("creating the epic: %d %s", code, body)
	}
	return f
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

// erased is what of an erasure's answer the tests read: when it was.
type erased struct {
	At time.Time `json:"at"`
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

// callAs is call with bearer as the credential, none when it is "", as
// internal/web/extensions_api_test.go's.
func callAs(t *testing.T, server *httptest.Server, bearer, method, path string) (int, string) {
	t.Helper()
	request, _ := http.NewRequest(method, server.URL+path, strings.NewReader("{}"))
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(content)
}

// newTestServerChecking is a server whose log also runs check after the
// rules, so a test can refuse one proposal, as
// internal/web/team_edit_api_test.go's.
func newTestServerChecking(t *testing.T, check func(events.Proposal) *events.Rejection) (*httptest.Server, *store.Store) {
	t.Helper()
	server, s, _ := newTestStateChecking(t, check)
	return server, s
}

// newTestStateChecking is newTestServerChecking and the state its log
// keeps, for a test that archives a task in it, as
// internal/web/team_edit_api_test.go's.
func newTestStateChecking(t *testing.T, check func(events.Proposal) *events.Rejection) (*httptest.Server, *store.Store, *current.State) {
	t.Helper()
	catalogue := events.Core()
	state := storetest.NewState(catalogue)
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), catalogue, state, func(p events.Proposal) *events.Rejection {
		if rejection := rules.Check(p, state, catalogue, storetest.Installed()...); rejection != nil {
			return rejection
		}
		return check(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: token, DefaultRepo: "/src/app", Clones: filepath.Join(t.TempDir(), "repos"),
		Blobs: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return server, s, state
}
