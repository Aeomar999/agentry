package web

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

const token = "ui-secret"

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	return newTestServerWith(t, tasks.Limits{})
}

// newTestServerWith is newTestServer with the new tasks' limits in limits.
func newTestServerWith(t *testing.T, limits tasks.Limits) (*httptest.Server, *store.Store) {
	t.Helper()
	return newTestServerOn(t, filepath.Join(t.TempDir(), "agentry.db"), limits)
}

// newTestServerOn is newTestServerWith on the log at path.
func newTestServerOn(t *testing.T, path string, limits tasks.Limits) (*httptest.Server, *store.Store) {
	t.Helper()
	s, state := storetest.Open(t, path)
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	hooks := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hooks")) })
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, DefaultRepo: "/src/app", Clones: filepath.Join(t.TempDir(), "repos"),
		Blobs: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}, Budget: limits.Budget, MaxTurns: limits.MaxTurns}, hooks, nil).Handler())
	t.Cleanup(server.Close)
	return server, s
}

// newTestServerChecking is newTestServer whose log also runs check after
// the rules, so a test can refuse one proposal.
func newTestServerChecking(t *testing.T, check func(events.Proposal) *events.Rejection) (*httptest.Server, *store.Store) {
	t.Helper()
	server, s, _ := newTestStateChecking(t, check)
	return server, s
}

// newTestStateChecking is newTestServerChecking and the state its log
// keeps, for a test that archives a task in it.
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
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, DefaultRepo: "/src/app", Clones: filepath.Join(t.TempDir(), "repos"),
		Blobs: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return server, s, state
}

func call(t *testing.T, server *httptest.Server, method, path, body string) (int, string) {
	t.Helper()
	request, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(content)
}

func TestTheAPINeedsTheUIToken(t *testing.T) {
	server, _ := newTestServer(t)
	for _, auth := range []string{"", "Bearer wrong", "Bearer hook-token"} {
		request, _ := http.NewRequest("GET", server.URL+"/api/tasks", nil)
		request.Header.Set("Authorization", auth)
		response, _ := http.DefaultClient.Do(request)
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Errorf("auth %q: %d", auth, response.StatusCode)
		}
	}

	// The terminal's websocket carries the token in the address.
	if response, _ := http.Get(server.URL + "/api/tasks?token=" + token); response.StatusCode != 200 {
		t.Errorf("with ?token=: %d", response.StatusCode)
	}

	// Opening the printed link sets the cookie, which then opens the API.
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	if response, err := client.Get(server.URL + "/?token=" + token); err != nil || response.StatusCode != 200 {
		t.Fatalf("open the link: %v %v", response, err)
	}
	if response, _ := client.Get(server.URL + "/api/tasks"); response.StatusCode != 200 {
		t.Errorf("with the cookie: %d", response.StatusCode)
	}
}

func TestChangesAnswerAsSoonAsTheLogMoves(t *testing.T) {
	server, s := newTestServer(t)
	start := s.LastPosition()
	go func() {
		time.Sleep(100 * time.Millisecond)
		s.Propose(events.Proposal{Stream: "task-1", Actor: httpapi.You, Type: events.TaskCreated,
			Data: map[string]any{"title": "t", "repo": "/r", "coordinator": "tech-lead"}})
	}()
	began := time.Now()
	_, body := call(t, server, "GET", "/api/changes?after="+itoa(start), "")
	if elapsed := time.Since(began); elapsed > 5*time.Second || !strings.Contains(body, `"position":`+itoa(start+1)) {
		t.Errorf("after %s: %s", elapsed, body)
	}
}

func TestAPIReadsSayThePositionTheyWereReadAt(t *testing.T) {
	server, s := newTestServer(t)
	for _, title := range []string{"one", "two"} {
		call(t, server, "POST", "/api/tasks", `{"title":"`+title+`"}`)
	}
	at := itoa(s.LastPosition())
	for _, path := range []string{"/api/tasks", "/api/tasks/task-1", "/api/tasks/task-1/events", "/api/questions", "/api/agents", "/api/config", "/api/policy", "/api/settings", "/api/features", "/api/tasks/task-9"} {
		if got := positionOf(t, server, "GET", path); got != at {
			t.Errorf("GET %s: position %q, want %s", path, got, at)
		}
	}
	for _, request := range [][2]string{{"POST", "/api/tasks"}, {"GET", "/"}} {
		if got := positionOf(t, server, request[0], request[1]); got != "" {
			t.Errorf("%s %s: position %q, want none", request[0], request[1], got)
		}
	}
}

func TestThePositionIsTakenBeforeTheRead(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	server := NewServer(s, state, Config{UIToken: token}, http.NotFoundHandler(), nil)
	before := s.LastPosition()
	read := server.atPosition(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		s.Propose(events.Proposal{Stream: "task-1", Actor: httpapi.You, Type: events.TaskCreated,
			Data: map[string]any{"title": "t", "repo": "/r", "coordinator": "tech-lead"}})
	}))
	recorder := httptest.NewRecorder()
	read.ServeHTTP(recorder, httptest.NewRequest("GET", "/api/tasks", nil))
	if s.LastPosition() == before {
		t.Fatal("the read proposed nothing")
	}
	if got := recorder.Header().Get(positionHeader); got != itoa(before) {
		t.Errorf("position %q, want %d, the one before the read", got, before)
	}
}

