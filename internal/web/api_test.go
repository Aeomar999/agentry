package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
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

func TestCreateListAndShowATask(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/tasks", `{"title":"Remove the TODOs","description":"in src/"}`); code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	if code, _ := call(t, server, "POST", "/api/tasks", `{"title":"  "}`); code != http.StatusBadRequest {
		t.Errorf("a task without a title: %d", code)
	}
	other := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Second","repo":"`+other+`"}`)
	if code, _ := call(t, server, "POST", "/api/tasks", `{"title":"Third","repo":"/no/such/repo"}`); code != http.StatusBadRequest {
		t.Errorf("a repo that isn't there: %d", code)
	}

	_, body := call(t, server, "GET", "/api/tasks", "")
	var list []taskSummary
	json.Unmarshal([]byte(body), &list)
	if len(list) != 2 || list[0].ID != "task-2" || list[1].Repo != "/src/app" || list[0].Repo != other || list[1].Status != "working" {
		t.Errorf("list (newest first, default repo): %+v", list)
	}

	_, body = call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if detail.Coordinator != "tech-lead" || detail.Description != "in src/" || len(detail.Trail) != 1 || detail.Trail[0].To != "tech-lead" {
		t.Errorf("detail: %+v", detail)
	}
	if code, _ := call(t, server, "GET", "/api/tasks/task-9", ""); code != http.StatusNotFound {
		t.Errorf("a missing task: %d", code)
	}
}

