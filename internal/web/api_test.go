package web

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
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
	server := httptest.NewServer(NewServer(s, Config{UIToken: token, DefaultRepo: "/src/app", Clones: filepath.Join(t.TempDir(), "repos"),
		Blobs: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}}, hooks, nil).Handler())
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
	if detail.Coordinator != "tech-lead" || detail.Description != "in src/" || len(detail.Trail) != 1 || detail.Trail[0].To != "tech-lead" || len(detail.Events) != 1 {
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
	if !strings.Contains(body, `"needsYou":"tech-lead asks to run: git push origin HEAD:dev"`) || !strings.Contains(body, `"approvals":[{`) {
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
			Data: map[string]any{"title": "t", "repo": "/r", "coordinator": "tech-lead"}})
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

// A task gets the default budget, or its own; raising it on a task stopped
// at its budget answers for you, and the task carries on.
func TestRaisingTheBudgetLetsATaskGoOn(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), rules.Check)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(NewServer(s, Config{UIToken: token, DefaultRepo: "/r", Budget: 5_000_000}, http.NotFoundHandler(), nil).Handler())
	defer server.Close()

	call(t, server, "POST", "/api/tasks", `{"description":"Default budget"}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Its own budget","budget":200000}`)
	_, body := call(t, server, "GET", "/api/tasks", "")
	var list []taskSummary
	json.Unmarshal([]byte(body), &list)
	if list[1].Budget != 5_000_000 || list[0].Budget != 200_000 {
		t.Errorf("budgets: %d, %d", list[1].Budget, list[0].Budget)
	}

	s.Propose(events.Proposal{Stream: "task-2", Actor: events.Actor{Kind: events.System, ID: "coordinator-loop"}, Type: events.TaskNeedsYou,
		Data: map[string]any{"question": "This task has used 210k tokens of its 200k budget."}})
	if code, body := call(t, server, "PUT", "/api/tasks/task-2/budget", `{"budget":1000000}`); code != 200 {
		t.Fatalf("raise: %d %s", code, body)
	}
	_, body = call(t, server, "GET", "/api/tasks/task-2", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if detail.Budget != 1_000_000 || detail.Status != "working" || detail.Trail[len(detail.Trail)-1].Text != "I raised the budget to 1M tokens. Carry on." {
		t.Errorf("after raising: budget %d, status %s, trail %+v", detail.Budget, detail.Status, detail.Trail)
	}
	if code, _ := call(t, server, "PUT", "/api/tasks/task-2/budget", `{"budget":0}`); code != http.StatusConflict {
		t.Errorf("a budget of 0: %d", code)
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
	if d := show(); d.Status != "working" || d.Worktree != nil || lastStep(d) != "Reopen this task. Carry on. The fix broke sign-out." {
		t.Errorf("reopened: %s, worktree %v, %q", d.Status, d.Worktree, lastStep(d))
	}
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
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), rules.Check)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(NewServer(s, Config{UIToken: token, DeskToken: "desk-secret", DefaultRepo: "/r"}, http.NotFoundHandler(), nil).Handler())
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