func positionOf(t *testing.T, server *httptest.Server, method, path string) string {
	t.Helper()
	request, _ := http.NewRequest(method, server.URL+path, strings.NewReader(`{"title":"x"}`))
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	return response.Header.Get(positionHeader)
}

func TestHookCallsGoToTheHookHandler(t *testing.T) {
	server, _ := newTestServer(t)
	response, err := http.Post(server.URL+"/hooks", "application/json", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if content, _ := io.ReadAll(response.Body); string(content) != "hooks" {
		t.Errorf("got %q", content)
	}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

func TestThePolicyShowsWithItsProblem(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	path := filepath.Join(t.TempDir(), "policy.toml")
	live, err := policy.OpenLive(path, "/home/dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("version = 2"), 0o600)
	live.Reload()
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, Policy: live}, http.NotFoundHandler(), nil).Handler())
	defer server.Close()

	_, body := call(t, server, "GET", "/api/policy", "")
	var shown struct {
		Path, Hash, Problem, UseForEveryone string
		Profiles                            []policy.ProfileSummary
	}
	json.Unmarshal([]byte(body), &shown)
	if shown.Path != path || shown.Hash != live.Current().Hash() || !strings.Contains(shown.Problem, "version must be 1") ||
		len(shown.Profiles) != 6 || shown.UseForEveryone != "allow-everything" {
		t.Errorf("policy: %+v", shown)
	}
	if _, body := call(t, server, "GET", "/api/config", ""); !strings.Contains(body, `"policyProblem":true`) {
		t.Errorf("config: %s", body)
	}
	os.WriteFile(path, policy.Default(), 0o600)
	live.Reload()
	if _, body := call(t, server, "GET", "/api/config", ""); !strings.Contains(body, `"policyProblem":false`) {
		t.Errorf("config once the file is fixed: %s", body)
	}
}

// needsTheUI skips a test that reads the built page when there is none: it
// isn't committed, so a fresh clone has it only after `make ui`.
func needsTheUI(t *testing.T) {
	t.Helper()
	if _, isBuilt := built(); !isBuilt {
		t.Skip("the UI isn't built: run `make ui`")
	}
}

func TestServesTheUI(t *testing.T) {
	needsTheUI(t)
	server, _ := newTestServer(t)
	get := func(path string) (*http.Response, string) {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		content, _ := io.ReadAll(response.Body)
		return response, string(content)
	}

	// The UI routes in the browser, so each of its pages is index.html,
	// which needs no token: the data behind it does.
	for _, path := range []string{"/", "/tasks", "/tasks/task-1?tab=events", "/policy"} {
		if response, body := get(path); response.StatusCode != 200 || !strings.Contains(body, `<div id="root">`) {
			t.Errorf("%s: %d, not the UI", path, response.StatusCode)
		}
	}

	// The bundles index.html names are served, and may be kept for good.
	_, index := get("/")
	start := strings.Index(index, "/assets/")
	if start < 0 {
		t.Fatal("index.html names no bundle")
	}
	bundle := index[start : start+strings.IndexByte(index[start:], '"')]
	if response, _ := get(bundle); response.StatusCode != 200 || !strings.Contains(response.Header.Get("Cache-Control"), "immutable") {
		t.Errorf("%s: %d %q", bundle, response.StatusCode, response.Header.Get("Cache-Control"))
	}

	// An API path that doesn't exist is not a page.
	if code, _ := call(t, server, "GET", "/api/nothing", ""); code != http.StatusNotFound {
		t.Errorf("/api/nothing: %d", code)
	}
}

