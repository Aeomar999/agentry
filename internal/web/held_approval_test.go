package web

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy/hooks"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/platform/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

const hookToken = "hook-secret"

// The gate's wait runs out before you answer: the agent's hook gives up,
// the action is held, and it stays open for you. Questions lists it, the
// task needs you, and approving it late on the page's route records it and
// hands the command back to the coordinator.
func TestATimedOutApprovalStaysOpenUntilYouAnswer(t *testing.T) {
	server, s := serverWithEngineerAtWork(t)
	const push = "git push origin HEAD:dev"

	if err := giveUpOn(server, "toolu_1", push); err == nil {
		t.Fatal("the hook should have given up waiting")
	}
	stillOpenUntilYouApprove(t, server, s, push)
}

// A hook that sends its deadline hears that the action is held before it
// gives up, so the agent is told not to retry it; it stays open for you all the same.
func TestTheHookHearsItsRequestIsHeldBeforeItsDeadline(t *testing.T) {
	server, s := serverWithEngineerAtWork(t)
	const push = "git push origin HEAD:dev"
	const deadline = time.Second

	response, err := postGateCall(server, "toolu_1", push, deadline, deadline)
	if err != nil {
		t.Fatalf("the hook should hear back before its deadline: %v", err)
	}
	defer response.Body.Close()
	var reply hookcall.Reply
	if response.StatusCode != http.StatusOK || json.NewDecoder(response.Body).Decode(&reply) != nil {
		t.Fatalf("HTTP %d, reply %+v", response.StatusCode, reply)
	}
	if reply.ExitCode != 0 || !strings.Contains(reply.Stdout, `"permissionDecision":"deny"`) ||
		!strings.Contains(reply.Stdout, "Don't retry it now") {
		t.Errorf("the agent should be told no, and not to retry: %+v", reply)
	}
	stillOpenUntilYouApprove(t, server, s, push)
}

// The hook token opens only the hook endpoint: on the API it is refused like
// no token at all, so an agent can't approve its own request.
func TestTheHookTokenDoesNotOpenTheAPI(t *testing.T) {
	server, s := serverWithEngineerAtWork(t)
	if err := giveUpOn(server, "toolu_1", "git push origin HEAD:dev"); err == nil {
		t.Fatal("the hook should have given up waiting")
	}
	waitFor(t, "the action to be held", func() (bool, bool) {
		stream, _ := s.Stream("task-1")
		return true, tasks.FromEvents(stream).Action("toolu_1").Held
	})

	for _, auth := range []string{"", "Bearer " + hookToken} {
		if code := statusWith(t, server, "GET", "/api/tasks", auth); code != http.StatusUnauthorized {
			t.Errorf("listing tasks with %q: HTTP %d, want 401", auth, code)
		}
	}
	if code := statusWith(t, server, "GET", "/api/tasks", "Bearer "+token); code != http.StatusOK {
		t.Fatalf("listing tasks with the UI token: HTTP %d, want 200", code)
	}

	approve := "/api/tasks/task-1/actions/toolu_1/approve"
	if code := statusWith(t, server, "POST", approve, "Bearer "+hookToken); code != http.StatusUnauthorized {
		t.Errorf("approving with the hook token: HTTP %d, want 401", code)
	}
	stream, _ := s.Stream("task-1")
	if action := tasks.FromEvents(stream).Action("toolu_1"); action.Decision != tasks.Undecided || !action.Held {
		t.Errorf("the action should still wait for you: %+v", action)
	}
}