func TestApproveOnlyWhatWasAsked(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Deploy"}`)
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"}, Type: events.ActionRequested, Correlation: "toolu_1",
			Data: map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Policy, ID: "rules"}, Type: events.ActionAsked, Correlation: "toolu_1",
			Data: map[string]any{"reason": "every push"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	if !strings.Contains(body, `"needsYou":"Waiting for you to approve the push to dev"`) || !strings.Contains(body, `"approvals":[{`) {
		t.Errorf("the task should show the approval it needs:\n%s", body)
	}
	if code, body := call(t, server, "POST", "/api/tasks/task-1/actions/toolu_1/approve", `{"reason":"looks right"}`); code != 200 {
		t.Errorf("approve: %d %s", code, body)
	}
	if code, body := call(t, server, "POST", "/api/tasks/task-1/actions/toolu_1/deny", ""); code != http.StatusConflict || !strings.Contains(body, "already approved") {
		t.Errorf("deciding twice: %d %s", code, body)
	}
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
		len(shown.Profiles) != 5 || shown.UseForEveryone != "allow-everything" {
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

func TestServesTheUI(t *testing.T) {
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

// A task can be just a description: the title stands in from its first
// line until the coordinator names it.
func TestATaskNeedsOnlyADescription(t *testing.T) {
	server, _ := newTestServer(t)
	long := "When a webhook delivery fails with a 503 the sender gives up at once, retry it once after a second\nMore detail."
	for _, c := range []struct{ description, wantTitle string }{
		{"Fix the login redirect\nIt loops after sign-in.", "Fix the login redirect"},
		{long, "When a webhook delivery fails with a 503 the sender gives up at once..."},
	} {
		body, _ := json.Marshal(map[string]string{"description": c.description})
		if code, reply := call(t, server, "POST", "/api/tasks", string(body)); code != 200 {
			t.Fatalf("create: %d %s", code, reply)
		}
	}
	_, body := call(t, server, "GET", "/api/tasks", "")
	var list []taskSummary
	json.Unmarshal([]byte(body), &list)
	if len(list) != 2 || list[1].Title != "Fix the login redirect" || list[0].Title != "When a webhook delivery fails with a 503 the sender gives up at once..." {
		t.Errorf("titles: %+v", list)
	}
	if code, _ := call(t, server, "POST", "/api/tasks", `{"description":"  "}`); code != http.StatusBadRequest {
		t.Errorf("an empty task: %d", code)
	}
}

// Agents are added, changed and removed from the page; the rules decide.
func TestAgentsAreEditedFromThePage(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/agents", `{"name":"designer","engine":"claude","prompt":"Design the page.","description":"Draws the UI"}`); code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/agents/engineer", `{"name":"ignored","engine":"codex","model":"gpt-5","prompt":"Write Go.","workers":[]}`); code != 200 {
		t.Fatalf("change: %d %s", code, body)
	}
	if code, _ := call(t, server, "PUT", "/api/agents/engineer", `{"engine":"claude","prompt":"x","workers":["build-box"]}`); code != http.StatusConflict {
		t.Errorf("a worker that doesn't exist: %d", code)
	}
	if code, _ := call(t, server, "DELETE", "/api/agents/tech-lead", ""); code != http.StatusConflict {
		t.Errorf("removing the only coordinator: %d", code)
	}
	if code, body := call(t, server, "DELETE", "/api/agents/designer", ""); code != 200 {
		t.Errorf("remove: %d %s", code, body)
	}

	_, body := call(t, server, "GET", "/api/agents", "")
	var roster []agents.Agent
	json.Unmarshal([]byte(body), &roster)
	engineer := roster[1]
	if len(roster) != 5 || engineer.Name != "engineer" || engineer.Engine != "codex" || engineer.Model != "gpt-5" || engineer.Workers == nil {
		t.Errorf("roster: %+v", roster)
	}
	if !strings.Contains(body, `"workers":[]`) {
		t.Error("an agent with no workers should list none, not null")
	}
}

func TestTasksCarryLabels(t *testing.T) {
	server, _ := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"Fix the login redirect","labels":["Bug"," auth ","bug",""]}`)
	labels := func() []string {
		_, body := call(t, server, "GET", "/api/tasks", "")
		var list []taskSummary
		json.Unmarshal([]byte(body), &list)
		return list[0].Labels
	}
	if got := labels(); strings.Join(got, ",") != "bug,auth" {
		t.Errorf("labels at creation: %v", got)
	}
	if code, body := call(t, server, "PUT", "/api/tasks/task-1/labels", `{"labels":["frontend"]}`); code != 200 {
		t.Fatalf("relabel: %d %s", code, body)
	}
	if got := labels(); strings.Join(got, ",") != "frontend" {
		t.Errorf("labels after relabelling: %v", got)
	}
	if code, _ := call(t, server, "PUT", "/api/tasks/task-1/labels", `{"labels":["`+strings.Repeat("x", 40)+`"]}`); code != http.StatusConflict {
		t.Errorf("a label too long: %d", code)
	}
}

// limitsServer is a server whose new tasks get a 5M budget and 2 turns, and
// ways to give a task turns and to stop it with a question, with more of the
// event's data: the loop's limit stops say "limit".
func limitsServer(t *testing.T) (server *httptest.Server, work func(id string, turns, tokens int), ask func(id string, by events.Actor, more map[string]any)) {
	t.Helper()
	return limitsServerChecked(t, rules.Check)
}

// limitsServerChecked is limitsServer with its own rules, to refuse what the
// real rules would refuse only in a race.
func limitsServerChecked(t *testing.T, check func(events.Proposal, rules.State, *events.Catalogue) *events.Rejection) (server *httptest.Server, work func(id string, turns, tokens int), ask func(id string, by events.Actor, more map[string]any)) {
	t.Helper()
	catalogue := events.Core()
	state := current.New(catalogue)
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, err := store.Open(path, catalogue, state, func(p events.Proposal) *events.Rejection { return check(p, state, catalogue) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	t.Cleanup(func() { // cleanups run last first: before the close
		storetest.CheckEqual(t, s, state)
		storetest.CheckSnapshots(t, path, catalogue, s.LastPosition())
	})
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server = httptest.NewServer(NewServer(s, state, Config{UIToken: token, DefaultRepo: "/r", Budget: 5_000_000, MaxTurns: 2}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	propose := func(p events.Proposal) {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatalf("%s: %v %v", p.Type, err, rejection)
		}
	}
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	work = func(id string, turns, tokens int) {
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/" + id, "branch": "b", "port": 7901}})
		for i := range turns {
			turn := fmt.Sprintf("turn-%d", i+1)
			propose(events.Proposal{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "hash": "h"}})
			propose(events.Proposal{Stream: id, Actor: worker, Type: events.TurnEnded, Correlation: turn,
				Data: map[string]any{"agent": "tech-lead", "report": "r", "tokens": tokens / turns}})
		}
	}
	ask = func(id string, by events.Actor, more map[string]any) {
		data := map[string]any{"question": "Go on?"}
		maps.Copy(data, more)
		propose(events.Proposal{Stream: id, Actor: by, Type: events.TaskNeedsYou, Data: data})
	}
	return server, work, ask
}

var (
	theLoop  = events.Actor{Kind: events.System, ID: "coordinator-loop"}
	techLead = events.Actor{Kind: events.Agent, ID: "tech-lead"}

	// copyQuestion is the loop's question to copy a file from outside a worktree.
	copyQuestion = map[string]any{"question": "Copy it?", "options": []string{"Copy it", "Don't copy it"},
		"fetchMachine": "local", "fetchPath": "/etc/hosts", "fetchAs": "hosts"}
)

func atLimit(limit string) map[string]any { return map[string]any{"limit": limit} }

func detailOfTask(t *testing.T, server *httptest.Server, id string) taskDetail {
	t.Helper()
	_, body := call(t, server, "GET", "/api/tasks/"+id, "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	return detail
}

func lastSaid(d taskDetail) string { return d.Trail[len(d.Trail)-1].Text }

// A new task gets the server's budget and limit of turns, its own, or none.
func TestANewTaskHasItsLimits(t *testing.T) {
	server, _, _ := limitsServer(t)
	for _, body := range []string{
		`{"description":"Defaults"}`,
		`{"description":"Its own","budget":200000,"maxTurns":50}`,
		`{"description":"Unlimited","budget":-1,"maxTurns":-1}`,
		`{"description":"Defaults again","budget":0,"maxTurns":0}`,
	} {
		if code, reply := call(t, server, "POST", "/api/tasks", body); code != 200 {
			t.Fatalf("%s: %d %s", body, code, reply)
		}
	}
	want := map[string][2]int{"task-1": {5_000_000, 2}, "task-2": {200_000, 50}, "task-3": {0, 0}, "task-4": {5_000_000, 2}}
	for id, limits := range want {
		if d := detailOfTask(t, server, id); d.Budget != limits[0] || d.MaxTurns != limits[1] {
			t.Errorf("%s: budget %d, max turns %d, want %v", id, d.Budget, d.MaxTurns, limits)
		}
	}
	if code, _ := call(t, server, "POST", "/api/tasks", `{"description":"x","maxTurns":-5}`); code != http.StatusConflict {
		t.Errorf("a limit of -5 turns: %d", code)
	}
	_, body := call(t, server, "GET", "/api/config", "")
	if !strings.Contains(body, `"budget":5000000`) || !strings.Contains(body, `"maxTurns":2`) {
		t.Errorf("config: %s", body)
	}
}

// Raising the budget, or making it unlimited, on a task stopped at its
// budget answers for you, and the task carries on. A coordinator's own
// question still waits for your answer.
func TestRaisingTheBudgetLetsATaskGoOn(t *testing.T) {
	server, work, ask := limitsServer(t)
	for range 3 {
		call(t, server, "POST", "/api/tasks", `{"description":"Small budget","budget":200000,"maxTurns":-1}`)
	}
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		work(id, 1, 210_000)
	}

	ask("task-1", theLoop, atLimit("budget"))
	if code, body := call(t, server, "PUT", "/api/tasks/task-1/budget", `{"budget":1000000}`); code != 200 || body != `{"answered":true,"budget":1000000}`+"\n" {
		t.Fatalf("raise: %d %s", code, body)
	}
	if d := detailOfTask(t, server, "task-1"); d.Budget != 1_000_000 || d.Status != "working" || lastSaid(d) != "I raised the budget to 1M tokens. Carry on." {
		t.Errorf("after raising: budget %d, status %s, said %q", d.Budget, d.Status, lastSaid(d))
	}

	ask("task-2", theLoop, atLimit("budget"))
	call(t, server, "PUT", "/api/tasks/task-2/budget", `{"budget":0}`)
	if d := detailOfTask(t, server, "task-2"); d.Budget != 0 || d.Status != "working" || lastSaid(d) != "I made the budget unlimited. Carry on." {
		t.Errorf("after unlimiting: budget %d, status %s, said %q", d.Budget, d.Status, lastSaid(d))
	}

	ask("task-3", techLead, nil)
	call(t, server, "PUT", "/api/tasks/task-3/budget", `{"budget":-1}`)
	if d := detailOfTask(t, server, "task-3"); d.Budget != 0 || d.Status != "needs you" {
		t.Errorf("the coordinator's question: budget %d, status %s", d.Budget, d.Status)
	}
	if code, _ := call(t, server, "PUT", "/api/tasks/task-3/budget", `{"budget":-2}`); code != http.StatusConflict {
		t.Errorf("a budget of -2: %d", code)
	}
}

// Raising the limit of turns above the turns had, or making it unlimited,
// on a task stopped at it answers for you, and the task carries on.
func TestRaisingTheTurnLimitLetsATaskGoOn(t *testing.T) {
	server, work, ask := limitsServer(t)
	for range 3 {
		call(t, server, "POST", "/api/tasks", `{"description":"Two turns"}`)
	}
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		work(id, 2, 1000)
	}

	ask("task-1", theLoop, atLimit("turns"))
	if _, body := call(t, server, "PUT", "/api/tasks/task-1/turns", `{"maxTurns":2}`); body != `{"answered":false,"maxTurns":2}`+"\n" {
		t.Errorf("not raised above the turns had, the reply: %s", body)
	}
	if d := detailOfTask(t, server, "task-1"); d.Status != "needs you" || d.TurnsSinceAnswer != 2 || !d.AtTurnLimit {
		t.Errorf("not raised above the turns had: status %s, turns %d", d.Status, d.TurnsSinceAnswer)
	}
	if code, body := call(t, server, "PUT", "/api/tasks/task-1/turns", `{"maxTurns":5}`); code != 200 {
		t.Fatalf("raise: %d %s", code, body)
	}
	if d := detailOfTask(t, server, "task-1"); d.MaxTurns != 5 || d.Status != "working" || lastSaid(d) != "I raised the turn limit to 5. Carry on." {
		t.Errorf("after raising: max turns %d, status %s, said %q", d.MaxTurns, d.Status, lastSaid(d))
	}

	ask("task-2", theLoop, atLimit("turns"))
	call(t, server, "PUT", "/api/tasks/task-2/turns", `{"maxTurns":-1}`)
	if d := detailOfTask(t, server, "task-2"); d.MaxTurns != 0 || d.Status != "working" || lastSaid(d) != "I made the turns unlimited. Carry on." {
		t.Errorf("after unlimiting: max turns %d, status %s, said %q", d.MaxTurns, d.Status, lastSaid(d))
	}

	ask("task-3", techLead, nil)
	call(t, server, "PUT", "/api/tasks/task-3/turns", `{"maxTurns":0}`)
	if d := detailOfTask(t, server, "task-3"); d.MaxTurns != 0 || d.Status != "needs you" {
		t.Errorf("the coordinator's question: max turns %d, status %s", d.MaxTurns, d.Status)
	}
	if code, _ := call(t, server, "PUT", "/api/tasks/task-3/turns", `{"maxTurns":-2}`); code != http.StatusConflict {
		t.Errorf("a limit of -2: %d", code)
	}
}

// A limit is kept though its answer to the task's stop is refused, as when
// the stop was answered as you saved: the reply says why, with a 200.
func TestALimitIsKeptThoughItsAnswerIsRefused(t *testing.T) {
	answeredFirst := func(p events.Proposal, state rules.State, catalogue *events.Catalogue) *events.Rejection {
		if p.Type == events.TaskAnswered {
			return &events.Rejection{Check: "order", Reason: "there's no open question"}
		}
		return rules.Check(p, state, catalogue)
	}
	server, work, ask := limitsServerChecked(t, answeredFirst)
	call(t, server, "POST", "/api/tasks", `{"description":"Budget","budget":200000,"maxTurns":-1}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Turns"}`)
	work("task-1", 1, 210_000)
	work("task-2", 2, 1000)
	ask("task-1", theLoop, atLimit("budget"))
	ask("task-2", theLoop, atLimit("turns"))

	if code, body := call(t, server, "PUT", "/api/tasks/task-1/budget", `{"budget":1000000}`); code != 200 ||
		body != `{"answered":false,"budget":1000000,"refused":"there's no open question"}`+"\n" {
		t.Errorf("the budget: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/tasks/task-2/turns", `{"maxTurns":5}`); code != 200 ||
		body != `{"answered":false,"maxTurns":5,"refused":"there's no open question"}`+"\n" {
		t.Errorf("the turns: %d %s", code, body)
	}
	if d := detailOfTask(t, server, "task-1"); d.Budget != 1_000_000 {
		t.Errorf("the budget kept: %d", d.Budget)
	}
	if d := detailOfTask(t, server, "task-2"); d.MaxTurns != 5 {
		t.Errorf("the turns kept: %d", d.MaxTurns)
	}
}

// On a done task nothing is kept, and the reply is a 409.
func TestADoneTasksLimitsStay(t *testing.T) {
	server, _, ask := limitsServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Done","kind":"epic","budget":200000,"maxTurns":5}`)
	ask("task-1", techLead, map[string]any{"options": []string{"Close the epic"}, "closing": "All shipped."})
	if code, body := call(t, server, "POST", "/api/tasks/task-1/answer", `{"answer":"Close the epic"}`); code != 200 {
		t.Fatalf("close: %d %s", code, body)
	}
	for _, change := range [][2]string{{"budget", `{"budget":1000000}`}, {"turns", `{"maxTurns":10}`}} {
		if code, body := call(t, server, "PUT", "/api/tasks/task-1/"+change[0], change[1]); code != http.StatusConflict || !strings.Contains(body, "is done") {
			t.Errorf("the %s: %d %s", change[0], code, body)
		}
	}
	if d := detailOfTask(t, server, "task-1"); d.Budget != 200_000 || d.MaxTurns != 5 {
		t.Errorf("after: budget %d, max turns %d", d.Budget, d.MaxTurns)
	}
}

// Raising a limit answers only the stop for that limit: not the other
// limit's, though the task is at both, and not the loop's other questions,
// such as copying a file, though the task is over that limit too.
func TestALimitAnswersOnlyItsOwnStop(t *testing.T) {
	server, work, ask := limitsServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"At both","budget":200000}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Copy, over budget","budget":200000,"maxTurns":-1}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Copy, past its turns","budget":-1}`)
	work("task-1", 2, 210_000)
	work("task-2", 1, 210_000)
	work("task-3", 2, 1000)

	ask("task-1", theLoop, atLimit("turns"))
	call(t, server, "PUT", "/api/tasks/task-1/budget", `{"budget":1000000}`)
	if d := detailOfTask(t, server, "task-1"); d.Budget != 1_000_000 || d.Status != "needs you" || d.TurnsSinceAnswer != 2 {
		t.Errorf("the budget, at the turn stop: budget %d, status %s, turns %d", d.Budget, d.Status, d.TurnsSinceAnswer)
	}
	call(t, server, "PUT", "/api/tasks/task-1/turns", `{"maxTurns":5}`)
	if d := detailOfTask(t, server, "task-1"); d.Status != "working" || lastSaid(d) != "I raised the turn limit to 5. Carry on." {
		t.Errorf("the turns, at the turn stop: status %s, said %q", d.Status, lastSaid(d))
	}

	for id, change := range map[string][2]string{
		"task-2": {"budget", `{"budget":1000000}`},
		"task-3": {"turns", `{"maxTurns":5}`},
	} {
		ask(id, theLoop, copyQuestion)
		if code, body := call(t, server, "PUT", "/api/tasks/"+id+"/"+change[0], change[1]); code != 200 {
			t.Fatalf("%s %s: %d %s", id, change[0], code, body)
		}
		if d := detailOfTask(t, server, id); d.Status != "needs you" || !strings.HasPrefix(d.NeedsYou, "Copy") {
			t.Errorf("%s, the %s with the copy question open: status %s, needs you %q", id, change[0], d.Status, d.NeedsYou)
		}
	}
}

// A stop asked before stops said their limit is known by its words, so it
// is answered by its own limit's endpoint, though the task is at both.
func TestAnOlderStopIsKnownByItsWords(t *testing.T) {
	server, work, ask := limitsServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"At both","budget":200000}`)
	call(t, server, "POST", "/api/tasks", `{"description":"At both","budget":200000}`)
	work("task-1", 2, 210_000)
	work("task-2", 2, 210_000)
	ask("task-1", theLoop, map[string]any{"question": "This task has had 2 turns without finishing. Answer with any guidance and the coordinator carries on."})
	ask("task-2", theLoop, map[string]any{"question": "This task has used 210k tokens of its 200k budget. Raise the budget to go on, or tell the coordinator to wrap up."})

	call(t, server, "PUT", "/api/tasks/task-1/budget", `{"budget":1000000}`)
	if d := detailOfTask(t, server, "task-1"); d.Status != "needs you" || d.StoppedAt != "turns" {
		t.Errorf("the budget, at the old turn stop: status %s, stopped at %q", d.Status, d.StoppedAt)
	}
	call(t, server, "PUT", "/api/tasks/task-1/turns", `{"maxTurns":-1}`)
	if d := detailOfTask(t, server, "task-1"); d.Status != "working" || lastSaid(d) != "I made the turns unlimited. Carry on." {
		t.Errorf("the turns, at the old turn stop: status %s, said %q", d.Status, lastSaid(d))
	}
	call(t, server, "PUT", "/api/tasks/task-2/turns", `{"maxTurns":5}`)
	if d := detailOfTask(t, server, "task-2"); d.Status != "needs you" || d.StoppedAt != "budget" {
		t.Errorf("the turns, at the old budget stop: status %s, stopped at %q", d.Status, d.StoppedAt)
	}
	call(t, server, "PUT", "/api/tasks/task-2/budget", `{"budget":1000000}`)
	if d := detailOfTask(t, server, "task-2"); d.Status != "working" || lastSaid(d) != "I raised the budget to 1M tokens. Carry on." {
		t.Errorf("the budget, at the old budget stop: status %s, said %q", d.Status, lastSaid(d))
	}
}