// The browser fetches what installs the page as an app without the token:
// the manifest, the service worker and the icons, at the root, with the
// types it checks. The API still needs the token.
func TestServesTheAppFilesWithoutTheToken(t *testing.T) {
	needsTheUI(t)
	server, _ := newTestServer(t)
	get := func(path string) (*http.Response, string) {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		content, _ := io.ReadAll(response.Body)
		return response, string(content)
	}

	response, body := get("/manifest.webmanifest")
	var manifest struct {
		StartURL string `json:"start_url"`
		Display  string
		Icons    []struct{ Src, Purpose string }
	}
	if response.StatusCode != 200 || response.Header.Get("Content-Type") != "application/manifest+json" || json.Unmarshal([]byte(body), &manifest) != nil {
		t.Fatalf("manifest: %d %q", response.StatusCode, response.Header.Get("Content-Type"))
	}
	if manifest.StartURL != "/" || manifest.Display != "standalone" || len(manifest.Icons) != 3 {
		t.Errorf("manifest: %+v", manifest)
	}
	files := map[string]string{"/sw.js": "text/javascript", "/icons/apple-touch-icon.png": "image/png", "/icons/icon.svg": "image/svg+xml"}
	for _, icon := range manifest.Icons {
		files[icon.Src] = "image/png"
	}
	for path, kind := range files {
		if response, _ := get(path); response.StatusCode != 200 || !strings.HasPrefix(response.Header.Get("Content-Type"), kind) {
			t.Errorf("%s: %d %q", path, response.StatusCode, response.Header.Get("Content-Type"))
		}
	}
	if response, _ := get("/sw.js"); response.Header.Get("Cache-Control") != "no-cache" {
		t.Errorf("sw.js may be kept: %q", response.Header.Get("Cache-Control"))
	}
	_, index := get("/")
	for _, link := range []string{`rel="manifest" href="/manifest.webmanifest"`, `rel="apple-touch-icon"`, `name="theme-color"`} {
		if !strings.Contains(index, link) {
			t.Errorf("index.html has no %s", link)
		}
	}

	for _, path := range []string{"/api/config", "/api/tasks", "/api/agents"} {
		if response, _ := get(path); response.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s without the token: %d", path, response.StatusCode)
		}
	}
}

// The desk hands work to the coordinator and reads it back, and nothing
// more: approving an agent's action takes the UI token.
func TestTheDeskTokenOpensOnlyTasks(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, DeskToken: "desk-secret"}, http.NotFoundHandler(), nil).Handler())
	defer server.Close()
	asDesk := func(method, path, body string) int {
		request, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer desk-secret")
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		return response.StatusCode
	}

	for _, allowed := range []struct{ method, path, body string }{
		{"POST", "/api/tasks", `{"title":"From the desk"}`},
		{"GET", "/api/tasks", ""},
		{"GET", "/api/tasks/task-1", ""},
	} {
		if code := asDesk(allowed.method, allowed.path, allowed.body); code != 200 {
			t.Errorf("desk %s %s: %d", allowed.method, allowed.path, code)
		}
	}
	for _, refused := range []struct{ method, path string }{
		{"POST", "/api/tasks/task-1/actions/push-1/approve"},
		{"POST", "/api/tasks/task-1/actions/push-1/deny"},
		{"GET", "/api/policy"},
		{"GET", "/api/changes"},
	} {
		if code := asDesk(refused.method, refused.path, `{}`); code != http.StatusUnauthorized {
			t.Errorf("desk %s %s: %d, want 401", refused.method, refused.path, code)
		}
	}
}

var techLead = events.Actor{Kind: events.Agent, ID: "tech-lead"} // tasksapi's fixture_test.go has a copy

// detailOfTask has a copy in tasksapi's fixture_test.go.
func detailOfTask(t *testing.T, server *httptest.Server, id string) taskDetail {
	t.Helper()
	_, body := call(t, server, "GET", "/api/tasks/"+id, "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	return detail
}

// The config sends no flag the page doesn't read: not whether there is a
// terminal or a desk, even on a server that has both.
func TestTheConfigSendsNoTerminalOrDeskFlags(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, Desk: http.NotFoundHandler()}, http.NotFoundHandler(), http.NotFoundHandler()).Handler())
	defer server.Close()
	_, body := call(t, server, "GET", "/api/config", "")
	var config map[string]any
	if err := json.Unmarshal([]byte(body), &config); err != nil {
		t.Fatalf("config: %v %s", err, body)
	}
	for _, key := range []string{"hasTerminal", "hasDesk"} {
		if _, ok := config[key]; ok {
			t.Errorf("config sends %s: %s", key, body)
		}
	}
	if _, ok := config["canAddWorkers"]; !ok {
		t.Errorf("config has no canAddWorkers: %s", body)
	}
}

