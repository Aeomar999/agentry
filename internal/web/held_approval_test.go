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

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/hooks"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

const hookToken = "hook-secret"

// The gate's wait runs out before you answer: the agent's hook gives up,
// the action is held, and it stays open for you. Questions lists it, the
// task needs you, and approving it late on the page's route records it and
// hands the command back to the coordinator.
func TestATimedOutApprovalStaysOpenUntilYouAnswer(t *testing.T) {
	server, s := engineerAtWorkBehind(t)
	const push = "git push origin HEAD:dev"

	if err := giveUpOn(server, "toolu_1", push); err == nil {
		t.Fatal("the hook should have given up waiting")
	}
	waitUntil(t, "the action is held", func() bool {
		stream, _ := s.Stream("task-1")
		return tasks.FromEvents(stream).Action("toolu_1").Held
	})

	_, body := call(t, server, "GET", "/api/questions", "")
	var questions []question
	json.Unmarshal([]byte(body), &questions)
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
	if last := d.Trail[len(d.Trail)-1]; last.To != "tech-lead" || !strings.Contains(last.Text, push) {
		t.Errorf("the coordinator should be handed the command back: %+v", last)
	}
	if _, body := call(t, server, "GET", "/api/questions", ""); body != "[]\n" {
		t.Errorf("an answered approval is no longer waiting:\n%s", body)
	}
}

// engineerAtWorkBehind is a web server, with the real hook endpoint, on a
// task whose engineer turn is open. Its policy asks you about everything.
func engineerAtWorkBehind(t *testing.T) (*httptest.Server, *store.Store) {
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
// as `agentry hook` does, with a short deadline in place of the real one.
func giveUpOn(server *httptest.Server, id, command string) error {
	payload := `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_use_id":"` + id + `","tool_input":{"command":"` + command + `"}}`
	body, _ := json.Marshal(hookcall.Call{Task: "task-1", Turn: "turn-2", Agent: "engineer", Engine: "claude", Event: "PreToolUse",
		Payload: json.RawMessage(payload)})
	request, _ := http.NewRequest(http.MethodPost, server.URL+hookcall.Path, bytes.NewReader(body))
	request.Header.Set(hookcall.TokenHeader, hookToken)
	response, err := (&http.Client{Timeout: 200 * time.Millisecond}).Do(request)
	if err == nil {
		response.Body.Close()
	}
	return err
}

// waitUntil waits for done, which the server makes true after the client
// has given up.
func waitUntil(t *testing.T, what string, done func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); !done(); time.Sleep(10 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting until %s", what)
		}
	}
}