// The loop's other stops carry no limit: lowering a limit below what the
// task has used doesn't make them a limit's stop, and raising it again
// doesn't answer them.
func TestALimitNeverAnswersTheLoopsOtherStops(t *testing.T) {
	server, work, ask := limitsServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"No coordinator","maxTurns":10}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Issues","budget":2000000}`)
	work("task-1", 2, 1000)
	work("task-2", 1, 300_000)
	ask("task-1", theLoop, map[string]any{"question": "The task's coordinator, tech-lead, no longer exists."})
	ask("task-2", theLoop, map[string]any{"question": "3 issues in a row, the last: no agent. Fix what's wrong and answer, and the coordinator carries on."})

	kindOf := func(id string) string {
		_, body := call(t, server, "GET", "/api/questions", "")
		var questions []question
		json.Unmarshal([]byte(body), &questions)
		for _, q := range questions {
			if q.Task == id {
				return q.Kind
			}
		}
		return ""
	}
	for id, change := range map[string][3]string{
		"task-1": {"turns", `{"maxTurns":1}`, `{"maxTurns":10}`},
		"task-2": {"budget", `{"budget":200000}`, `{"budget":2000000}`},
	} {
		call(t, server, "PUT", "/api/tasks/"+id+"/"+change[0], change[1])
		if kind, d := kindOf(id), detailOfTask(t, server, id); kind != "question" || d.StoppedAt != "" {
			t.Errorf("%s, the %s lowered below what's used: kind %q, stopped at %q", id, change[0], kind, d.StoppedAt)
		}
		call(t, server, "PUT", "/api/tasks/"+id+"/"+change[0], change[2])
		if d := detailOfTask(t, server, id); d.Status != "needs you" || lastSaid(d) == "" || strings.HasPrefix(lastSaid(d), "I ") {
			t.Errorf("%s, the %s raised again: status %s, said %q", id, change[0], d.Status, lastSaid(d))
		}
	}
}

