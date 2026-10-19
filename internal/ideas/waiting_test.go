package ideas_test

import (
	"fmt"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/mcp"
	"gitlab.com/dev.chempa/agentry/internal/settings"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// What waits for you, pinned on a log of every kind: the core's entries
// oldest first, then the open ideas, for a session outside any repo and
// for one in each repo. A dismissed idea leaves the line but not the key.
func TestWhatWaitsForYouIsTheSameList(t *testing.T) {
	server, s := newTestServer(t)
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
	agreed, rejection, err := s.Create(ideas.Prefix, events.Proposal{Actor: agentry, Type: ideas.IdeaSuggested, Data: map[string]any{
		"title": "Show the costs", "problem": "p", "proposal": "f", "size": "M", "repo": other, "task": "task-8", "kind": "",
		"backedBy": []string{"product-strategist", "user-advocate", "technologist", "skeptic"}}})
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	auto := agreed.Stream
	visionaries, _ := agents.TemplateNamed(ideas.VisionariesTeam)
	for _, p := range append(agents.AddTeam(visionaries, nil, you), []events.Proposal{
		{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged, Data: map[string]any{settings.AutoAcceptIdeas: true}},
		{Stream: "task-9", Actor: agentry, Type: events.TaskCreated, Data: map[string]any{"title": "Show the costs", "repo": other,
			"coordinator": "tech-lead", "parked": "backlog", "idea": auto}},
		{Stream: auto, Actor: agentry, Type: ideas.IdeaAccepted, Data: map[string]any{"task": "task-9", "note": "auto-accepted: all Visionaries agreed"}},
	}...) {
		mustPropose(t, s, p)
	}

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
	for _, repo := range []string{"", "/src/app", other} {
		checkWaiting(t, "in "+repo, waitingIn(t, api, repo), append(core, open))
	}

	if code, body := call(t, server, "POST", "/api/ideas/"+tagRepos+"/dismiss", `{"note":"not now"}`); code != 200 {
		t.Fatal(code, body)
	}
	here := features.Waiting{Key: "ideas/3", Task: "task-7", Kind: "ideas",
		Line: "2 open ideas from the Visionaries: Sort the columns, Search the board (list_ideas)"}
	checkWaiting(t, "in /src/app, its ideas all here", waitingIn(t, api, "/src/app"), append(core, here))
	here.Line = strings.Replace(here.Line, "(list_ideas)", "(list_ideas all)", 1)
	checkWaiting(t, "in "+other+", with none of its own", waitingIn(t, api, other), append(core, here))
	checkWaiting(t, "outside a repo, after the dismiss", waitingIn(t, api, ""), append(core, here))
}

// waitingIn is what agentry mcp lists as waiting for a session in repo
// ("" outside one): WaitingFor's entries, then the feature's about repo, as
// the end of every reply has them.
func waitingIn(t *testing.T, api client.Client, repo string) []features.Waiting {
	t.Helper()
	all, err := mcp.WaitingFor(api, nil)
	if err != nil {
		t.Fatal(err)
	}
	return append(all, ideas.Feature().Waiting(api, repo)...)
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