// The board's columns are the work's stages: a card goes onto any of them,
// and the coordinator still makes the move.
func TestCardsMoveThroughTheStages(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"Fix the login redirect"}`)
	show := func() taskDetail {
		_, body := call(t, server, "GET", "/api/tasks/task-1", "")
		var detail taskDetail
		json.Unmarshal([]byte(body), &detail)
		return detail
	}
	lastStep := func(d taskDetail) string { return d.Trail[len(d.Trail)-1].Text }
	move := func(to, note string) int {
		code, _ := call(t, server, "POST", "/api/tasks/task-1/direct", `{"to":"`+to+`","note":"`+note+`"}`)
		return code
	}

	move("backlog", "")
	if d := show(); d.Status != "held" || d.Parked != "backlog" {
		t.Errorf("onto Backlog: %s, parked %q", d.Status, d.Parked)
	}
	move("todo", "")
	if d := show(); d.Parked != "todo" {
		t.Errorf("onto To do: parked %q", d.Parked)
	}
	steps := len(show().Trail)
	move("in-progress", "")
	if d := show(); d.Parked != "" || d.Status != "working" || len(d.Trail) != steps {
		t.Errorf("started: parked %q, %s, %d steps (want %d: nothing to say)", d.Parked, d.Status, len(d.Trail), steps)
	}
	move("review", "Check the redirect on Safari.")
	if d := show(); lastStep(d) != "Have this task reviewed next. Check the redirect on Safari." {
		t.Errorf("onto Review: %q", lastStep(d))
	}
	if code := move("sideways", ""); code != http.StatusBadRequest {
		t.Errorf("a column that doesn't exist: %d", code)
	}

	// Done, then back for more work.
	worker, coordinator := events.Actor{Kind: events.Worker, ID: "laptop"}, events.Actor{Kind: events.Agent, ID: "tech-lead"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "report": "r"}},
		{Stream: "task-1", Actor: coordinator, Type: events.TaskDone, Data: map[string]any{"summary": "Fixed."}},
	} {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatalf("%s: %v %v", p.Type, err, rejection)
		}
	}
	if code := move("done", ""); code != http.StatusConflict {
		t.Errorf("a done task onto Done: %d", code)
	}
	move("in-progress", "The fix broke sign-out.")
	if d := show(); d.Status != "working" || d.Worktree != nil || lastStep(d) != "Reopen this task. This needs more work: hand it back to an agent that builds it. The fix broke sign-out." {
		t.Errorf("reopened: %s, worktree %v, %q", d.Status, d.Worktree, lastStep(d))
	}
}

// Your moves to Done, To do and the Backlog stand in the same request: the
// turn in progress stops, and the coordinator only wraps up.
func TestYourMovesOnTheBoardStand(t *testing.T) {
	server, s := newTestServer(t)
	worker := events.Actor{Kind: events.Worker, ID: "laptop"}
	working := func(id string) {
		for _, p := range []events.Proposal{
			{Stream: id, Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/" + id, "branch": "b", "port": 7901}},
			{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		} {
			if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
				t.Fatalf("%s: %v %v", p.Type, err, rejection)
			}
		}
	}
	move := func(id, to string) {
		if code, body := call(t, server, "POST", "/api/tasks/"+id+"/direct", `{"to":"`+to+`","note":"Enough."}`); code != 200 {
			t.Fatalf("%s onto %s: %d %s", id, to, code, body)
		}
	}
	load := func(id string) *tasks.Task {
		stream, _ := s.Stream(id)
		return tasks.FromEvents(stream)
	}

	call(t, server, "POST", "/api/tasks", `{"description":"Fix the login redirect"}`)
	working("task-1")
	move("task-1", "done")
	if task := load("task-1"); task.Status() != tasks.Done || !task.DoneByYou || !task.Turns[0].Stopped || task.WrapUp == nil {
		t.Errorf("onto Done: %s, by you %v, turn stopped %v, wrap-up %+v", task.Status(), task.DoneByYou, task.Turns[0].Stopped, task.WrapUp)
	}

	call(t, server, "POST", "/api/tasks", `{"description":"Tidy the README"}`)
	working("task-2")
	move("task-2", "todo")
	if task := load("task-2"); task.Status() != tasks.Held || task.Parked != "todo" || !task.Turns[0].Stopped || task.WrapUp == nil {
		t.Errorf("onto To do: %s, parked %q, turn stopped %v, wrap-up %+v", task.Status(), task.Parked, task.Turns[0].Stopped, task.WrapUp)
	}

	call(t, server, "POST", "/api/tasks", `{"title":"Ship it","kind":"epic"}`)
	move("task-3", "done")
	if task := load("task-3"); !task.IsDone {
		t.Errorf("an epic onto Done should close: %s", task.Status())
	}

	// An epic onto Done parks its open stories, in the same request.
	call(t, server, "POST", "/api/tasks", `{"title":"Add the store","kind":"story"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Add the command","kind":"story"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Ship the tool","stories":["task-4","task-5"]}`)
	working("task-4")
	move("task-6", "done")
	if task := load("task-6"); !task.IsDone {
		t.Errorf("the epic should close: %s", task.Status())
	}
	if task := load("task-4"); task.Parked != "backlog" || !task.Turns[0].Stopped || task.WrapUp == nil {
		t.Errorf("the story at work: parked %q, turn stopped %v, wrap-up %+v", task.Parked, task.Turns[0].Stopped, task.WrapUp)
	}
	if task := load("task-5"); task.Parked == "" || task.WrapUp != nil {
		t.Errorf("the story not started: parked %q, wrap-up %+v", task.Parked, task.WrapUp)
	}
}

// Your moves to In progress and Review stand in the same request, from
// any column, until the coordinator hands the task on: then its column
// follows the agent that has it.
func TestYourMovesToInProgressAndReviewStand(t *testing.T) {
	server, s := newTestServer(t)
	worker, lead := events.Actor{Kind: events.Worker, ID: "laptop"}, events.Actor{Kind: events.Agent, ID: "tech-lead"}
	propose := func(p events.Proposal) *events.Rejection {
		_, rejection, err := s.Propose(p)
		if err != nil {
			t.Fatal(err)
		}
		return rejection
	}
	handOn := func(id, turn, agent string) *events.Rejection {
		for _, p := range []events.Proposal{
			{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
			{Stream: id, Actor: worker, Type: events.TurnEnded, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "report": "r"}},
		} {
			if rejection := propose(p); rejection != nil {
				t.Fatalf("%s: %v", p.Type, rejection)
			}
		}
		return propose(events.Proposal{Stream: id, Actor: lead, Type: events.TaskAssigned,
			Data: map[string]any{"agent": agent, "instructions": "go", "checks": agent == "reviewer"}})
	}
	move := func(id, to string) {
		if code, body := call(t, server, "POST", "/api/tasks/"+id+"/direct", `{"to":"`+to+`"}`); code != 200 {
			t.Fatalf("%s onto %s: %d %s", id, to, code, body)
		}
	}
	column := func(id string) string {
		_, body := call(t, server, "GET", "/api/tasks", "")
		var list []taskSummary
		json.Unmarshal([]byte(body), &list)
		for _, summary := range list {
			if summary.ID == id {
				return summary.Column
			}
		}
		return "missing"
	}

	call(t, server, "POST", "/api/tasks", `{"description":"Fix the login redirect"}`)
	propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}})
	if rejection := handOn("task-1", "turn-1", "engineer"); rejection != nil || column("task-1") != "in-progress" {
		t.Fatalf("with the engineer: %v, column %s", rejection, column("task-1"))
	}
	move("task-1", "review")
	if c := column("task-1"); c != "review" {
		t.Errorf("onto Review: column %s", c)
	}
	if rejection := handOn("task-1", "turn-2", "engineer"); rejection == nil ||
		rejection.Reason != "the user moved this task to Review; hand it to an agent that checks work" {
		t.Errorf("handed back to the engineer: %v", rejection)
	}
	if rejection := propose(events.Proposal{Stream: "task-1", Actor: lead, Type: events.TaskAssigned,
		Data: map[string]any{"agent": "reviewer", "instructions": "check it", "checks": true}}); rejection != nil || column("task-1") != "review" {
		t.Errorf("to the reviewer: %v, column %s", rejection, column("task-1"))
	}
	if rejection := handOn("task-1", "turn-3", "engineer"); rejection != nil || column("task-1") != "in-progress" {
		t.Errorf("your move is over: the next handoff is the coordinator's: %v, column %s", rejection, column("task-1"))
	}
	handOn("task-1", "turn-4", "reviewer")
	move("task-1", "in-progress")
	if c := column("task-1"); c != "in-progress" {
		t.Errorf("sent back from Review: column %s", c)
	}
	if rejection := handOn("task-1", "turn-5", "tester"); rejection == nil ||
		rejection.Reason != "the user moved this task back to In progress; hand it to an agent that builds it" {
		t.Errorf("handed to the tester: %v", rejection)
	}
	if rejection := propose(events.Proposal{Stream: "task-1", Actor: lead, Type: events.TaskAssigned,
		Data: map[string]any{"agent": "engineer", "instructions": "fix it"}}); rejection != nil || column("task-1") != "in-progress" {
		t.Errorf("to the engineer: %v, column %s", rejection, column("task-1"))
	}

	// From Done: reopened, in the column you chose.
	call(t, server, "POST", "/api/tasks", `{"description":"Tidy the README"}`)
	propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w2", "branch": "b", "port": 7902}})
	handOn("task-2", "turn-1", "reviewer")
	propose(events.Proposal{Stream: "task-2", Actor: lead, Type: events.TaskDone, Data: map[string]any{"summary": "Tidied."}})
	move("task-2", "in-progress")
	if c := column("task-2"); c != "in-progress" {
		t.Errorf("from Done onto In progress: column %s", c)
	}
	move("task-2", "done")
	move("task-2", "review")
	if c := column("task-2"); c != "review" {
		t.Errorf("from Done onto Review: column %s", c)
	}

	// From To do: started, in the column you chose.
	call(t, server, "POST", "/api/tasks", `{"description":"Add a logo"}`)
	move("task-3", "todo")
	move("task-3", "review")
	if c := column("task-3"); c != "review" {
		t.Errorf("from To do onto Review: column %s", c)
	}
	move("task-3", "backlog")
	if c := column("task-3"); c != "backlog" {
		t.Errorf("parked again: column %s", c)
	}
}

// Moving done work, or work a checker had before you parked it, back to
// In progress sends it back for more work: not to a checker again.
func TestBackToInProgressFromDoneOrParkedNeedsMoreWork(t *testing.T) {
	server, s := newTestServer(t)
	worker, lead := events.Actor{Kind: events.Worker, ID: "laptop"}, events.Actor{Kind: events.Agent, ID: "tech-lead"}
	propose := func(p events.Proposal) *events.Rejection {
		_, rejection, err := s.Propose(p)
		if err != nil {
			t.Fatal(err)
		}
		return rejection
	}
	handOn := func(id, turn, agent string) *events.Rejection {
		for _, p := range []events.Proposal{
			{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
			{Stream: id, Actor: worker, Type: events.TurnEnded, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "report": "r"}},
		} {
			if rejection := propose(p); rejection != nil {
				t.Fatalf("%s: %v", p.Type, rejection)
			}
		}
		return propose(events.Proposal{Stream: id, Actor: lead, Type: events.TaskAssigned,
			Data: map[string]any{"agent": agent, "instructions": "go", "checks": agent == "reviewer"}})
	}
	move := func(id, to string) {
		if code, body := call(t, server, "POST", "/api/tasks/"+id+"/direct", `{"to":"`+to+`"}`); code != 200 {
			t.Fatalf("%s onto %s: %d %s", id, to, code, body)
		}
	}
	load := func(id string) *tasks.Task {
		stream, _ := s.Stream(id)
		return tasks.FromEvents(stream)
	}
	const moreWork = "This needs more work: hand it back to an agent that builds it."

	call(t, server, "POST", "/api/tasks", `{"description":"Fix the login redirect"}`)
	propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}})
	handOn("task-1", "turn-1", "engineer")
	propose(events.Proposal{Stream: "task-1", Actor: lead, Type: events.TaskDone, Data: map[string]any{"summary": "Fixed."}})
	move("task-1", "in-progress")
	if task := load("task-1"); !task.SentBack || !strings.Contains(task.Handoff.Text, moreWork) {
		t.Errorf("from Done: sent back %v, handoff %+v", task.SentBack, task.Handoff)
	}
	propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}})
	if rejection := handOn("task-1", "turn-2", "reviewer"); rejection == nil ||
		rejection.Reason != "the user moved this task back to In progress; hand it to an agent that builds it" {
		t.Errorf("reopened, handed to the reviewer: %v", rejection)
	}

	call(t, server, "POST", "/api/tasks", `{"description":"Tidy the README"}`)
	propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w2", "branch": "b", "port": 7902}})
	handOn("task-2", "turn-1", "reviewer")
	move("task-2", "todo")
	move("task-2", "in-progress")
	if task := load("task-2"); !task.SentBack || task.Trail[len(task.Trail)-1].Text != moreWork {
		t.Errorf("parked after review: sent back %v, last step %+v", task.SentBack, task.Trail[len(task.Trail)-1])
	}
}

// A task an agent that ships work has shows in Shipping. Moving a card
// there ships it now: the move stands at once, until the coordinator hands
// it to an agent of its team that ships work; a team with none refuses it.
// Moved out again, it keeps the other columns' meaning.
func TestYourMoveToShippingShipsItNow(t *testing.T) {
	server, s := newTestServer(t)
	worker, lead := events.Actor{Kind: events.Worker, ID: "laptop"}, events.Actor{Kind: events.Agent, ID: "tech-lead"}
	propose := func(p events.Proposal) *events.Rejection {
		_, rejection, err := s.Propose(p)
		if err != nil {
			t.Fatal(err)
		}
		return rejection
	}
	handOn := func(id, turn, agent string) *events.Rejection {
		for _, p := range []events.Proposal{
			{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
			{Stream: id, Actor: worker, Type: events.TurnEnded, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "report": "r"}},
		} {
			if rejection := propose(p); rejection != nil {
				t.Fatalf("%s: %v", p.Type, rejection)
			}
		}
		return propose(events.Proposal{Stream: id, Actor: lead, Type: events.TaskAssigned,
			Data: map[string]any{"agent": agent, "instructions": "go", "checks": agent == "reviewer", "ships": agent == "deployer"}})
	}
	move := func(id, to string) {
		if code, body := call(t, server, "POST", "/api/tasks/"+id+"/direct", `{"to":"`+to+`"}`); code != 200 {
			t.Fatalf("%s onto %s: %d %s", id, to, code, body)
		}
	}
	load := func(id string) *tasks.Task {
		stream, _ := s.Stream(id)
		return tasks.FromEvents(stream)
	}
	summary := func(id string) taskSummary {
		_, body := call(t, server, "GET", "/api/tasks", "")
		var list []taskSummary
		json.Unmarshal([]byte(body), &list)
		for _, summary := range list {
			if summary.ID == id {
				return summary
			}
		}
		return taskSummary{Column: "missing"}
	}
	column := func(id string) string { return summary(id).Column }

	// The coordinator hands it to the deployer: Shipping, with no move of yours.
	call(t, server, "POST", "/api/tasks", `{"description":"Fix the login redirect"}`)
	propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}})
	handOn("task-1", "turn-1", "deployer")
	if c, task := column("task-1"), load("task-1"); c != "shipping" || task.Column != "" {
		t.Errorf("with the deployer: column %s, your column %q", c, task.Column)
	}
	if !summary("task-1").CanShip {
		t.Error("a team with a deployer can ship")
	}

	// Shipped now, from In progress: your move, written at once.
	call(t, server, "POST", "/api/tasks", `{"description":"Tidy the README"}`)
	propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w2", "branch": "b", "port": 7902}})
	handOn("task-2", "turn-1", "engineer")
	before, _ := s.Stream("task-2")
	move("task-2", "shipping")
	stream, _ := s.Stream("task-2")
	first := stream[len(before)]
	if first.Actor.Kind != events.Human || first.Text("column") != "shipping" || column("task-2") != "shipping" {
		t.Errorf("onto Shipping: first event %s by %s with column %q, board column %s", first.Type, first.Actor.Kind, first.Text("column"), column("task-2"))
	}
	if text := load("task-2").Trail[len(load("task-2").Trail)-1].Text; !strings.Contains(text, "hand it to deployer") ||
		!strings.Contains(text, "maybe before it was reviewed or tested") || !strings.Contains(text, "needs no earlier review") || !strings.Contains(text, "a failure stops the push") {
		t.Errorf("the coordinator is told: %q", text)
	}
	if rejection := handOn("task-2", "turn-2", "engineer"); rejection == nil ||
		rejection.Reason != "the user moved this task to Shipping; hand it to an agent that ships work" {
		t.Errorf("handed to the engineer: %v", rejection)
	}
	if rejection := handOn("task-2", "turn-3", "reviewer"); rejection == nil || column("task-2") != "shipping" {
		t.Errorf("handed to the reviewer: %v, column %s", rejection, column("task-2"))
	}
	if rejection := propose(events.Proposal{Stream: "task-2", Actor: lead, Type: events.TaskAssigned,
		Data: map[string]any{"agent": "deployer", "instructions": "ship it", "ships": true}}); rejection != nil {
		t.Fatalf("to the deployer: %v", rejection)
	}
	if task := load("task-2"); column("task-2") != "shipping" || task.Column != "" || !task.Handoff.Ships {
		t.Errorf("with the deployer you asked for: column %s, your column %q, handoff %+v", column("task-2"), task.Column, task.Handoff)
	}
	propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.TurnStarted, Correlation: "turn-4", Data: map[string]any{"agent": "deployer", "hash": "h"}})
	if task := load("task-2"); !task.Turns[len(task.Turns)-1].Shipped {
		t.Errorf("the deployer's turn ships it for you: %+v", task.Turns[len(task.Turns)-1])
	}
	propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.TurnEnded, Correlation: "turn-4", Data: map[string]any{"agent": "deployer", "report": "Shipped."}})

	// Out of Shipping: back to In progress is more work.
	move("task-2", "in-progress")
	if task := load("task-2"); !task.SentBack || task.Column != "in-progress" {
		t.Errorf("sent back from Shipping: sent back %v, column %q", task.SentBack, task.Column)
	}
	if rejection := handOn("task-2", "turn-5", "deployer"); rejection == nil ||
		rejection.Reason != "the user moved this task back to In progress; hand it to an agent that builds it" {
		t.Errorf("sent back, handed to the deployer: %v", rejection)
	}
	handOn("task-2", "turn-6", "engineer")
	move("task-2", "shipping")
	move("task-2", "review")
	if task := load("task-2"); column("task-2") != "review" || task.SentBack {
		t.Errorf("from Shipping onto Review: column %s, sent back %v", column("task-2"), task.SentBack)
	}
	move("task-2", "shipping")
	move("task-2", "done")
	if task := load("task-2"); !task.DoneByYou || task.WrapUp == nil || column("task-2") != "done" {
		t.Errorf("from Shipping onto Done: by you %v, wrap-up %+v, column %s", task.DoneByYou, task.WrapUp, column("task-2"))
	}

	// From Done and from To do: reopened or started, in Shipping.
	move("task-2", "shipping")
	if task := load("task-2"); task.IsDone || column("task-2") != "shipping" {
		t.Errorf("from Done onto Shipping: done %v, column %s", task.IsDone, column("task-2"))
	}
	move("task-2", "todo")
	move("task-2", "shipping")
	if task := load("task-2"); task.Parked != "" || column("task-2") != "shipping" {
		t.Errorf("from To do onto Shipping: parked %q, column %s", task.Parked, column("task-2"))
	}

	// A team with no agent that ships work refuses the move, and writes nothing.
	if code, body := call(t, server, "POST", "/api/teams", `{"template":"research"}`); code != 200 {
		t.Fatalf("adding the Research team: %d %s", code, body)
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Size the market","team":"Research"}`)
	before, _ = s.Stream("task-3")
	code, body := call(t, server, "POST", "/api/tasks/task-3/direct", `{"to":"shipping"}`)
	after, _ := s.Stream("task-3")
	if code != http.StatusConflict || !strings.Contains(body, "Research has no agent that ships work") || len(after) != len(before) {
		t.Errorf("a team with no shipper: %d %q, %d events written", code, body, len(after)-len(before))
	}
	if _, body := call(t, server, "GET", "/api/tasks", ""); summary("task-3").CanShip || !strings.Contains(body, `"canShip":false`) {
		t.Errorf("a team with no shipper says so, so the board offers no Shipping:\n%s", body)
	}
}

