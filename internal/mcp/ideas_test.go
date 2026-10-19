package mcp

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/client"
)

// A fake Agentry with ideas in every state and nothing else waiting. It
// records what the tools ask of it, and refuses as the server does: an idea
// that isn't open can't be accepted or dismissed.
type ideasAgentry struct {
	mu    sync.Mutex
	ideas []map[string]any
	calls []string
	fail  bool // GET /api/ideas fails
}

func newIdeasAgentry(t *testing.T) (*ideasAgentry, client.Client) {
	fake := &ideasAgentry{ideas: []map[string]any{
		{"id": "idea-4", "title": "Faster boot", "problem": "Start takes 9 s.", "proposal": "Load lazily.", "size": "S", "task": "task-24", "repo": "/other", "state": "open",
			"about": "task-20", "aboutTitle": "Boot faster"},
		{"id": "idea-3", "title": "Webhook log", "problem": "No one sees failures.", "proposal": "A log page.", "size": "M", "task": "task-24", "repo": "/repo",
			"state": "accepted", "became": "task-30", "note": "soon"},
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
		if f.fail {
			http.Error(w, "the log is broken", http.StatusInternalServerError)
			return
		}
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
				var choice struct{ Kind string }
				if json.Unmarshal(body, &choice); choice.Kind == "bug" {
					http.Error(w, "an idea becomes a task or an epic, not bug", http.StatusConflict)
					return
				}
				io.WriteString(w, `{"task":"task-31","accepted":true,"refused":""}`)
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

// startOnIdeas is start, on the ideas fake.
func startOnIdeas(t *testing.T, canElicit bool, answers func(string, json.RawMessage) any) (*claude, *ideasAgentry) {
	fake, agentry := newIdeasAgentry(t)
	return startOn(t, agentry, canElicit, answers), fake
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
	for _, want := range []string{"idea-3 [accepted] Webhook log", "became task-30: soon", "idea-2 [dismissed] Dark sidebar", "dismissed: not now", "idea-1 [open]"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in:\n%s", want, all)
		}
	}
	if dismissed := c.tool("list_ideas", map[string]any{"state": "dismissed"}); !strings.Contains(dismissed, "idea-2") || strings.Contains(dismissed, "idea-1 [") {
		t.Errorf("dismissed:\n%s", dismissed)
	}
	if everywhere := c.tool("list_ideas", map[string]any{"all": true}); !strings.Contains(everywhere, `idea-4 [open] Faster boot (size S, task, suggested in task-24, after the epic task-20 "Boot faster" closed)`) {
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
	if text := c.tool("accept_idea", map[string]any{"id": "idea-1", "note": "go"}); !strings.Contains(text, "idea-1 became task-31") {
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

// Open ideas are one line of what waits for you; a new idea is news, one
// decided isn't.
func TestOpenIdeasWaitForYou(t *testing.T) {
	fake, agentry := newIdeasAgentry(t)
	ideasIn := func(waiting []Waiting) []Waiting {
		var found []Waiting
		for _, w := range waiting {
			if w.Kind == "ideas" {
				found = append(found, w)
			}
		}
		return found
	}
	before, err := WaitingFor(agentry)
	if err != nil {
		t.Fatal(err)
	}
	if got := ideasIn(before); len(got) != 1 || got[0].Line != "2 open ideas from the Visionaries: Faster boot, Retry webhooks (list_ideas all)" || got[0].Task != "task-24" {
		t.Fatalf("waiting: %+v", before)
	}

	fake.mu.Lock()
	fake.ideas[0]["state"] = "dismissed"
	fake.mu.Unlock()
	after, _ := WaitingFor(agentry)
	if got := ideasIn(after); len(got) != 1 || got[0].Line != "1 open idea from the Visionaries: Retry webhooks (list_ideas all)" || !Seen(before)[got[0].Key] {
		t.Errorf("an idea decided isn't news: %+v, before %+v", after, before)
	}

	fake.mu.Lock()
	fake.ideas = append([]map[string]any{{"id": "idea-5", "title": "Undo a move", "task": "task-40", "state": "open"},
		{"id": "idea-6", "title": "Bulk start", "task": "task-40", "state": "open"},
		{"id": "idea-7", "title": "Tags", "task": "task-40", "state": "open"}}, fake.ideas...)
	fake.mu.Unlock()
	news, _ := WaitingFor(agentry)
	if got := ideasIn(news); len(got) != 1 || got[0].Line != "4 open ideas from the Visionaries: Undo a move, Bulk start, Tags, and 1 more (list_ideas all)" || Seen(before)[got[0].Key] {
		t.Errorf("new ideas are news: %+v", news)
	}

	fake.mu.Lock()
	for _, idea := range fake.ideas {
		idea["state"] = "accepted"
	}
	fake.mu.Unlock()
	if none, _ := WaitingFor(agentry); len(ideasIn(none)) != 0 {
		t.Errorf("no open ideas, yet: %+v", none)
	}
	fake.mu.Lock()
	fake.fail = true
	fake.mu.Unlock()
	if _, err := WaitingFor(agentry); err != nil {
		t.Errorf("ideas the server can't list should be left out: %v", err)
	}
}

// Every reply ends with the open ideas among what waits for you.
func TestRepliesEndWithTheOpenIdeas(t *testing.T) {
	c, fake := startOnIdeas(t, false, nil)
	if text := c.tool("list_tasks", map[string]any{}); !strings.HasSuffix(text, "Waiting for the user (1):\n- 2 open ideas from the Visionaries: Faster boot, Retry webhooks (list_ideas all)") {
		t.Errorf("reply: %s", text)
	}
	fake.mu.Lock()
	for _, idea := range fake.ideas {
		idea["state"] = "dismissed"
	}
	fake.mu.Unlock()
	if text := c.tool("list_tasks", map[string]any{}); strings.Contains(text, "Waiting for the user") {
		t.Errorf("nothing waits, yet: %s", text)
	}
}

// The entry points at list_ideas with all when some open ideas are in
// another repo, which list_ideas alone doesn't show, or when the repo isn't
// known, as in the hook's WaitingFor. Either way the key is the same, so
// where an idea is isn't news.
func TestOpenIdeasPointAtTheirRepo(t *testing.T) {
	fake, agentry := newIdeasAgentry(t)
	elsewhere, _ := openIdeas(agentry, "/repo")
	if want := "2 open ideas from the Visionaries: Faster boot, Retry webhooks (list_ideas all)"; elsewhere.Line != want {
		t.Errorf("some in another repo: %q, want %q", elsewhere.Line, want)
	}
	fake.mu.Lock()
	fake.ideas[0]["repo"] = "/repo"
	fake.mu.Unlock()
	here, _ := openIdeas(agentry, "/repo")
	if want := "2 open ideas from the Visionaries: Faster boot, Retry webhooks (list_ideas)"; here.Line != want {
		t.Errorf("all in this repo: %q, want %q", here.Line, want)
	}
	if here.Key != "ideas/4" || elsewhere.Key != here.Key {
		t.Errorf("keys %q and %q, want ideas/4 for both", elsewhere.Key, here.Key)
	}
	waiting, _ := WaitingFor(agentry)
	if len(waiting) != 1 || !strings.HasSuffix(waiting[0].Line, "(list_ideas all)") || waiting[0].Key != here.Key {
		t.Errorf("repo not known: %+v, want (list_ideas all) and key %q", waiting, here.Key)
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
// the server refuses, which comes back as its reason. A state no idea has
// lists none.
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
	if text := c.tool("list_ideas", map[string]any{"state": "bogus"}); !strings.HasPrefix(text, "No bogus ideas for /repo.") {
		t.Errorf("unknown state:\n%s", text)
	}
}