// The questions list marks a stop at the budget or the limit of turns, with
// its numbers, and no other question, though the task is over a limit.
func TestQuestionsShowWhichLimitStopped(t *testing.T) {
	server, work, ask := limitsServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"Turns","budget":200000}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Budget","budget":200000,"maxTurns":-1}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Copy","budget":200000,"maxTurns":-1}`)
	work("task-1", 2, 210_000)
	work("task-2", 1, 210_000)
	work("task-3", 1, 210_000)
	ask("task-1", theLoop, atLimit("turns"))
	ask("task-2", theLoop, atLimit("budget"))
	ask("task-3", theLoop, copyQuestion)

	_, body := call(t, server, "GET", "/api/questions", "")
	var questions []question
	json.Unmarshal([]byte(body), &questions)
	kinds := map[string]question{}
	for _, q := range questions {
		kinds[q.Task] = q
	}
	if q := kinds["task-1"]; q.Kind != "turns" || q.TurnsSinceAnswer != 2 || q.MaxTurns != 2 || q.Budget != 0 {
		t.Errorf("the turn stop: %+v", q)
	}
	if q := kinds["task-2"]; q.Kind != "budget" || q.TokensUsed != 210_000 || q.Budget != 200_000 || q.MaxTurns != 0 {
		t.Errorf("the budget stop: %+v", q)
	}
	if q := kinds["task-3"]; q.Kind != "question" || q.Budget != 0 {
		t.Errorf("the copy question: %+v", q)
	}
	for id, want := range map[string]string{"task-1": "turns", "task-2": "budget", "task-3": ""} {
		if d := detailOfTask(t, server, id); d.StoppedAt != want {
			t.Errorf("%s stopped at %q, want %q", id, d.StoppedAt, want)
		}
	}
}

// Tasks list in your priority order; a new one starts at the top.
func TestTasksListInPriorityOrder(t *testing.T) {
	server, _ := newTestServer(t)
	for _, d := range []string{"first", "second", "third"} {
		call(t, server, "POST", "/api/tasks", `{"description":"`+d+`"}`)
	}
	order := func() string {
		_, body := call(t, server, "GET", "/api/tasks", "")
		var list []taskSummary
		json.Unmarshal([]byte(body), &list)
		ids := []string{}
		for _, task := range list {
			ids = append(ids, task.ID)
		}
		return strings.Join(ids, " ")
	}
	if got := order(); got != "task-3 task-2 task-1" {
		t.Errorf("newest first: %s", got)
	}
	_, body := call(t, server, "GET", "/api/tasks", "")
	var list []taskSummary
	json.Unmarshal([]byte(body), &list)
	// task-1 dropped between task-3 and task-2
	call(t, server, "PUT", "/api/tasks/task-1/rank", fmt.Sprintf(`{"rank":%f}`, (list[0].Rank+list[1].Rank)/2))
	if got := order(); got != "task-3 task-1 task-2" {
		t.Errorf("after the drop: %s", got)
	}
}

// Everything waiting for you, from every task, in one list, oldest first.
func TestQuestionsGatherWhatWaitsForYou(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"Pick a layout"}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Ship it"}`)
	worker := events.Actor{Kind: events.Worker, ID: "laptop"}
	coordinator := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	propose := func(p events.Proposal) {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatalf("%s: %v %v", p.Type, err, rejection)
		}
	}
	for _, id := range []string{"task-1", "task-2"} {
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/" + id, "branch": "b", "port": 7901}})
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}})
	}
	propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "report": "r"}})
	propose(events.Proposal{Stream: "task-1", Actor: coordinator, Type: events.TaskNeedsYou,
		Data: map[string]any{"question": "Which layout?", "options": []string{"Tabs", "One page"}, "page": "<svg></svg>"}})
	propose(events.Proposal{Stream: "task-2", Actor: coordinator, Type: events.ActionRequested, Correlation: "push-1", Data: map[string]any{"tool": "Bash", "command": "git push"}})
	propose(events.Proposal{Stream: "task-2", Actor: events.Actor{Kind: events.Policy, ID: "policy"}, Type: events.ActionAsked, Correlation: "push-1", Data: map[string]any{"reason": "every push asks"}})

	_, body := call(t, server, "GET", "/api/questions", "")
	var questions []question
	json.Unmarshal([]byte(body), &questions)
	if len(questions) != 2 {
		t.Fatalf("questions: %s", body)
	}
	ask, push := questions[0], questions[1]
	if ask.Kind != "question" || ask.Task != "task-1" || ask.Text != "Which layout?" || len(ask.Options) != 2 || ask.Page != "<svg></svg>" {
		t.Errorf("the question: %+v", ask)
	}
	if push.Kind != "approval" || push.Action != "push-1" || push.Text != "git push" || push.Reason != "every push asks" {
		t.Errorf("the approval: %+v", push)
	}
}

