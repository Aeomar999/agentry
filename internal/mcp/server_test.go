package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/coordinator"
	"gitlab.com/dev.chempa/agentry/internal/features"
)

// A fake Agentry: two tasks, one action waiting for approval and an epic
// asking to be closed. It records what the tools ask of it.
func fakeAgentry(t *testing.T) (*httptest.Server, *[]string) {
	var mu sync.Mutex
	calls := &[]string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		*calls = append(*calls, r.Method+" "+r.URL.Path+" "+strings.TrimSpace(string(body)))
		mu.Unlock()
		switch r.Method + " " + r.URL.Path {
		case "GET /api/tasks":
			io.WriteString(w, `[{"id":"task-2","title":"Ship it","repo":"/repo","status":"needs you","kind":"epic","stories":2,"storiesDone":2,"rank":-2},
				{"id":"task-1","title":"Fix the login","repo":"/repo","status":"needs you","currentAgent":"deployer","rank":-1,"budget":5000000,"tokensUsed":1200000,"maxTurns":24,"turnsSinceAnswer":4},
				{"id":"task-3","title":"Polish","repo":"/repo","status":"working","kind":"epic","stories":3,"storiesDone":3,"closing":"All polished.","rank":-3},
				{"id":"task-4","title":"Release","repo":"/repo","status":"working","currentAgent":"deployer","column":"shipping","rank":-4,"turns":7,"turnsSinceAnswer":2},
				{"id":"task-9","title":"Elsewhere","repo":"/other","status":"working","currentAgent":"engineer","rank":-9}]`)
		case "GET /api/tasks/task-3":
			io.WriteString(w, `{"id":"task-3","title":"Polish","repo":"/repo","status":"working","kind":"epic","stories":3,"storiesDone":3,"closing":"All polished.","tokensUnknown":"Tokens unknown on 2 turns: transcript not saved"}`)
		case "GET /api/tasks/task-5":
			io.WriteString(w, `{"id":"task-5","title":"Login page","repo":"/repo","status":"done","column":"done","pushed":true,"doneByYou":true}`)
		case "GET /api/questions":
			io.WriteString(w, `[{"kind":"approval","task":"task-1","taskTitle":"Fix the login","agent":"deployer","text":"git push origin dev","action":"push-1","tool":"Bash","reason":"pushing asks you"},
				{"kind":"question","task":"task-2","taskTitle":"Ship it","agent":"tech-lead","text":"Every story of this epic is done. Close it?","options":["Close the epic"]}]`)
		case "POST /api/tasks":
			io.WriteString(w, `{"stream":"task-10","notFiled":[]}`)
		default:
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(server.Close)
	return server, calls
}

// claude plays Claude Code: it sends requests and answers the server's.
type claude struct {
	t        *testing.T
	in       io.Writer
	out      *bufio.Scanner
	next     int
	answers  func(method string, params json.RawMessage) any // to the server's requests
	hello    json.RawMessage                                 // what initialize answered
	notified []string                                        // the server's notifications, oldest first
}

func start(t *testing.T, canElicit bool, answers func(string, json.RawMessage) any) (*claude, *[]string) {
	agentry, calls := fakeAgentry(t)
	return startOn(t, client.Client{Where: client.Where{URL: agentry.URL, Page: "https://agentry.example"}, Token: "secret"}, canElicit, answers), calls
}

// startOn starts the server in /repo on agentry, and Claude Code with it.
func startOn(t *testing.T, agentry client.Client, canElicit bool, answers func(string, json.RawMessage) any) *claude {
	return startWith(t, agentry, nil, canElicit, answers)
}

// startWith is startOn with the features in list.
func startWith(t *testing.T, agentry client.Client, list []features.Feature, canElicit bool, answers func(string, json.RawMessage) any) *claude {
	s := &Server{agentry: agentry, repo: "/repo", version: "test", features: list}
	toServer, fromClaude := io.Pipe()
	fromServer, toClaude := io.Pipe()
	go s.Serve(context.Background(), toServer, toClaude)
	t.Cleanup(func() { fromClaude.Close(); toClaude.Close() })
	c := &claude{t: t, in: fromClaude, out: bufio.NewScanner(fromServer), answers: answers}
	capabilities := map[string]any{}
	if canElicit {
		capabilities["elicitation"] = map[string]any{}
	}
	c.hello = c.call("initialize", map[string]any{"protocolVersion": latest, "capabilities": capabilities})
	return c
}

func (c *claude) call(method string, params any) json.RawMessage {
	c.t.Helper()
	c.next++
	encoded, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": c.next, "method": method, "params": params})
	c.in.Write(append(encoded, '\n'))
	for c.out.Scan() {
		var m message
		json.Unmarshal(c.out.Bytes(), &m)
		if m.Method != "" && m.ID == nil { // the server tells something
			c.notified = append(c.notified, m.Method)
			continue
		}
		if m.Method != "" { // the server asks something
			result, _ := json.Marshal(c.answers(m.Method, m.Params))
			reply, _ := json.Marshal(message{JSONRPC: "2.0", ID: m.ID, Result: result})
			c.in.Write(append(reply, '\n'))
			continue
		}
		if string(m.ID) == strconv.Itoa(c.next) {
			if m.Error != nil {
				c.t.Fatalf("%s: %s", method, m.Error.Message)
			}
			return m.Result
		}
	}
	c.t.Fatalf("%s: no answer", method)
	return nil
}

// waitFor reads what the server sends until it notifies method, and fails
// if that takes more than a few seconds.
func (c *claude) waitFor(method string) {
	c.t.Helper()
	deadline := time.After(5 * time.Second)
	for !slices.Contains(c.notified, method) {
		scanned := make(chan bool, 1)
		go func() { scanned <- c.out.Scan() }()
		select {
		case ok := <-scanned:
			if !ok {
				c.t.Fatalf("no %s: the server stopped", method)
			}
		case <-deadline:
			c.t.Fatalf("no %s within 5 s; notified %v", method, c.notified)
		}
		var m message
		json.Unmarshal(c.out.Bytes(), &m)
		if m.Method != "" && m.ID == nil {
			c.notified = append(c.notified, m.Method)
		}
	}
}

func (c *claude) tool(name string, arguments any) string {
	c.t.Helper()
	var result struct {
		IsError bool
		Content []struct{ Text string }
	}
	json.Unmarshal(c.call("tools/call", map[string]any{"name": name, "arguments": arguments}), &result)
	if len(result.Content) == 0 {
		c.t.Fatalf("%s: no content", name)
	}
	if result.IsError {
		return "error: " + result.Content[0].Text
	}
	return result.Content[0].Text
}

func TestTheBoardAndHandingOverWork(t *testing.T) {
	c, calls := start(t, false, nil)
	var listed struct{ Tools []struct{ Name string } }
	json.Unmarshal(c.call("tools/list", nil), &listed)
	if len(listed.Tools) < 10 {
		t.Errorf("tools: %+v", listed.Tools)
	}

	board := c.tool("status", map[string]any{})
	for _, want := range []string{"Agentry board for /repo", "task-1: Waiting for you to approve the push to dev (review_approvals)", "task-2: the epic Ship it is ready to close", "task-2 Ship it: 2 of 2 stories done",
		"task-4 Release: deployer is working (Shipping) · 0 tokens (no limit) · 7 turns (no limit)"} {
		if !strings.Contains(board, want) {
			t.Errorf("the board is missing %q:\n%s", want, board)
		}
	}
	if strings.Contains(board, "Elsewhere") {
		t.Errorf("another repo's task is on this repo's board:\n%s", board)
	}
	tasks := c.tool("list_tasks", map[string]any{})
	if !strings.Contains(tasks, "task-4 [working · Shipping] Release · 0 tokens (no limit) · 7 turns (no limit)") {
		t.Errorf("list_tasks should name the board column and what the task used:\n%s", tasks)
	}
	if !strings.Contains(tasks, "task-1 [needs you] Fix the login · 1.2M of 5M tokens · 4 of 24 turns") {
		t.Errorf("list_tasks should say what the task used of its limits:\n%s", tasks)
	}

	created := c.tool("create_task", map[string]any{"title": "Add a retry", "description": "Retry on 503."})
	if !strings.Contains(created, "Created task-10") || !strings.Contains(created, "https://agentry.example/tasks/task-10") ||
		!strings.Contains(created, "Waiting for the user (2)") {
		t.Errorf("create: %s", created)
	}
	if !strings.Contains(strings.Join(*calls, "\n"), `POST /api/tasks {"budget":0,"description":"Retry on 503.","epic":"","kind":"","labels":null,"maxTurns":0,"repo":"/repo"`) {
		t.Errorf("the task should be made in this repo: %v", *calls)
	}

	// Without elicitation, approving is left to the page.
	if text := c.tool("review_approvals", map[string]any{}); !strings.Contains(text, "theirs to give") || !strings.Contains(text, "https://agentry.example/tasks?status=needs-you") {
		t.Errorf("approvals without elicitation: %s", text)
	}
	if text := c.tool("answer_question", map[string]any{"task": "task-2", "answer": "close the epic"}); !strings.Contains(text, "the user's to approve") {
		t.Errorf("the model closed an epic by answering: %s", text)
	}
	for _, call := range *calls {
		if strings.Contains(call, "/approve") || strings.Contains(call, "Close the epic") {
			t.Errorf("approved without the user: %s", call)
		}
	}
}

// A held action was refused long ago: the user hears that, when it was held
// (with the zone), and what approving it does now.
func TestAHeldApprovalSaysWhenItWasHeld(t *testing.T) {
	heldAt := time.Date(2026, 10, 19, 2, 30, 0, 0, time.UTC)
	agentry := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/questions":
			fmt.Fprintf(w, `[{"kind":"approval","task":"task-1","taskTitle":"Fix the login","agent":"deployer","text":"git push origin dev",
				"action":"push-1","tool":"Bash","reason":"pushing asks you","held":true,"heldAt":%q}]`, heldAt.Format(time.RFC3339))
		case "GET /api/tasks":
			io.WriteString(w, `[]`)
		default:
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(agentry.Close)
	var asked string
	c := startOn(t, client.Client{Where: client.Where{URL: agentry.URL}}, true, func(method string, params json.RawMessage) any {
		var p struct{ Message string }
		json.Unmarshal(params, &p)
		asked = p.Message
		return map[string]any{"action": "decline"}
	})
	c.tool("review_approvals", map[string]any{})
	for _, want := range []string{"Bash: git push origin dev", "it was held at " + heldAt.Local().Format("Mon 2 Jan 15:04 MST") + ",", "deployer was told no",
		"deployer may run this exact command once, in its next turn"} {
		if !strings.Contains(asked, want) {
			t.Errorf("the question should say %q:\n%s", want, asked)
		}
	}
}

func TestApprovalsAreAskedOfTheUser(t *testing.T) {
	var asked []string
	c, calls := start(t, true, func(method string, params json.RawMessage) any {
		var p struct{ Message string }
		json.Unmarshal(params, &p)
		asked = append(asked, method+": "+p.Message)
		if strings.Contains(p.Message, "Close the epic") {
			return map[string]any{"action": "accept", "content": map[string]any{"decision": "close"}}
		}
		return map[string]any{"action": "accept", "content": map[string]any{"decision": "deny", "reason": "not on a Friday"}}
	})
	if text := c.tool("review_approvals", map[string]any{}); !strings.Contains(text, "task-1 push-1: denied by the user") {
		t.Errorf("review: %s", text)
	}
	if text := c.tool("close_epic", map[string]any{"epic": "task-2"}); !strings.Contains(text, "The user closed task-2") {
		t.Errorf("close: %s", text)
	}
	if len(asked) != 2 || !strings.Contains(asked[0], "elicitation/create: deployer, on task-1 (Fix the login), asks to run") {
		t.Errorf("asked: %v", asked)
	}
	all := strings.Join(*calls, "\n")
	if !strings.Contains(all, `POST /api/tasks/task-1/actions/push-1/deny {"reason":"not on a Friday"}`) ||
		!strings.Contains(all, `POST /api/tasks/task-2/answer {"answer":"Close the epic"}`) {
		t.Errorf("calls: %s", all)
	}
}

// An epic you left open with every story done isn't waiting for you, and
// you can still close it; one with stories going can't be.
func TestYouCloseAnEpicYouLeftOpen(t *testing.T) {
	var asked []string
	decision := "close"
	c, calls := start(t, true, func(method string, params json.RawMessage) any {
		var p struct{ Message string }
		json.Unmarshal(params, &p)
		asked = append(asked, p.Message)
		return map[string]any{"action": "accept", "content": map[string]any{"decision": decision}}
	})
	board := c.tool("status", map[string]any{})
	if !strings.Contains(board, "task-3 Polish: all 3 stories done, open until the user closes it (close_epic)") {
		t.Errorf("the open epic should be on the board:\n%s", board)
	}
	if needs := board[:strings.Index(board, "Epics")]; strings.Contains(needs, "task-3") {
		t.Errorf("an epic left open isn't waiting for you:\n%s", board)
	}
	if text := c.tool("close_epic", map[string]any{"epic": "task-3"}); !strings.Contains(text, "The user closed task-3") {
		t.Errorf("close: %s", text)
	}
	if len(asked) != 1 || !strings.Contains(asked[0], "Close the epic task-3, Polish?") || !strings.Contains(asked[0], "All polished.") {
		t.Errorf("asked: %v", asked)
	}
	if !strings.Contains(strings.Join(*calls, "\n"), `POST /api/tasks/task-3/answer {"answer":"Close the epic"}`) {
		t.Errorf("calls: %v", *calls)
	}
	if text := c.tool("close_epic", map[string]any{"epic": "task-9"}); !strings.Contains(text, "task-9 isn't ready to close") {
		t.Errorf("a task not ready closed: %s", text)
	}

	// Answering with the close answer, though nothing asks it, still asks
	// you, and sends the answer only once you approve.
	closeAnswer := `POST /api/tasks/task-3/answer {"answer":"Close the epic"}`
	for _, try := range []struct {
		decision, says string
		sent           []string
	}{{"keep open", "The user kept task-3 open", nil}, {"close", "The user closed task-3", []string{closeAnswer}}} {
		asked, *calls, decision = nil, nil, try.decision
		text := c.tool("answer_question", map[string]any{"task": "task-3", "answer": " close the EPIC "})
		sent := slices.DeleteFunc(slices.Clone(*calls), func(call string) bool { return !strings.HasPrefix(call, "POST /api/tasks/task-3/answer") })
		if !strings.Contains(text, try.says) || len(asked) != 1 || !slices.Equal(sent, try.sent) {
			t.Errorf("answered %s: %q, asked %d times, sent %v", try.decision, text, len(asked), sent)
		}
	}
}

func TestTheCloseAnswerIsTheCoordinators(t *testing.T) {
	if closeEpic != coordinator.CloseEpic || copyFile != coordinator.CopyFile {
		t.Errorf("the approving answers differ: %q/%q here, %q/%q in the coordinator", closeEpic, copyFile, coordinator.CloseEpic, coordinator.CopyFile)
	}
}

// show_task, list_tasks and status say what a task has used of its budget
// and its turns, with no limit too, and none used yet.
func TestTheTaskSaysItsLimits(t *testing.T) {
	for _, c := range []struct {
		task task
		want string
	}{
		{task{Budget: 5_000_000, TokensUsed: 1_200_000, MaxTurns: 24, Turns: 30, TurnsSinceAnswer: 18}, "1.2M of 5M tokens · 18 of 24 turns"},
		{task{TokensUsed: 950, Turns: 14, TurnsSinceAnswer: 3}, "950 tokens (no limit) · 14 turns (no limit)"},
		{task{}, "0 tokens (no limit) · 0 turns (no limit)"},
	} {
		if got := usageOf(c.task); got != c.want {
			t.Errorf("%+v: %q, want %q", c.task, got, c.want)
		}
	}
}

// show_task says on how many turns the tokens couldn't be counted, and why,
// as the server words it.
func TestTheTaskSaysWhereItsTokensAreUnknown(t *testing.T) {
	c, _ := start(t, false, nil)
	if text := c.tool("show_task", map[string]any{"task": "task-3"}); !strings.Contains(text, "\nTokens unknown on 2 turns: transcript not saved\n") {
		t.Errorf("show_task should say where the tokens are unknown:\n%s", text)
	}
}

// show_task says a done task whose work an agent pushed is shipped, and
// that the user moved it to Done when they did.
func TestTheTaskSaysItShipped(t *testing.T) {
	c, _ := start(t, false, nil)
	text := c.tool("show_task", map[string]any{"task": "task-5"})
	if !strings.Contains(text, "\nStatus: shipped · on the board: Done ·") || !strings.Contains(text, "\nMoved to Done by you.\n") {
		t.Errorf("show_task should say it shipped and who moved it:\n%s", text)
	}
	for _, c := range []struct {
		task task
		want string
	}{
		{task{Status: "done", Pushed: true}, "shipped"},
		{task{Status: "done"}, "done"},
		{task{Status: "working", Pushed: true}, "working"},
	} {
		if got := statusOf(c.task); got != c.want {
			t.Errorf("%+v: %q, want %q", c.task, got, c.want)
		}
	}
}

// show_task says where the task is on the board, as the board names it.
func TestTheTaskSaysItsColumn(t *testing.T) {
	if got := columnOf(task{Column: "shipping"}); got != "on the board: Shipping" {
		t.Errorf("shipping: %q", got)
	}
	if got := columnOf(task{Column: "in-progress"}); got != "on the board: In progress" {
		t.Errorf("in progress: %q", got)
	}
}
