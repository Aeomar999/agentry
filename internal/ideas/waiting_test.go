package ideas_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/mcp"
	"gitlab.com/dev.chempa/agentry/internal/settings"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// What waits for you, pinned on a log of every kind: the core's entries
// oldest first, then the open ideas, for a session outside any repo and
// for one in each repo, the same from GET /api/waiting and from agentry
// mcp, in its lists and at the end of its replies. A dismissed idea leaves
// the line but not the key.
func TestWhatWaitsForYouIsTheSameList(t *testing.T) {
	server, s, _ := newServer(t, tasks.Limits{})
	other := gitRepo(t)
	api := client.Client{Where: client.Where{URL: server.URL}, Token: token}
	worker, policy := events.Actor{Kind: events.Worker, ID: "laptop"}, events.Actor{Kind: events.Policy, ID: "policy"}
	theLoop := events.Actor{Kind: events.System, ID: tasks.Loop}
	for _, body := range []string{`{"title":"Pick a layout"}`, `{"title":"Ship it"}`, `{"title":"Big job","budget":200000}`,
		`{"title":"Ship the epic","kind":"epic"}`, `{"title":"Stuck"}`, `{"title":"Copy hosts"}`,
		`{"title":"Think up features"}`, `{"title":"Think up more","repo":"` + other + `"}`} {
		if code, reply := call(t, server, "POST", "/api/tasks", body); code != 200 {
			t.Fatal(code, reply)
		}
	}
	for i := range 6 {
		id := fmt.Sprintf("task-%d", i+1)
		mustPropose(t, s, events.Proposal{Stream: id, Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/" + id, "branch": "b", "port": 7901}})
		mustPropose(t, s, events.Proposal{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}})
	}
	ended := func(id string, tokens int) {
		mustPropose(t, s, events.Proposal{Stream: id, Actor: worker, Type: events.TurnEnded, Correlation: "turn-1",
			Data: map[string]any{"agent": "tech-lead", "report": "r", "tokens": tokens}})
	}
	ended("task-1", 0)
	mustPropose(t, s, events.Proposal{Stream: "task-1", Actor: coordinator, Type: events.TaskNeedsYou,
		Data: map[string]any{"question": "Which layout?", "options": []string{"Tabs", "One page"}}})
	mustPropose(t, s, events.Proposal{Stream: "task-2", Actor: coordinator, Type: events.ActionRequested, Correlation: "push-1",
		Data: map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"}})
	mustPropose(t, s, events.Proposal{Stream: "task-2", Actor: policy, Type: events.ActionAsked, Correlation: "push-1", Data: map[string]any{"reason": "every push asks"}})
	ended("task-3", 210_000)
	mustPropose(t, s, events.Proposal{Stream: "task-3", Actor: theLoop, Type: events.TaskNeedsYou,
		Data: map[string]any{"question": "It used its budget. Raise it?", "limit": "budget"}})
	ended("task-4", 0)
	mustPropose(t, s, events.Proposal{Stream: "task-4", Actor: coordinator, Type: events.TaskNeedsYou,
		Data: map[string]any{"question": "Every story is done. Close it?", "options": []string{"Close the epic"}, "closing": "All shipped."}})
	mustPropose(t, s, events.Proposal{Stream: "task-5", Actor: worker, Type: events.AgentWaiting, Correlation: "turn-1",
		Data: map[string]any{"question": "Trust this folder?"}})
	ended("task-6", 0)
	mustPropose(t, s, events.Proposal{Stream: "task-6", Actor: theLoop, Type: events.TaskNeedsYou, Data: map[string]any{
		"question": "Copy /etc/hosts into the worktree?", "options": []string{"Copy it", "Don't copy it"},
		"fetchMachine": "local", "fetchPath": "/etc/hosts", "fetchAs": "hosts"}})

	suggestIdea(t, s, "task-7", "/src/app", "Search the board")
	suggestIdea(t, s, "task-7", "/src/app", "Sort the columns")
	tagRepos := suggestIdea(t, s, "task-8", other, "Tag the repos")
	autoAccepted(t, s, "task-8", other, "Show the costs", "task-9")

	core := []features.Waiting{
		{Key: "task-1/question//Which layout?", Task: "task-1", Kind: "needs you", Line: "task-1: tech-lead asks: Which layout?"},
		{Key: "task-2/approval/push-1/git push origin HEAD:dev", Task: "task-2", Kind: "needs you", Line: "task-2: Waiting for you to approve the push to dev (review_approvals)"},
		{Key: "task-3/budget//It used its budget. Raise it?", Task: "task-3", Kind: "needs you", Line: "task-3: Agentry asks: It used its budget. Raise it?"},
		{Key: "task-4/question//Every story is done. Close it?", Task: "task-4", Kind: "needs you", Line: "task-4: the epic Ship the epic is ready to close (close_epic)"},
		{Key: "task-5/waiting//Trust this folder?", Task: "task-5", Kind: "needs you", Line: "task-5: tech-lead is stuck on a prompt in its terminal: Trust this folder?"},
		{Key: "task-6/question//Copy /etc/hosts into the worktree?", Task: "task-6", Kind: "needs you",
			Line: `task-6: Copy /etc/hosts into the worktree? (answer_question with "Copy it" asks the user)`},
	}
	open := features.Waiting{Key: "ideas/3", Task: "task-8", Kind: "ideas",
		Line: "3 open ideas from the Visionaries: Tag the repos, Sort the columns, Search the board (list_ideas all)"}
	both := func(where, repo string, want []features.Waiting) {
		t.Helper()
		checkWaiting(t, where, waitingFor(t, api, repo), want)
		checkWaiting(t, where+", from GET /api/waiting", served(t, api, repo), want)
	}
	for _, repo := range []string{"", "/src/app", other} {
		both("in "+repo, repo, append(core, open))
	}
	checkNote(t, "a reply outside a repo", api, t.TempDir(), append(core, open))
	checkNote(t, "a reply in "+other, api, other, append(core, open))

	if code, body := call(t, server, "POST", "/api/ideas/"+tagRepos+"/dismiss", `{"note":"not now"}`); code != 200 {
		t.Fatal(code, body)
	}
	here := features.Waiting{Key: "ideas/3", Task: "task-7", Kind: "ideas",
		Line: "2 open ideas from the Visionaries: Sort the columns, Search the board (list_ideas)"}
	both("in /src/app, its ideas all here", "/src/app", append(core, here))
	here.Line = strings.Replace(here.Line, "(list_ideas)", "(list_ideas all)", 1)
	both("in "+other+", with none of its own", other, append(core, here))
	both("outside a repo, after the dismiss", "", append(core, here))
	checkNote(t, "a reply outside a repo, after the dismiss", api, t.TempDir(), append(core, here))
	checkNote(t, "a reply in "+other+", after the dismiss", api, other, append(core, here))
}

// waitingFor is what agentry mcp says waits for a session in repo ("" outside one).
func waitingFor(t *testing.T, api client.Client, repo string) []features.Waiting {
	t.Helper()
	all, err := mcp.WaitingFor(api, repo)
	if err != nil {
		t.Fatal(err)
	}
	return all
}

// checkNote checks that a reply of agentry mcp, run in dir, ends with the
// count of want and its first five lines (here the core's, so the repo
// isn't seen: the mcp tests check it is passed).
func checkNote(t *testing.T, where string, api client.Client, dir string, want []features.Waiting) {
	t.Helper()
	note := fmt.Sprintf("\n\nWaiting for the user (%d):", len(want))
	for _, w := range want[:min(5, len(want))] {
		note += "\n- " + w.Line
	}
	if reply := replyIn(t, api, dir, "list_tasks"); !strings.HasSuffix(reply, note) {
		t.Errorf("%s:\n%s\nwant it to end with:%s", where, reply, note)
	}
}

// replyIn is the text agentry mcp, run in dir, replies to the tool.
func replyIn(t *testing.T, api client.Client, dir, tool string) string {
	t.Helper()
	toServer, fromClaude := io.Pipe()
	fromServer, toClaude := io.Pipe()
	go mcp.New(api, dir, "test", []features.Feature{ideas.Feature()}).Serve(context.Background(), toServer, toClaude)
	defer func() { fromClaude.Close(); toClaude.Close() }()
	fmt.Fprintf(fromClaude, `{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":%q}}`+"\n", tool)
	out := bufio.NewScanner(fromServer)
	out.Buffer(nil, 1<<20)
	for out.Scan() {
		var m struct {
			ID     int
			Result struct{ Content []struct{ Text string } }
		}
		if json.Unmarshal(out.Bytes(), &m) == nil && m.ID == 1 && len(m.Result.Content) == 1 {
			return m.Result.Content[0].Text
		}
	}
	t.Fatal("the server stopped before it replied")
	return ""
}

// served is what GET /api/waiting lists for a session in repo.
func served(t *testing.T, api client.Client, repo string) []features.Waiting {
	t.Helper()
	var all []features.Waiting
	if err := api.Call("GET", "/api/waiting?repo="+url.QueryEscape(repo), nil, &all); err != nil {
		t.Fatal(err)
	}
	return all
}

// autoAccepted is an idea from task about repo that Agentry accepted for
// you as task as: every Visionary backed it, with their team added and
// your rule on.
func autoAccepted(t *testing.T, s *store.Store, task, repo, title, as string) string {
	t.Helper()
	created, rejection, err := s.Create(ideas.Prefix, events.Proposal{Actor: agentry, Type: ideas.IdeaSuggested, Data: map[string]any{
		"title": title, "problem": "p", "proposal": "f", "size": "M", "repo": repo, "task": task, "kind": "",
		"backedBy": []string{"product-strategist", "user-advocate", "technologist", "skeptic"}}})
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	visionaries, _ := agents.TemplateNamed(ideas.VisionariesTeam)
	for _, p := range append(agents.AddTeam(visionaries, nil, you), []events.Proposal{
		{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged, Data: map[string]any{settings.AutoAcceptIdeas: true}},
		{Stream: as, Actor: agentry, Type: events.TaskCreated, Data: map[string]any{"title": title, "repo": repo,
			"coordinator": "tech-lead", "parked": "backlog", "idea": created.Stream}},
		{Stream: created.Stream, Actor: agentry, Type: ideas.IdeaAccepted, Data: map[string]any{"task": as, "note": "auto-accepted: all Visionaries agreed"}},
	}...) {
		mustPropose(t, s, p)
	}
	return created.Stream
}

func checkWaiting(t *testing.T, where string, got, want []features.Waiting) {
	t.Helper()
	line := func(list []features.Waiting) string {
		var b strings.Builder
		for _, w := range list {
			fmt.Fprintf(&b, "%q %s %q %q\n", w.Key, w.Task, w.Kind, w.Line)
		}
		return b.String()
	}
	if line(got) != line(want) {
		t.Errorf("%s:\n%s\nwant:\n%s", where, line(got), line(want))
	}
}

// Open ideas are one line of what waits for you; a new idea is news, one
// decided isn't, and one Agentry accepted for you isn't either.
func TestOpenIdeasWaitForYou(t *testing.T) {
	s, state, repo := openSeeded(t)
	waiting := func() []features.Waiting { return ideas.Feature().Waiting(state, "") }
	retry := suggest(t, s, repo, map[string]any{"title": "Retry webhooks"})
	boot := suggest(t, s, "/other", map[string]any{"title": "Faster boot"})
	before := waiting()
	if len(before) != 1 || before[0].Line != "2 open ideas from the Visionaries: Faster boot, Retry webhooks (list_ideas all)" ||
		before[0].Task != "task-1" || before[0].Kind != "ideas" || before[0].Key != "ideas/2" {
		t.Fatalf("waiting: %+v", before)
	}

	mustPropose(t, s, events.Proposal{Stream: boot, Actor: you, Type: ideas.IdeaDismissed, Data: map[string]any{"note": "not now"}})
	if after := waiting(); len(after) != 1 || after[0].Line != "1 open idea from the Visionaries: Retry webhooks (list_ideas all)" || after[0].Key != before[0].Key {
		t.Errorf("an idea decided isn't news: %+v, before %+v", after, before)
	}
	autoAccepted(t, s, "task-1", repo, "Search", "task-2")
	if agreed := waiting(); len(agreed) != 1 || agreed[0].Line != "1 open idea from the Visionaries: Retry webhooks (list_ideas all)" || agreed[0].Key != before[0].Key {
		t.Errorf("an idea Agentry accepted for you isn't news: %+v, before %+v", agreed, before)
	}

	var more []string
	for _, title := range []string{"Undo a move", "Bulk start", "Tags"} {
		more = append(more, suggest(t, s, repo, map[string]any{"title": title}))
	}
	if news := waiting(); len(news) != 1 || news[0].Line != "4 open ideas from the Visionaries: Tags, Bulk start, Undo a move, and 1 more (list_ideas all)" || news[0].Key != "ideas/5" {
		t.Errorf("new ideas are news: %+v", news)
	}

	for _, id := range append(more, retry) {
		mustPropose(t, s, events.Proposal{Stream: id, Actor: you, Type: ideas.IdeaDismissed, Data: map[string]any{"note": "no"}})
	}
	if none := waiting(); len(none) != 0 {
		t.Errorf("no open ideas, yet: %+v", none)
	}
}

// The entry points at list_ideas with all when some open ideas are in
// another repo, which list_ideas alone doesn't show, or when the repo isn't
// known, as outside a session. Either way the key is the same, so where an
// idea is isn't news.
func TestOpenIdeasPointAtTheirRepo(t *testing.T) {
	s, state, repo := openSeeded(t)
	suggest(t, s, repo, map[string]any{"title": "Retry webhooks"})
	boot := suggest(t, s, "/other", map[string]any{"title": "Faster boot"})
	elsewhere := ideas.Feature().Waiting(state, repo)[0]
	if want := "2 open ideas from the Visionaries: Faster boot, Retry webhooks (list_ideas all)"; elsewhere.Line != want {
		t.Errorf("some in another repo: %q, want %q", elsewhere.Line, want)
	}
	mustPropose(t, s, events.Proposal{Stream: boot, Actor: you, Type: ideas.IdeaDismissed, Data: map[string]any{"note": "not now"}})
	here := ideas.Feature().Waiting(state, repo)[0]
	if want := "1 open idea from the Visionaries: Retry webhooks (list_ideas)"; here.Line != want {
		t.Errorf("all in this repo: %q, want %q", here.Line, want)
	}
	if here.Key != "ideas/2" || elsewhere.Key != here.Key {
		t.Errorf("keys %q and %q, want ideas/2 for both", elsewhere.Key, here.Key)
	}
	unknown := ideas.Feature().Waiting(state, "")
	if len(unknown) != 1 || !strings.HasSuffix(unknown[0].Line, "(list_ideas all)") || unknown[0].Key != here.Key {
		t.Errorf("repo not known: %+v, want (list_ideas all) and key %q", unknown, here.Key)
	}
}