// A held action says so, and when, on the task and in Questions: its
// agent was told no long ago, and the command may no longer fit.
func TestAHeldActionSaysItWasHeld(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Deploy"}`)
	worker, policy := events.Actor{Kind: events.Worker, ID: "local"}, events.Actor{Kind: events.Policy, ID: "rules"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"}, Type: events.ActionRequested, Correlation: "toolu_1",
			Data: map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"}},
		{Stream: "task-1", Actor: policy, Type: events.ActionAsked, Correlation: "toolu_1", Data: map[string]any{"reason": "every push"}},
		{Stream: "task-1", Actor: policy, Type: events.ActionHeld, Correlation: "toolu_1"},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}

	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if len(detail.Approvals) != 1 || !detail.Approvals[0].Held || detail.Approvals[0].HeldAt.IsZero() {
		t.Fatalf("the task should show the held approval:\n%s", body)
	}
	heldAt := detail.Approvals[0].HeldAt

	_, body = call(t, server, "GET", "/api/questions", "")
	var questions []question
	json.Unmarshal([]byte(body), &questions)
	if len(questions) != 1 || !questions[0].Held || !questions[0].HeldAt.Equal(heldAt) || questions[0].Text != "git push origin HEAD:dev" {
		t.Errorf("Questions should show the held approval with its full command:\n%s", body)
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

// You can edit what an agent knows, and see who changed it.
func TestYouEditWhatAnAgentKnows(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "PUT", "/api/agents/engineer/knowledge", `{"knowledge":"  Prefer table tests.  "}`); code != 200 {
		t.Fatalf("set: %d %s", code, body)
	}
	call(t, server, "PUT", "/api/agents/engineer", `{"engine":"claude","prompt":"Write Go."}`) // settings, not knowledge
	_, body := call(t, server, "GET", "/api/agents", "")
	var roster []agents.Agent
	json.Unmarshal([]byte(body), &roster)
	if roster[1].Knowledge != "Prefer table tests." {
		t.Errorf("knowledge: %q", roster[1].Knowledge)
	}
	_, body = call(t, server, "GET", "/api/agents/engineer/knowledge", "")
	if !strings.Contains(body, `"by":"you"`) || !strings.Contains(body, "Prefer table tests.") {
		t.Errorf("history: %s", body)
	}
}

// The history lists the same versions with the same texts whether the log
// keeps each as a full copy or as a patch on the one before, across a rename.
func TestTheKnowledgeHistoryIsTheSameFromPatches(t *testing.T) {
	versions := []string{"Use gofmt.", "Use gofmt.\nRun go vet.", "Use gofmt -s.\nRun go vet.", "Use gofmt -s.\nRun go vet ./..."}
	history := func(patches bool) string {
		server, s := newTestServer(t)
		engineer := agents.Agent{Name: "engineer"}
		by, before := events.Actor{Kind: events.Agent, ID: "engineer"}, ""
		for i, text := range versions {
			if i == 2 {
				s.Propose(engineer.Renaming(httpapi.You, "builder"))
				engineer.Name, by.ID = "builder", "builder"
			}
			p := engineer.Learned(by, text)
			if patches && i > 0 {
				p = engineer.Patching(by, agents.Diff(before, text))
			}
			if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
				t.Fatal(err, rejection)
			}
			before = text
		}
		_, body := call(t, server, "GET", "/api/agents/builder/knowledge", "")
		var changes []map[string]any
		json.Unmarshal([]byte(body), &changes)
		for _, change := range changes {
			delete(change, "at") // each log was written at its own time
		}
		listed, _ := json.Marshal(changes)
		return string(listed)
	}
	copies, patches := history(false), history(true)
	if want := `[{"by":"builder","knowledge":"Use gofmt -s.\nRun go vet ./..."},{"by":"builder","knowledge":"Use gofmt -s.\nRun go vet."}]`; copies != want {
		t.Errorf("from full copies: %s\nwant %s", copies, want)
	}
	if patches != copies {
		t.Errorf("from patches: %s\nfrom full copies: %s", patches, copies)
	}
}