// statusWith sends a request with the reason "fine" and the Authorization header auth
// ("" for none), and gives the HTTP status it got.
func statusWith(t *testing.T, server *httptest.Server, method, path, auth string) int {
	t.Helper()
	request, _ := http.NewRequest(method, server.URL+path, strings.NewReader(`{"reason":"fine"}`))
	if auth != "" {
		request.Header.Set("Authorization", auth)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	return response.StatusCode
}

// stillOpenUntilYouApprove checks that the action asking for command, which
// the hook stopped waiting on, is held: Questions lists it, the task needs
// you, and approving it late on the page's route records it and hands the
// command back to the coordinator.
func stillOpenUntilYouApprove(t *testing.T, server *httptest.Server, s *store.Store, push string) {
	t.Helper()
	waitFor(t, "the action to be held", func() (bool, bool) {
		stream, _ := s.Stream("task-1")
		held := tasks.FromEvents(stream).Action("toolu_1").Held
		return true, held
	})

	_, body := call(t, server, "GET", "/api/questions", "")
	var questions []question
	if err := json.Unmarshal([]byte(body), &questions); err != nil {
		t.Fatalf("Questions isn't a list: %v\n%s", err, body)
	}
	if len(questions) != 1 || questions[0].Kind != "approval" || !questions[0].Held || questions[0].Action != "toolu_1" ||
		questions[0].Text != push {
		t.Fatalf("Questions should list the held approval:\n%s", body)
	}
	if d := detailOfTask(t, server, "task-1"); d.Status != tasks.NeedsYou {
		t.Errorf("status %s, want %s", d.Status, tasks.NeedsYou)
	}

	if code, body := call(t, server, "POST", "/api/tasks/task-1/actions/toolu_1/approve", `{"reason":"fine"}`); code != http.StatusOK {
		t.Fatalf("approving late: %d %s", code, body)
	}
	d := detailOfTask(t, server, "task-1")
	if len(d.Actions) == 0 || d.Actions[len(d.Actions)-1].Decision != string(tasks.Approved) {
		t.Errorf("the action should be recorded approved: %+v", d.Actions)
	}
	if len(d.Trail) == 0 {
		t.Fatal("the trail should hand the coordinator the command back, but it's empty")
	}
	if last := d.Trail[len(d.Trail)-1]; last.To != "tech-lead" || !strings.Contains(last.Text, push) {
		t.Errorf("the coordinator should be handed the command back: %+v", last)
	}
	if _, body := call(t, server, "GET", "/api/questions", ""); body != "[]\n" {
		t.Errorf("an answered approval is no longer waiting:\n%s", body)
	}
}

// serverWithEngineerAtWork is a web server, with the real hook endpoint, on a
// task whose engineer turn is open. Its policy asks you about everything.
func serverWithEngineerAtWork(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	you, worker := events.Actor{Kind: events.Human, ID: "you"}, events.Actor{Kind: events.Worker, ID: "local"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Ship", "repo": "/r", "coordinator": "tech-lead"}},
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "report": "r"}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"}, Type: events.TaskAssigned,
			Data: map[string]any{"agent": "engineer", "instructions": "push it"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-2", Data: map[string]any{"agent": "engineer", "hash": "h"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	askAlways := func(string, policy.Call) policy.Verdict {
		return policy.Verdict{Decision: policy.Ask, Rule: "every push"}
	}
	gate := hooks.HTTP(hooks.NewHandler(s, state, askAlways), hookToken)
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, DefaultRepo: "/src/app", Clones: filepath.Join(t.TempDir(), "repos"),
		Blobs: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}}, gate, nil).Handler())
	t.Cleanup(server.Close)
	return server, s
}

// giveUpOn posts the engineer's request for command to the hook endpoint
// as a hook that sends no deadline does, giving up after a short wait.
func giveUpOn(server *httptest.Server, id, command string) error {
	response, err := postGateCall(server, id, command, 0, 200*time.Millisecond)
	if err == nil {
		response.Body.Close()
	}
	return err
}

// postGateCall posts the engineer's request for command to the hook endpoint
// as `agentry hook` does: the call carries deadline (0 for none), and the
// client gives up after wait.
func postGateCall(server *httptest.Server, id, command string, deadline, wait time.Duration) (*http.Response, error) {
	payload := `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_use_id":"` + id + `","tool_input":{"command":"` + command + `"}}`
	body, _ := json.Marshal(hookcall.Call{Task: "task-1", Turn: "turn-2", Agent: "engineer", Engine: "claude", Event: "PreToolUse",
		Payload: json.RawMessage(payload), Deadline: deadline})
	request, _ := http.NewRequest(http.MethodPost, server.URL+hookcall.Path, bytes.NewReader(body))
	request.Header.Set(hookcall.TokenHeader, hookToken)
	return (&http.Client{Timeout: wait}).Do(request)
}