// A push held for you in Shipping says where it goes, not the command.
func TestAHeldPushSaysWhereItGoes(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Ship the fix"}`)
	worker, deployer := events.Actor{Kind: events.Worker, ID: "local"}, events.Actor{Kind: events.Agent, ID: "deployer"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"}, Type: events.TaskAssigned,
			Data: map[string]any{"agent": "deployer", "instructions": "ship it", "ships": true}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "deployer", "hash": "h"}},
		{Stream: "task-1", Actor: deployer, Type: events.ActionRequested, Correlation: "toolu_1",
			Data: map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Policy, ID: "rules"}, Type: events.ActionAsked, Correlation: "toolu_1",
			Data: map[string]any{"reason": "every push"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	_, body := call(t, server, "GET", "/api/tasks", "")
	if !strings.Contains(body, `"column":"shipping"`) || !strings.Contains(body, `"needsYou":"Waiting for you to approve the push to dev"`) {
		t.Errorf("the held push should read plainly in Shipping:\n%s", body)
	}
}

// An epic you move to Done, whether it is asking you to close it or you
// left it open, is closed by the move: nothing is left to ask again.
func TestAnEpicOntoDoneLeavesNothingToClose(t *testing.T) {
	server, s := newTestServer(t)
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	ask := func(id string) {
		if _, rejection, err := s.Propose(events.Proposal{Stream: id, Actor: lead, Type: events.TaskNeedsYou, Data: map[string]any{
			"question": "Close it?", "options": []string{"Close the epic"}, "closing": "All shipped."}}); err != nil || rejection != nil {
			t.Fatal(err, rejection)
		}
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Ship it","kind":"epic"}`)
	ask("task-1")
	call(t, server, "POST", "/api/tasks", `{"title":"Ship that","kind":"epic"}`)
	ask("task-2")
	call(t, server, "POST", "/api/tasks/task-2/answer", `{"answer":"Leave it open"}`)

	for _, id := range []string{"task-1", "task-2"} {
		if code, body := call(t, server, "POST", "/api/tasks/"+id+"/direct", `{"to":"done"}`); code != 200 {
			t.Fatalf("%s onto Done: %d %s", id, code, body)
		}
		stream, _ := s.Stream(id)
		if task := tasks.FromEvents(stream); !task.IsDone || task.Closing != "" || task.Question != "" || task.Asked.By != "" {
			t.Errorf("%s: done %v, closing %q, question %q, asked by %q", id, task.IsDone, task.Closing, task.Question, task.Asked.By)
		}
		call(t, server, "POST", "/api/tasks/"+id+"/answer", `{"answer":"Close the epic"}`)
		stream, _ = s.Stream(id)
		if done := countOf(stream, events.TaskDone); done != 1 {
			t.Errorf("%s: closed %d times", id, done)
		}
	}
	if _, body := call(t, server, "GET", "/api/questions", ""); strings.Contains(body, "task-") {
		t.Errorf("a closed epic asks nothing: %s", body)
	}
}

// countOf is how many events of a kind a stream has.
func countOf(stream []events.Event, kind events.Type) int {
	n := 0
	for _, e := range stream {
		if e.Type == kind {
			n++
		}
	}
	return n
}

// Windows gets PowerShell versions of both install commands, with the
// server and the code quoted for PowerShell.
func TestInstallCommandsForWindows(t *testing.T) {
	server, _ := newTestServer(t)
	for _, c := range []struct{ invite, script, from string }{
		{"/api/integrations/claude-code", "/integrations/claude-code.ps1", "/integrations/agentry?code="},
	} {
		_, body := call(t, server, "POST", c.invite, "")
		var invite struct{ Command, WindowsCommand string }
		json.Unmarshal([]byte(body), &invite)
		if !strings.HasPrefix(invite.WindowsCommand, "irm '") || !strings.HasSuffix(invite.WindowsCommand, "' | iex") || !strings.Contains(invite.WindowsCommand, c.script+"?code=") {
			t.Fatalf("windows command: %q", invite.WindowsCommand)
		}
		code := invite.WindowsCommand[strings.Index(invite.WindowsCommand, "code=")+5 : strings.Index(invite.WindowsCommand, "' | iex")]
		response, err := http.Get(server.URL + c.script + "?code=" + code)
		if err != nil || response.StatusCode != 200 {
			t.Fatalf("script: %v %v", err, response.StatusCode)
		}
		script, _ := io.ReadAll(response.Body)
		if !strings.Contains(string(script), "$code = '"+code+"'") || !strings.Contains(string(script), c.from) {
			t.Errorf("the script should carry its code, quoted for PowerShell:\n%s", script)
		}
		if response, _ := http.Get(server.URL + c.script + "?code=wrong"); response.StatusCode != http.StatusUnauthorized {
			t.Errorf("a wrong code got a script: %d", response.StatusCode)
		}
	}
	if got := powerShellQuoted("it's"); got != "'it''s'" {
		t.Errorf("quoted %q", got)
	}
}
