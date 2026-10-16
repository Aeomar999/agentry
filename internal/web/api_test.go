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

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

const token = "ui-secret"

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), rules.Check)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	hooks := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hooks")) })
	server := httptest.NewServer(NewServer(s, Config{UIToken: token, DefaultRepo: "/src/app"}, hooks, nil).Handler())
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
	call(t, server, "POST", "/api/tasks", `{"title":"Second","repo":"/src/other"}`)

	_, body := call(t, server, "GET", "/api/tasks", "")
	var list []taskSummary
	json.Unmarshal([]byte(body), &list)
	if len(list) != 2 || list[0].ID != "task-2" || list[1].Repo != "/src/app" || list[0].Repo != "/src/other" || list[1].Status != "working" {
		t.Errorf("list (newest first, default repo): %+v", list)
	}

	_, body = call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if detail.Coordinator != "coordinator" || detail.Description != "in src/" || len(detail.Trail) != 1 || detail.Trail[0].To != "coordinator" || len(detail.Events) != 1 {
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
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "coordinator", "hash": "h"}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "coordinator"}, Type: events.ActionRequested, Correlation: "toolu_1",
			Data: map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Policy, ID: "rules"}, Type: events.ActionAsked, Correlation: "toolu_1",
			Data: map[string]any{"reason": "every push"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	if !strings.Contains(body, `"needsYou":"coordinator asks to run: git push origin HEAD:dev"`) || !strings.Contains(body, `"approvals":[{`) {
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
	start, _ := s.LastPosition()
	go func() {
		time.Sleep(100 * time.Millisecond)
		s.Propose(events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated,
			Data: map[string]any{"title": "t", "repo": "/r", "coordinator": "coordinator"}})
	}()
	began := time.Now()
	_, body := call(t, server, "GET", "/api/changes?after="+itoa(start), "")
	if elapsed := time.Since(began); elapsed > 5*time.Second || !strings.Contains(body, `"position":`+itoa(start+1)) {
		t.Errorf("after %s: %s", elapsed, body)
	}
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
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), rules.Check)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	path := filepath.Join(t.TempDir(), "policy.toml")
	live, err := policy.OpenLive(path, "/home/dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(path, []byte("version = 2"), 0o600)
	live.Reload()
	server := httptest.NewServer(NewServer(s, Config{UIToken: token, Policy: live}, http.NotFoundHandler(), nil).Handler())
	defer server.Close()

	_, body := call(t, server, "GET", "/api/policy", "")
	var shown struct {
		Path, Hash, Problem, UseForEveryone string
		Profiles                            []policy.ProfileSummary
	}
	json.Unmarshal([]byte(body), &shown)
	if shown.Path != path || shown.Hash != live.Current().Hash() || !strings.Contains(shown.Problem, "version must be 1") ||
		len(shown.Profiles) != 4 || shown.UseForEveryone != "allow-everything" {
		t.Errorf("policy: %+v", shown)
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

// The desk hands work to the coordinator and reads it back, and nothing
// more: approving an agent's action takes the UI token.
func TestTheDeskTokenOpensOnlyTasks(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), rules.Check)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(NewServer(s, Config{UIToken: token, DeskToken: "desk-secret"}, http.NotFoundHandler(), nil).Handler())
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
		{"POST", "/api/tasks", `{"title":"From the desk","repo":"/src/app"}`},
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