func TestATaskCanStartANewRepo(t *testing.T) {
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	server, _ := newTestServer(t)
	path := filepath.Join(t.TempDir(), "exchange-market-data-design")
	if code, body := call(t, server, "POST", "/api/tasks", `{"title":"Design market data","repo":"`+path+`","newRepo":true}`); code != 200 {
		t.Fatalf("a task in a new repo: %d %s", code, body)
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if detail.Repo != path {
		t.Errorf("the task's repo: %q", detail.Repo)
	}
	if out, err := exec.Command("git", "-C", path, "log", "--format=%s").Output(); err != nil || strings.TrimSpace(string(out)) != "Start: Design market data" {
		t.Errorf("the new repo's history: %q %v", out, err)
	}
	if code, _ := call(t, server, "POST", "/api/tasks", `{"title":"Again","repo":"`+path+`","newRepo":true}`); code != http.StatusBadRequest {
		t.Errorf("a new repo where one already is: %d", code)
	}
}

// A task, or a move, can name its repo by a git address: it is cloned here.
func TestATaskInARepoNamedByItsAddressWorksInAClone(t *testing.T) {
	server, _ := newTestServer(t)
	address := "file://" + filepath.ToSlash(gitRepo(t))
	if code, body := call(t, server, "POST", "/api/tasks", `{"title":"Make it a PWA","repo":"`+address+`"}`); code != 200 {
		t.Fatalf("a task in a repo by its address: %d %s", code, body)
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if !strings.Contains(detail.Repo, string(filepath.Separator)+"repos"+string(filepath.Separator)) {
		t.Fatalf("the task should work in a clone: %q", detail.Repo)
	}
	if out, err := exec.Command("git", "-C", detail.Repo, "remote", "get-url", "origin").Output(); err != nil || strings.TrimSpace(string(out)) != address {
		t.Errorf("the clone's origin: %q %v", out, err)
	}
	if code, _ := call(t, server, "POST", "/api/tasks", `{"title":"Nowhere","repo":"file:///no/such/repo"}`); code != http.StatusBadRequest {
		t.Errorf("an address with no repo: %d", code)
	}
}

func TestATeamIsAddedFromATemplateAndTakesTasks(t *testing.T) {
	server, s := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/teams", `{"template":"research"}`); code != 200 {
		t.Fatalf("adding the Research team: %d %s", code, body)
	}
	stream, _ := s.Stream(events.AgentsStream)
	roster := agents.FromEvents(stream)
	if lead, ok := roster.Coordinator("Research"); !ok || lead.Name != "research-lead" || len(roster.Assignees("Research")) != 3 {
		t.Fatalf("the Research team: %v", roster.Teams())
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Size the market","team":"Research"}`)
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if detail.Coordinator != "research-lead" {
		t.Errorf("a Research task goes to its lead, not %q", detail.Coordinator)
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Fix the login"}`)
	_, body = call(t, server, "GET", "/api/tasks/task-2", "")
	json.Unmarshal([]byte(body), &detail)
	if detail.Coordinator != "tech-lead" {
		t.Errorf("a task that names no team goes to the first team's coordinator, not %q", detail.Coordinator)
	}
}

func TestRenamingAnAgentKeepsItsTasksGoing(t *testing.T) {
	server, s := newTestServer(t)
	stream, _ := s.Stream(events.AgentsStream)
	lead, _ := agents.FromEvents(stream).Coordinator("")
	call(t, server, "POST", "/api/tasks", `{"title":"Fix the login"}`)
	call(t, server, "PUT", "/api/agents/"+lead.Name+"/knowledge", `{"knowledge":"Slice big tasks."}`)

	if code, body := call(t, server, "POST", "/api/agents/"+lead.Name+"/rename", `{"to":"lead-dev"}`); code != 200 {
		t.Fatalf("renaming: %d %s", code, body)
	}
	stream, _ = s.Stream(events.AgentsStream)
	renamed, ok := agents.FromEvents(stream).Get("lead-dev")
	if !ok || !renamed.Coordinates || renamed.Prompt != lead.Prompt || renamed.Knowledge != "Slice big tasks." {
		t.Errorf("the agent keeps everything but its name: %+v", renamed)
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if detail.Coordinator != "lead-dev" || detail.Trail[0].To != lead.Name {
		t.Errorf("the task goes on with lead-dev, and its history keeps the old name: coordinator %q, first step to %q", detail.Coordinator, detail.Trail[0].To)
	}
	if code, _ := call(t, server, "POST", "/api/agents/lead-dev/rename", `{"to":"engineer"}`); code != http.StatusConflict {
		t.Errorf("renaming to a name in use: %d", code)
	}
}

// A team goes with all its agents, once no unfinished task needs them; the
// last team stays.
func TestATeamIsRemovedWithItsAgents(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "DELETE", "/api/teams/Software", ""); code != http.StatusConflict || !strings.Contains(body, "the only one") {
		t.Errorf("the last team: %d %s", code, body)
	}
	if code, body := call(t, server, "POST", "/api/teams", `{"template":"Writing"}`); code != 200 {
		t.Fatalf("add Writing: %d %s", code, body)
	}
	call(t, server, "POST", "/api/tasks", `{"description":"Write the launch post","team":"Writing"}`)
	if code, body := call(t, server, "DELETE", "/api/teams/writing", ""); code != http.StatusConflict || !strings.Contains(body, "task-1") {
		t.Errorf("a team a task needs: %d %s", code, body)
	}
	if code, body := call(t, server, "DELETE", "/api/agents/editor", ""); code != http.StatusConflict || !strings.Contains(body, "task-1") {
		t.Errorf("an agent a task needs: %d %s", code, body)
	}
	if code, body := call(t, server, "DELETE", "/api/agents/fact-checker", ""); code != 200 {
		t.Errorf("an agent no task needs: %d %s", code, body)
	}
	call(t, server, "POST", "/api/teams", `{"template":"Research"}`)
	if code, body := call(t, server, "DELETE", "/api/teams/Research", ""); code != 200 {
		t.Fatalf("remove Research: %d %s", code, body)
	}
	_, body := call(t, server, "GET", "/api/agents", "")
	if strings.Contains(body, `"Research"`) || strings.Contains(body, "research-lead") {
		t.Errorf("Research should be gone: %s", body)
	}
}

// The templates say their ID and the features they turn on, so the page can
// find the team made from one by template, whatever it is called.
func TestTeamTemplatesSayTheirIDAndFeatures(t *testing.T) {
	server, _ := newTestServer(t)
	_, body := call(t, server, "GET", "/api/teams/templates", "")
	var templates []struct {
		ID       string
		Team     string
		Features []string
	}
	if err := json.Unmarshal([]byte(body), &templates); err != nil || len(templates) != len(agents.Templates) {
		t.Fatalf("templates: %v %s", err, body)
	}
	for i, template := range templates {
		want := agents.Templates[i]
		if template.ID != want.ID || template.Team != want.Team || !slices.Equal(template.Features, want.Features) {
			t.Errorf("template %d is %+v, not %s (%s) with %v", i, template, want.ID, want.Team, want.Features)
		}
	}
	if !strings.Contains(body, `"id":"visionaries"`) || !strings.Contains(body, `"features":["ideas"]`) || !strings.Contains(body, `"features":[]`) {
		t.Errorf("Visionaries should list ideas, and a template with none an empty list:\n%s", body)
	}
}

// Editing an agent keeps the template it came from, whatever the page sends,
// so its team keeps the features the template turns on.
func TestEditingAnAgentKeepsItsTemplate(t *testing.T) {
	server, s := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/teams", `{"template":"Visionaries"}`); code != 200 {
		t.Fatalf("add Visionaries: %d %s", code, body)
	}
	for _, change := range []string{
		`{"engine":"claude","profile":"read-mostly","prompt":"Lead.","coordinates":true,"team":"Visionaries"}`,
		`{"engine":"claude","profile":"read-mostly","prompt":"Lead.","coordinates":true,"team":"Visionaries","template":"software"}`,
	} {
		if code, body := call(t, server, "PUT", "/api/agents/visionary-lead", change); code != 200 {
			t.Fatalf("change: %d %s", code, body)
		}
		stream, _ := s.Stream(events.AgentsStream)
		if template := agents.FromEvents(stream).TemplateOf("Visionaries"); template != "visionaries" {
			t.Errorf("%s: the team's template is %q, not visionaries", change, template)
		}
	}
	_, body := call(t, server, "GET", "/api/agents", "")
	var roster []agents.Agent
	json.Unmarshal([]byte(body), &roster)
	for _, agent := range roster {
		if agent.Name == "visionary-lead" && agent.Template != "visionaries" {
			t.Errorf("the page should show the lead's template: %+v", agent)
		}
	}
	if !strings.Contains(body, `"template":"visionaries"`) {
		t.Errorf("GET /api/agents should show template: %s", body)
	}
}

// Only adding a team makes agents from a template: one you make yourself has
// none, even if the page sends one.
func TestAnAgentYouMakeHasNoTemplate(t *testing.T) {
	server, s := newTestServer(t)
	agent := `{"name":"thinker","engine":"claude","prompt":"Think.","coordinates":true,"team":"Thinkers","template":"visionaries"}`
	if code, body := call(t, server, "POST", "/api/agents", agent); code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	stream, _ := s.Stream(events.AgentsStream)
	if template := agents.FromEvents(stream).TemplateOf("Thinkers"); template != "" {
		t.Errorf("a team you made has template %q", template)
	}
}

// Tasks are filed under epics, from the page or the desk: made with their
// stories, moved between epics, and taken out.
func TestTasksAreFiledUnderEpics(t *testing.T) {
	server, _ := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Add the store","kind":"story"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Add the command","kind":"story"}`)
	code, body := call(t, server, "POST", "/api/tasks", `{"title":"Ship the TODO tool","stories":["task-1","task-9"]}`)
	if code != 200 || !strings.Contains(body, `"stream":"task-3"`) || !strings.Contains(body, "task-9: no task task-9") {
		t.Fatalf("an epic made with its stories: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/tasks/task-2/epic", `{"epic":"task-3"}`); code != 200 {
		t.Fatalf("file a story: %d %s", code, body)
	}
	_, body = call(t, server, "GET", "/api/tasks/task-3", "")
	if !strings.Contains(body, `"kind":"epic"`) || !strings.Contains(body, `"stories":2`) || !strings.Contains(body, "I added task-2 to this epic") {
		t.Errorf("the epic should have both stories: %s", body)
	}
	if code, body := call(t, server, "PUT", "/api/tasks/task-3/epic", `{"epic":"task-1"}`); code != http.StatusConflict || !strings.Contains(body, "isn't an epic") {
		t.Errorf("filed under a story: %d %s", code, body)
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Another epic","kind":"epic"}`)
	if code, body := call(t, server, "PUT", "/api/tasks/task-2/epic", `{"epic":"task-4"}`); code != 200 {
		t.Fatalf("move a story: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/tasks/task-1/epic", `{"epic":""}`); code != 200 {
		t.Fatalf("take a story out: %d %s", code, body)
	}
	_, body = call(t, server, "GET", "/api/tasks/task-3", "")
	if !strings.Contains(body, `"stories":0`) {
		t.Errorf("the first epic should have no stories left: %s", body)
	}
	_, body = call(t, server, "GET", "/api/tasks/task-2", "")
	if !strings.Contains(body, `"epic":"task-4"`) {
		t.Errorf("task-2 should be in task-4: %s", body)
	}
}

// You approve closing an epic, on the page; the desk can't for you, and
// any other answer goes to its coordinator.
func TestYouCloseAnEpic(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, DeskToken: "desk-secret", DefaultRepo: "/r"}, http.NotFoundHandler(), nil).Handler())
	defer server.Close()
	call(t, server, "POST", "/api/tasks", `{"title":"Ship it","kind":"epic"}`)
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	s.Propose(events.Proposal{Stream: "task-1", Actor: lead, Type: events.TaskNeedsYou, Data: map[string]any{
		"question": "Close it?", "options": []string{"Close the epic"}, "closing": "All shipped."}})

	request, _ := http.NewRequest("POST", server.URL+"/api/tasks/task-1/answer", strings.NewReader(`{"answer":"Close the epic"}`))
	request.Header.Set("Authorization", "Bearer desk-secret")
	if response, err := http.DefaultClient.Do(request); err != nil || response.StatusCode != http.StatusForbidden {
		t.Errorf("the desk closed an epic: %v %v", response.StatusCode, err)
	}
	if code, body := call(t, server, "POST", "/api/tasks/task-1/answer", `{"answer":"close the epic"}`); code != 200 {
		t.Fatalf("close: %d %s", code, body)
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	if !strings.Contains(body, `"status":"done"`) || !strings.Contains(body, `"summary":"All shipped."`) {
		t.Errorf("the epic should be closed: %s", body)
	}
}

