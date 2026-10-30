package ideas_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/platform/client"
)

// A fake Agentry with ideas in every state and nothing else waiting. It
// records what the tools ask of it, and refuses as the server does: an idea
// that isn't open can't be accepted or dismissed. An accepted idea's task
// is in the column asked for, In progress with start, unless column is set;
// with like, an idea isn't accepted without anyway, as it looks like the
// epic task-30. Accepted as an epic, its task says it is one.
type ideasAgentry struct {
	mu     sync.Mutex
	ideas  []map[string]any
	calls  []string
	column *string // the column an accept answers, when set
	like   bool    // an accept without anyway is refused as looking like task-30
}

func newIdeasAgentry(t *testing.T) (*ideasAgentry, client.Client) {
	fake := &ideasAgentry{ideas: []map[string]any{
		{"id": "idea-4", "title": "Faster boot", "problem": "Start takes 9 s.", "proposal": "Load lazily.", "size": "S", "task": "task-24", "repo": "/other", "state": "open",
			"about": "task-20", "aboutTitle": "Boot faster"},
		{"id": "idea-3", "title": "Webhook log", "problem": "No one sees failures.", "proposal": "A log page.", "size": "M", "task": "task-24", "repo": "/repo",
			"state": "accepted", "became": "task-30", "becameKind": "epic", "note": "soon"},
		{"id": "idea-2", "title": "Dark sidebar", "problem": "Too bright.", "proposal": "A dark one.", "size": "S", "task": "task-24", "repo": "/repo",
			"state": "dismissed", "note": "not now"},
		{"id": "idea-1", "title": "Retry webhooks", "problem": "Webhooks fail on a 503.", "proposal": "Retry them three times.", "size": "M", "kind": "epic",
			"task": "task-24", "repo": "/repo", "state": "open", "backedBy": []string{"product-thinker", "skeptic"}, "doubtedBy": []string{"user-advocate"}},
	}}
	server := httptest.NewServer(http.HandlerFunc(fake.serve))
	t.Cleanup(server.Close)
	return fake, client.Client{Where: client.Where{URL: server.URL, Page: "https://agentry.example"}, Token: "secret"}
}

func (f *ideasAgentry) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	body, _ := io.ReadAll(r.Body)
	f.calls = append(f.calls, r.Method+" "+r.URL.Path+" "+strings.TrimSpace(string(body)))
	id, action, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/api/ideas/"), "/")
	switch {
	case r.Method == "GET" && r.URL.Path == "/api/ideas":
		list := []map[string]any{}
		for _, idea := range f.ideas {
			if repo := r.URL.Query().Get("repo"); repo == "" || idea["repo"] == repo {
				list = append(list, idea)
			}
		}
		json.NewEncoder(w).Encode(list)
	case r.Method == "POST" && (action == "accept" || action == "dismiss"):
		for _, idea := range f.ideas {
			if idea["id"] != id {
				continue
			}
			if idea["state"] != "open" {
				http.Error(w, "idea "+id+" was "+idea["state"].(string)+" already", http.StatusConflict)
				return
			}
			if action == "accept" {
				var choice struct {
					Kind          string
					Start, Anyway bool
				}
				if json.Unmarshal(body, &choice); choice.Kind == "bug" {
					http.Error(w, "an idea becomes a task or an epic, not bug", http.StatusConflict)
					return
				}
				if f.like && !choice.Anyway {
					w.WriteHeader(http.StatusConflict)
					json.NewEncoder(w).Encode(map[string]any{"error": id + ` looks like the open task task-30 "Retry failed webhooks"`,
						"duplicate": map[string]string{"id": "task-30", "title": "Retry failed webhooks", "column": "backlog", "kind": "epic"}})
					return
				}
				column := map[bool]string{false: "backlog", true: "in-progress"}[choice.Start]
				if f.column != nil {
					column = *f.column
				}
				json.NewEncoder(w).Encode(map[string]any{"task": "task-31", "column": column, "kind": map[string]string{"epic": "epic"}[choice.Kind], "accepted": true, "refused": ""})
				return
			}
			io.WriteString(w, `{}`)
			return
		}
		http.Error(w, "no such idea", http.StatusNotFound)
	case r.URL.Path == "/api/tasks" || r.URL.Path == "/api/questions":
		io.WriteString(w, `[]`)
	default:
		io.WriteString(w, `{}`)
	}
}

