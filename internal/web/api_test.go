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

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
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