// An epic you leave open, with every story done, isn't waiting for you, and
// you can still close it from the page.
func TestYouCloseAnEpicYouLeftOpen(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, DefaultRepo: "/r"}, http.NotFoundHandler(), nil).Handler())
	defer server.Close()
	call(t, server, "POST", "/api/tasks", `{"title":"Ship it","kind":"epic"}`)
	s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"}, Type: events.TaskNeedsYou,
		Data: map[string]any{"question": "Close it?", "options": []string{"Close the epic"}, "closing": "All shipped."}})
	if code, body := call(t, server, "POST", "/api/tasks/task-1/answer", `{"answer":"Leave it open"}`); code != 200 {
		t.Fatalf("answer: %d %s", code, body)
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	if !strings.Contains(body, `"status":"working"`) || !strings.Contains(body, `"closing":"All shipped."`) {
		t.Errorf("the epic should be open, ready to close: %s", body)
	}
	if _, body := call(t, server, "GET", "/api/questions", ""); strings.Contains(body, "task-1") {
		t.Errorf("an epic left open isn't waiting for you: %s", body)
	}
	if code, body := call(t, server, "POST", "/api/tasks/task-1/answer", `{"answer":"Close the epic"}`); code != 200 {
		t.Fatalf("close: %d %s", code, body)
	}
	_, body = call(t, server, "GET", "/api/tasks/task-1", "")
	if !strings.Contains(body, `"status":"done"`) || !strings.Contains(body, `"summary":"All shipped."`) || !strings.Contains(body, `"closing":""`) {
		t.Errorf("the epic should be closed: %s", body)
	}
}

