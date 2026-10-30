package tasksapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/platform/client"
)

// A fake Agentry: tasks, epics and a story of one, one action waiting for
// approval and an epic asking to be closed. It records what the tools ask of it.
// It mirrors internal/mcp/server_test.go's.
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
				{"id":"task-9","title":"Elsewhere","repo":"/other","status":"working","currentAgent":"engineer","rank":-9},
				{"id":"task-6","title":"Polish the menu","repo":"/repo","status":"held","epic":"task-3","rank":-6}]`)
		case "GET /api/tasks/task-3":
			io.WriteString(w, `{"id":"task-3","title":"Polish","repo":"/repo","status":"working","kind":"epic","stories":3,"storiesDone":3,"closing":"All polished.","tokensUnknown":"Tokens unknown on 2 turns: transcript not saved"}`)
		case "GET /api/tasks/task-6":
			io.WriteString(w, `{"id":"task-6","title":"Polish the menu","repo":"/repo","status":"held","epic":"task-3"}`)
		case "GET /api/tasks/task-5":
			io.WriteString(w, `{"id":"task-5","title":"Login page","repo":"/repo","status":"done","column":"done","pushed":true,"doneByYou":true}`)
		case "GET /api/questions":
			io.WriteString(w, `[{"kind":"approval","task":"task-1","taskTitle":"Fix the login","agent":"deployer","text":"git push origin dev","action":"push-1","tool":"Bash","reason":"pushing asks you"},
				{"kind":"question","task":"task-2","taskTitle":"Ship it","taskKind":"epic","agent":"tech-lead","text":"Every story of this epic is done. Close it?","options":["Close the epic"]}]`)
		case "GET /api/waiting":
			io.WriteString(w, `[{"key":"task-1/approval/push-1/git push origin dev","task":"task-1","kind":"needs you","line":"task-1: Waiting for you to approve the push to dev (review_approvals)"},
				{"key":"task-2/question//Every story of this epic is done. Close it?","task":"task-2","kind":"needs you","line":"epic-2: the epic Ship it is ready to close (close_epic)"}]`)
		case "POST /api/tasks":
			io.WriteString(w, `{"stream":"task-10","notFiled":[]}`)
		default:
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(server.Close)
	return server, calls
}

// claude plays Claude Code calling the tools in agentry mcp, for a session
// in /repo. When it can elicit, answers plays the user. It mirrors
// internal/extensions/ideas/tools_test.go's.
type claude struct {
	t         *testing.T
	tools     []extensions.Tool
	canElicit bool
	answers   func(method string, params json.RawMessage) any
}

func start(t *testing.T, canElicit bool, answers func(string, json.RawMessage) any) (*claude, *[]string) {
	agentry, calls := fakeAgentry(t)
	return startOn(t, client.Client{Where: client.Where{URL: agentry.URL, Page: "https://agentry.example"}, Token: "secret"}, canElicit, answers), calls
}

// startOn starts the tools in /repo on agentry.
func startOn(t *testing.T, agentry client.Client, canElicit bool, answers func(string, json.RawMessage) any) *claude {
	return &claude{t: t, tools: Client{Agentry: agentry, Repo: "/repo"}.Tools(), canElicit: canElicit, answers: answers}
}

// tool is a tool's reply, or "error: " and its error, as agentry mcp
// answers a tools/call, without the note of what waits that it adds.
func (c *claude) tool(name string, arguments any) string {
	c.t.Helper()
	ctx := context.Background()
	if c.canElicit {
		ctx = extensions.WithAsk(ctx, c.ask)
	}
	encoded, _ := json.Marshal(arguments)
	for _, tool := range c.tools {
		if tool.Name == name {
			text, err := tool.Run(ctx, encoded)
			if err != nil {
				return "error: " + err.Error()
			}
			return text
		}
	}
	c.t.Fatalf("no tool %s", name)
	return ""
}

// ask is an elicitation/create, answered by answers.
func (c *claude) ask(_ context.Context, text string, schema map[string]any) (map[string]any, bool, error) {
	params, _ := json.Marshal(map[string]any{"message": text, "requestedSchema": schema})
	encoded, _ := json.Marshal(c.answers("elicitation/create", params))
	var reply struct {
		Action  string
		Content map[string]any
	}
	json.Unmarshal(encoded, &reply)
	return reply.Content, reply.Action == "accept", nil
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

// show_task, list_tasks and status say what a task has used of its budget
// and its turns, with no limit too, and none used yet.
func TestTheTaskSaysItsLimits(t *testing.T) {
	for _, c := range []struct {
		task Task
		want string
	}{
		{Task{Budget: 5_000_000, TokensUsed: 1_200_000, MaxTurns: 24, Turns: 30, TurnsSinceAnswer: 18}, "1.2M of 5M tokens · 18 of 24 turns"},
		{Task{TokensUsed: 950, Turns: 14, TurnsSinceAnswer: 3}, "950 tokens (no limit) · 14 turns (no limit)"},
		{Task{}, "0 tokens (no limit) · 0 turns (no limit)"},
	} {
		if got := usageOf(c.task); got != c.want {
			t.Errorf("%+v: %q, want %q", c.task, got, c.want)
		}
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
		task Task
		want string
	}{
		{Task{Status: "done", Pushed: true}, "shipped"},
		{Task{Status: "done"}, "done"},
		{Task{Status: "working", Pushed: true}, "working"},
	} {
		if got := statusOf(c.task); got != c.want {
			t.Errorf("%+v: %q, want %q", c.task, got, c.want)
		}
	}
}

// show_task says where the task is on the board, as the board names it.
func TestTheTaskSaysItsColumn(t *testing.T) {
	if got := columnOf(Task{Column: "shipping"}); got != "on the board: Shipping" {
		t.Errorf("shipping: %q", got)
	}
	if got := columnOf(Task{Column: "in-progress"}); got != "on the board: In progress" {
		t.Errorf("in progress: %q", got)
	}
}