// posts are the accepts and dismisses sent, and forgets them.
func (f *ideasAgentry) posts() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var posts []string
	for _, call := range f.calls {
		if strings.HasPrefix(call, "POST /api/ideas/") {
			posts = append(posts, call)
		}
	}
	f.calls = nil
	return posts
}

// claude plays Claude Code calling ideas' tools in agentry mcp, for a
// session in /repo. When it can elicit, answers plays the user.
type claude struct {
	t         *testing.T
	tools     []extensions.Tool
	canElicit bool
	answers   func(method string, params json.RawMessage) any
}

// startOnIdeas starts the ideas' tools on the ideas fake.
func startOnIdeas(t *testing.T, canElicit bool, answers func(string, json.RawMessage) any) (*claude, *ideasAgentry) {
	fake, agentry := newIdeasAgentry(t)
	return &claude{t: t, tools: ideas.Extension().Tools(agentry, "/repo"), canElicit: canElicit, answers: answers}, fake
}

// tool is a tool's reply, or "error: " and its error, as agentry mcp
// answers a tools/call.
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

func TestListIdeasShowsTheOpenOnesUnlessAsked(t *testing.T) {
	c, _ := startOnIdeas(t, false, nil)
	open := c.tool("list_ideas", map[string]any{})
	for _, want := range []string{"idea-1 [open] Retry webhooks (size M, epic, suggested in task-24)", "problem: Webhooks fail on a 503.",
		"feature: Retry them three times.", "backed by: product-thinker, skeptic · doubted by: user-advocate"} {
		if !strings.Contains(open, want) {
			t.Errorf("missing %q in:\n%s", want, open)
		}
	}
	if list, _, _ := strings.Cut(open, "Waiting for the user"); strings.Contains(list, "idea-2") || strings.Contains(list, "idea-3") || strings.Contains(list, "idea-4") {
		t.Errorf("only this repo's open ideas, by default:\n%s", open)
	}

	all := c.tool("list_ideas", map[string]any{"state": "all"})
	for _, want := range []string{"idea-3 [accepted] Webhook log", "became epic-30: soon", "idea-2 [dismissed] Dark sidebar", "dismissed: not now", "idea-1 [open]"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	if dismissed := c.tool("list_ideas", map[string]any{"state": "dismissed"}); !strings.Contains(dismissed, "idea-2") || strings.Contains(dismissed, "idea-1 [") {
		t.Errorf("dismissed:\n%s", dismissed)
	}
	if everywhere := c.tool("list_ideas", map[string]any{"all": true}); !strings.Contains(everywhere, `idea-4 [open] Faster boot (size S, task, suggested in task-24, after epic-20 "Boot faster" closed)`) {
		t.Errorf("every repo:\n%s", everywhere)
	}
}

// The user decides on an idea, in Claude Code: what they confirm is sent,
// and what they decline sends nothing.
func TestTheUserDecidesOnAnIdea(t *testing.T) {
	var asked []string
	var answer map[string]any
	c, fake := startOnIdeas(t, true, func(method string, params json.RawMessage) any {
		var p struct{ Message string }
		json.Unmarshal(params, &p)
		asked = append(asked, p.Message)
		return answer
	})

	answer = map[string]any{"action": "accept", "content": map[string]any{"decision": "accept", "kind": "task", "note": "start small"}}
	text := c.tool("accept_idea", map[string]any{"id": "idea-1", "kind": "epic"})
	if !strings.Contains(text, "idea-1 became task-31") || !strings.Contains(text, "https://agentry.example/tasks/task-31") {
		t.Errorf("accept: %s", text)
	}
	if len(asked) != 1 || !strings.Contains(asked[0], "Accept the idea idea-1, Retry webhooks?") || !strings.Contains(asked[0], "Webhooks fail on a 503.") {
		t.Errorf("asked: %v", asked)
	}
	if posts := fake.posts(); len(posts) != 1 || posts[0] != `POST /api/ideas/idea-1/accept {"kind":"task","note":"start small"}` {
		t.Errorf("the user's kind and note should be sent: %v", posts)
	}

	answer = map[string]any{"action": "accept", "content": map[string]any{"decision": "dismiss", "note": "we have retries"}}
	if text := c.tool("dismiss_idea", map[string]any{"id": "idea-1"}); !strings.Contains(text, "The user dismissed idea-1, Retry webhooks") {
		t.Errorf("dismiss: %s", text)
	}
	if posts := fake.posts(); len(posts) != 1 || posts[0] != `POST /api/ideas/idea-1/dismiss {"note":"we have retries"}` {
		t.Errorf("dismissed: %v", posts)
	}

	for _, declined := range []map[string]any{
		{"action": "accept", "content": map[string]any{"decision": "leave open"}},
		{"action": "decline"},
		{"action": "cancel"},
	} {
		answer = declined
		for _, tool := range []string{"accept_idea", "dismiss_idea"} {
			if text := c.tool(tool, map[string]any{"id": "idea-1"}); !strings.Contains(text, "The user left idea-1 open. Nothing was written.") {
				t.Errorf("%s declined with %v: %s", tool, declined, text)
			}
		}
		if posts := fake.posts(); len(posts) != 0 {
			t.Errorf("declined with %v, yet sent %v", declined, posts)
		}
	}

	// A refusal comes back as what the server said.
	answer = map[string]any{"action": "accept", "content": map[string]any{"decision": "accept"}}
	if text := c.tool("accept_idea", map[string]any{"id": "idea-2"}); text != "error: idea-2 wasn't accepted: idea idea-2 was dismissed already" {
		t.Errorf("refused accept: %s", text)
	}
	answer = map[string]any{"action": "accept", "content": map[string]any{"decision": "dismiss"}}
	if text := c.tool("dismiss_idea", map[string]any{"id": "idea-3"}); text != "error: idea-3 wasn't dismissed: idea idea-3 was accepted already" {
		t.Errorf("refused dismiss: %s", text)
	}
	if text := c.tool("accept_idea", map[string]any{"id": "idea-9"}); text != "error: no idea idea-9" {
		t.Errorf("unknown: %s", text)
	}
}

// Without elicitation the tools act on their arguments, which the
// descriptions say must be the user's own decision.
func TestWithoutElicitationTheArgumentsAreTheDecision(t *testing.T) {
	c, fake := startOnIdeas(t, false, nil)
	if text := c.tool("accept_idea", map[string]any{"id": "idea-1", "note": "go"}); !strings.Contains(text, "idea-1 became epic-31") {
		t.Errorf("accept: %s", text)
	}
	if text := c.tool("dismiss_idea", map[string]any{"id": "idea-1", "note": "no"}); !strings.Contains(text, "The user dismissed idea-1") {
		t.Errorf("dismiss: %s", text)
	}
	want := []string{`POST /api/ideas/idea-1/accept {"kind":"epic","note":"go"}`, `POST /api/ideas/idea-1/dismiss {"note":"no"}`}
	if posts := fake.posts(); strings.Join(posts, "\n") != strings.Join(want, "\n") {
		t.Errorf("sent %v, want %v (the idea's own kind when none is given)", posts, want)
	}
	if text := c.tool("accept_idea", map[string]any{"id": "idea-2"}); !strings.Contains(text, "idea idea-2 was dismissed already") {
		t.Errorf("refused: %s", text)
	}
}

// An idea that looks like an open task: with elicitation the user sees that
// task and chooses to make another or leave the idea open; without, the
// reply names the task and says how to make another, and nothing is sent
// with anyway until the tool is called with it.
func TestAnIdeaLikeAnOpenTaskIsTheUsersToMakeAnyway(t *testing.T) {
	var asked []string
	var again string // the user's answer to the second question
	c, fake := startOnIdeas(t, true, func(method string, params json.RawMessage) any {
		var p struct{ Message string }
		json.Unmarshal(params, &p)
		asked = append(asked, p.Message)
		if len(asked)%2 == 1 {
			return map[string]any{"action": "accept", "content": map[string]any{"decision": "accept", "kind": "task"}}
		}
		return map[string]any{"action": "accept", "content": map[string]any{"decision": again}}
	})
	fake.like = true

	again = "leave open"
	text := c.tool("accept_idea", map[string]any{"id": "idea-1"})
	if !strings.Contains(text, "The user left idea-1 open") || !strings.Contains(text, "epic-30") || !strings.Contains(text, "Nothing was written.") {
		t.Errorf("left open: %s", text)
	}
	if len(asked) != 2 || !strings.Contains(asked[1], `looks like the open task epic-30 "Retry failed webhooks", in the user's Backlog`) ||
		!strings.Contains(asked[1], "https://agentry.example/tasks/task-30") || !strings.Contains(asked[1], "Make another task anyway?") {
		t.Errorf("asked: %q", asked)
	}
	if posts := fake.posts(); len(posts) != 1 || posts[0] != `POST /api/ideas/idea-1/accept {"kind":"task","note":""}` {
		t.Errorf("only the refused accept should be sent: %v", posts)
	}

	again = "make another anyway"
	if text := c.tool("accept_idea", map[string]any{"id": "idea-1"}); !strings.Contains(text, "idea-1 became task-31") {
		t.Errorf("made anyway: %s", text)
	}
	want := []string{`POST /api/ideas/idea-1/accept {"kind":"task","note":""}`, `POST /api/ideas/idea-1/accept {"anyway":true,"kind":"task","note":""}`}
	if posts := fake.posts(); strings.Join(posts, "\n") != strings.Join(want, "\n") {
		t.Errorf("sent %v, want %v", posts, want)
	}

	plain, fake := startOnIdeas(t, false, nil)
	fake.like = true
	text = plain.tool("accept_idea", map[string]any{"id": "idea-1"})
	if !strings.Contains(text, `idea-1 looks like the open task epic-30 "Retry failed webhooks"`) || !strings.Contains(text, "Nothing was written.") ||
		!strings.Contains(text, "call accept_idea again with anyway") {
		t.Errorf("without elicitation: %s", text)
	}
	if text := plain.tool("accept_idea", map[string]any{"id": "idea-1", "anyway": true}); !strings.Contains(text, "idea-1 became epic-31") {
		t.Errorf("with anyway: %s", text)
	}
	want = []string{`POST /api/ideas/idea-1/accept {"kind":"epic","note":""}`, `POST /api/ideas/idea-1/accept {"anyway":true,"kind":"epic","note":""}`}
	if posts := fake.posts(); strings.Join(posts, "\n") != strings.Join(want, "\n") {
		t.Errorf("sent %v, want %v", posts, want)
	}
}

// An idea without a size or a source task reads cleanly.
func TestAnIdeaWithoutSizeOrTask(t *testing.T) {
	var asked string
	c, fake := startOnIdeas(t, true, func(method string, params json.RawMessage) any {
		var p struct{ Message string }
		json.Unmarshal(params, &p)
		asked = p.Message
		return map[string]any{"action": "decline"}
	})
	fake.mu.Lock()
	fake.ideas = append(fake.ideas, map[string]any{"id": "idea-5", "title": "Tags", "problem": "Hard to find.", "proposal": "Tags.", "repo": "/repo", "state": "open"})
	fake.mu.Unlock()
	if list := c.tool("list_ideas", map[string]any{}); !strings.Contains(list, "idea-5 [open] Tags (task)\n") {
		t.Errorf("list:\n%s", list)
	}
	c.tool("accept_idea", map[string]any{"id": "idea-5"})
	if !strings.HasSuffix(asked, "Feature: Tags.\n\nTask.") {
		t.Errorf("asked: %q", asked)
	}
}

// What the user says is sent as it is: a kind of task, no note, and a kind
// the server refuses, which comes back as its reason. An unknown state is
// refused, naming the ones there are, rather than listing nothing.
func TestTheArgumentsAreSentAsGiven(t *testing.T) {
	c, fake := startOnIdeas(t, false, nil)
	c.tool("accept_idea", map[string]any{"id": "idea-1", "kind": "task"})
	c.tool("dismiss_idea", map[string]any{"id": "idea-1"})
	want := []string{`POST /api/ideas/idea-1/accept {"kind":"task","note":""}`, `POST /api/ideas/idea-1/dismiss {"note":""}`}
	if posts := fake.posts(); strings.Join(posts, "\n") != strings.Join(want, "\n") {
		t.Errorf("sent %v, want %v", posts, want)
	}
	if text := c.tool("accept_idea", map[string]any{"id": "idea-1", "kind": "bug"}); text != "error: idea-1 wasn't accepted: an idea becomes a task or an epic, not bug" {
		t.Errorf("bad kind: %s", text)
	}
	if text := c.tool("list_ideas", map[string]any{"state": "opne"}); text != `error: no state "opne": use open, accepted, dismissed or all` {
		t.Errorf("unknown state:\n%s", text)
	}
}

// start makes the accepted idea's task start. Without elicitation the
// argument decides; with it the user's backlog/start answer does, offered
// with the argument as its default. The reply says where the task went, and
// an accept that doesn't start sends what it always did.
func TestAcceptingAnIdeaCanStartItsTask(t *testing.T) {
	c, fake := startOnIdeas(t, false, nil)
	if text := c.tool("accept_idea", map[string]any{"id": "idea-1", "start": true}); !strings.Contains(text, "idea-1 became epic-31, started (In progress)") {
		t.Errorf("started: %s", text)
	}
	if text := c.tool("accept_idea", map[string]any{"id": "idea-1", "start": false}); !strings.Contains(text, "idea-1 became epic-31, in the user's Backlog") {
		t.Errorf("not started: %s", text)
	}
	want := []string{`POST /api/ideas/idea-1/accept {"kind":"epic","note":"","start":true}`, `POST /api/ideas/idea-1/accept {"kind":"epic","note":""}`}
	if posts := fake.posts(); strings.Join(posts, "\n") != strings.Join(want, "\n") {
		t.Errorf("sent %v, want %v", posts, want)
	}

	var offered []any
	var answer map[string]any
	c, fake = startOnIdeas(t, true, func(method string, params json.RawMessage) any {
		var p struct {
			RequestedSchema struct {
				Properties struct{ Start struct{ Default any } }
			}
		}
		json.Unmarshal(params, &p)
		offered = append(offered, p.RequestedSchema.Properties.Start.Default)
		return answer
	})
	for _, step := range []struct {
		start  bool
		answer string
		reply  string
		body   string
	}{
		{true, "backlog", "in the user's Backlog", `{"kind":"task","note":""}`},
		{false, "start", "started (In progress)", `{"kind":"task","note":"","start":true}`},
	} {
		answer = map[string]any{"action": "accept", "content": map[string]any{"decision": "accept", "kind": "task", "note": "", "start": step.answer}}
		if text := c.tool("accept_idea", map[string]any{"id": "idea-1", "start": step.start}); !strings.Contains(text, step.reply) {
			t.Errorf("start %v, answered %s: %s", step.start, step.answer, text)
		}
		if posts := fake.posts(); len(posts) != 1 || posts[0] != "POST /api/ideas/idea-1/accept "+step.body {
			t.Errorf("start %v, answered %s: sent %v, want %s", step.start, step.answer, posts, step.body)
		}
	}
	if len(offered) != 2 || offered[0] != "start" || offered[1] != "backlog" {
		t.Errorf("the start argument should be the default offered: %v", offered)
	}
}

// The reply says where Agentry says the task is, not what was asked: a
// retry after a crashed accept reuses the task where it was made.
func TestAnAcceptedIdeaSaysWhereItsTaskReallyIs(t *testing.T) {
	c, fake := startOnIdeas(t, false, nil)
	for _, step := range []struct {
		start  bool
		column string
		reply  string
	}{
		{true, "backlog", "idea-1 became epic-31, in the user's Backlog"},
		{false, "in-progress", "idea-1 became epic-31, started (In progress)"},
		{false, "review", "idea-1 became epic-31, started (Review)"},
		{true, "todo", "idea-1 became epic-31, in To do:"},
		{true, "done", "idea-1 became epic-31, Done:"},
		{true, "", "idea-1 became epic-31: https://agentry.example/tasks/task-31"},
	} {
		fake.column = &step.column
		if text := c.tool("accept_idea", map[string]any{"id": "idea-1", "start": step.start}); !strings.Contains(text, step.reply) {
			t.Errorf("start %v, in %q: %s", step.start, step.column, text)
		}
	}
}