// Claude Code on another machine connects with an install command from the
// Integrations page: its code works once, the credential it gets opens the
// API, and revoking the connection closes it again.
func TestClaudeCodeConnectsFromTheIntegrationsPage(t *testing.T) {
	server, _ := newTestServer(t)
	code, body := call(t, server, "POST", "/api/integrations/claude-code", "")
	if code != 200 || !strings.Contains(body, "| sh") {
		t.Fatalf("invite: %d %s", code, body)
	}
	var invite struct{ Command string }
	json.Unmarshal([]byte(body), &invite)
	installCode := invite.Command[strings.Index(invite.Command, "code=")+5 : strings.LastIndex(invite.Command, "'")]

	response, err := http.Get(server.URL + "/integrations/claude-code.sh?code=" + installCode)
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("script: %v %v", err, response.StatusCode)
	}
	script, _ := io.ReadAll(response.Body)
	if !strings.Contains(string(script), "code='"+installCode+"'") || !strings.Contains(string(script), "mcp connect") {
		t.Errorf("the script should carry its code:\n%s", script)
	}

	connect := func() (int, string) {
		response, _ := http.Post(server.URL+"/api/integrations/connect", "application/json",
			strings.NewReader(`{"code":"`+installCode+`","host":"laptop","user":"me","os":"darwin"}`))
		got, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(got)
	}
	status, got := connect()
	var made struct{ Credential string }
	json.Unmarshal([]byte(got), &made)
	if status != 200 || made.Credential == "" {
		t.Fatalf("connect: %d %s", status, got)
	}
	if status, _ := connect(); status != http.StatusConflict {
		t.Errorf("the code worked twice: %d", status)
	}
	if response, _ := http.Get(server.URL + "/integrations/claude-code.sh?code=" + installCode); response.StatusCode != http.StatusUnauthorized {
		t.Errorf("the script came with a used code: %d", response.StatusCode)
	}

	asConnection := func() int {
		request, _ := http.NewRequest("GET", server.URL+"/api/tasks", nil)
		request.Header.Set("Authorization", "Bearer "+made.Credential)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode
	}
	if code := asConnection(); code != 200 {
		t.Errorf("the credential should open the API: %d", code)
	}
	_, body = call(t, server, "GET", "/api/integrations/connections", "")
	var listed struct{ Connections []struct{ ID, Host string } }
	json.Unmarshal([]byte(body), &listed)
	if len(listed.Connections) != 1 || listed.Connections[0].Host != "laptop" || strings.Contains(body, made.Credential) {
		t.Fatalf("connections: %s", body)
	}
	if code, body := call(t, server, "DELETE", "/api/integrations/connections/"+listed.Connections[0].ID, ""); code != 200 {
		t.Fatalf("revoke: %d %s", code, body)
	}
	if code := asConnection(); code != http.StatusUnauthorized {
		t.Errorf("a revoked credential still works: %d", code)
	}
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

// You share files with a task, download them, and take them away.
func TestYouShareFilesWithATask(t *testing.T) {
	server, _ := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Crunch the numbers"}`)
	request, _ := http.NewRequest("POST", server.URL+"/api/tasks/task-1/shared?name=data/sales.csv", strings.NewReader("a,b\n1,2\n"))
	request.Header.Set("Authorization", "Bearer "+token)
	if response, err := http.DefaultClient.Do(request); err != nil || response.StatusCode != 200 {
		t.Fatalf("upload: %v %v", err, response.StatusCode)
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1/shared", "")
	if !strings.Contains(body, `"name":"data/sales.csv"`) || !strings.Contains(body, `"size":8`) || !strings.Contains(body, `"by":"you"`) {
		t.Errorf("list: %s", body)
	}
	if _, body := call(t, server, "GET", "/api/tasks/task-1", ""); !strings.Contains(body, `"shared":[{"name":"data/sales.csv"`) {
		t.Errorf("the task should show it: %s", body)
	}
	if code, body := call(t, server, "GET", "/api/tasks/task-1/shared/file?name=data/sales.csv", ""); code != 200 || body != "a,b\n1,2\n" {
		t.Errorf("download: %d %q", code, body)
	}
	bad, _ := http.NewRequest("POST", server.URL+"/api/tasks/task-1/shared?name=../escape", strings.NewReader("x"))
	bad.Header.Set("Authorization", "Bearer "+token)
	if response, _ := http.DefaultClient.Do(bad); response.StatusCode != http.StatusBadRequest {
		t.Errorf("a name outside the folder: %d", response.StatusCode)
	}
	if code, body := call(t, server, "DELETE", "/api/tasks/task-1/shared?name=data/sales.csv", ""); code != 200 {
		t.Fatalf("remove: %d %s", code, body)
	}
	if _, body := call(t, server, "GET", "/api/tasks/task-1/shared", ""); strings.TrimSpace(body) != "[]" {
		t.Errorf("after removing: %s", body)
	}
}
