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
	if code, _ := call(t, server, "DELETE", "/api/agents/coordinator", ""); code != http.StatusConflict {
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

// Moving a card tells the coordinator what you want next, in the trail.
func TestMovingACardDirectsTheCoordinator(t *testing.T) {
	server, _ := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"Fix the login redirect"}`)
	if code, body := call(t, server, "POST", "/api/tasks/task-1/direct", `{"to":"reviewer","note":"It was reviewed by hand already."}`); code != 200 {
		t.Fatalf("direct: %d %s", code, body)
	}
	if code, _ := call(t, server, "POST", "/api/tasks/task-1/direct", `{"to":"designer"}`); code != http.StatusBadRequest {
		t.Errorf("to an agent that doesn't exist: %d", code)
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	last := detail.Trail[len(detail.Trail)-1]
	if last.From != "you" || last.Text != "Hand this task to reviewer next. It was reviewed by hand already." || detail.Status != "working" {
		t.Errorf("trail %+v, status %s", detail.Trail, detail.Status)
	}
}

// Labels are cleaned when a task is created, replaced later, and kept on a
// finished task too.
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
	coordinator := events.Actor{Kind: events.Agent, ID: "coordinator"}
	propose := func(p events.Proposal) {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatalf("%s: %v %v", p.Type, err, rejection)
		}
	}
	for _, id := range []string{"task-1", "task-2"} {
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/" + id, "branch": "b", "port": 7901}})
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "coordinator", "hash": "h"}})
	}
	propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "coordinator", "report": "r"}})
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
