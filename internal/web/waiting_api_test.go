package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// GET /api/waiting is the core's entries oldest first, then what each
// extension says waits about ?repo=, worked out on the server's state. A
// extension is asked though nothing shows it, and none waiting is []. An
// epic's line names it epic-N; its key and task stay task-N.
func TestWaitingIsTheCoreThenEachExtension(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	polls := extensions.Extension{Name: "polls", Waiting: func(state extensions.Reader, repo string) []extensions.Waiting {
		if len(state.TaskIDs()) == 0 {
			return nil
		}
		return []extensions.Waiting{{Key: "polls/1", Kind: "polls", Line: fmt.Sprintf("1 poll in %q of %d tasks", repo, len(state.TaskIDs()))}}
	}}
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, DeskToken: "desk-secret", DefaultRepo: "/src/app",
		Extensions: []extensions.Extension{polls}}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	if code, body := call(t, server, "GET", "/api/waiting", ""); code != 200 || body != "[]\n" {
		t.Errorf("nothing waiting: %d %q", code, body)
	}

	call(t, server, "POST", "/api/tasks", `{"title":"Pick a layout","kind":"epic"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Ship it"}`)
	worker, coordinator := events.Actor{Kind: events.Worker, ID: "laptop"}, events.Actor{Kind: events.Agent, ID: "tech-lead"}
	for _, p := range []events.Proposal{
		{Stream: "task-2", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/task-2", "branch": "b", "port": 7901}},
		{Stream: "task-2", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		{Stream: "task-2", Actor: coordinator, Type: events.ActionRequested, Correlation: "push-1", Data: map[string]any{"tool": "Bash", "command": "git push"}},
		{Stream: "task-2", Actor: events.Actor{Kind: events.Policy, ID: "policy"}, Type: events.ActionAsked, Correlation: "push-1", Data: map[string]any{"reason": "every push asks"}},
		{Stream: "task-1", Actor: coordinator, Type: events.TaskNeedsYou, Data: map[string]any{"question": "Which layout?"}},
	} {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatalf("%s: %v %v", p.Type, err, rejection)
		}
	}

	code, body := call(t, server, "GET", "/api/waiting?repo=/src/app", "")
	var waiting []extensions.Waiting
	if err := json.Unmarshal([]byte(body), &waiting); err != nil || code != 200 {
		t.Fatal(code, err, body)
	}
	want := []extensions.Waiting{
		{Key: "task-2/approval/push-1/git push", Task: "task-2", Kind: "needs you", Line: "task-2: Waiting for you to approve a push (review_approvals)"},
		{Key: "task-1/question//Which layout?", Task: "task-1", Kind: "needs you", Line: "epic-1: tech-lead asks: Which layout?"},
		{Key: "polls/1", Kind: "polls", Line: `1 poll in "/src/app" of 2 tasks`},
	}
	if fmt.Sprint(waiting) != fmt.Sprint(want) {
		t.Errorf("waiting:\n%v\nwant:\n%v", waiting, want)
	}

	if code, _ := callAs(t, server, "", "GET", "/api/waiting"); code != http.StatusUnauthorized {
		t.Errorf("no token: %d, want 401", code)
	}
	if code, _ := callAs(t, server, "desk-secret", "GET", "/api/waiting"); code != http.StatusUnauthorized {
		t.Errorf("the desk: %d, want 401", code)
	}
}
