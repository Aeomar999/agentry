package hooks

import (
	"bytes"
	"context"
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
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// judgeByCommand: "ls" is allowed, "sudo" denied, anything else asks.
func judgeByCommand(_ string, call policy.Call) policy.Verdict {
	switch {
	case strings.HasPrefix(call.Input, "ls"):
		return policy.Verdict{Decision: policy.Allow, Rule: "stays inside the worktree"}
	case strings.HasPrefix(call.Input, "sudo"):
		return policy.Verdict{Decision: policy.Deny, Rule: "never sudo"}
	}
	return policy.Verdict{Decision: policy.Ask, Rule: "may reach outside the worktree"}
}

// engineerAtWork is a store with a task whose engineer turn (turn-2) is open.
func engineerAtWork(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), rules.Check)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	you := events.Actor{Kind: events.Human, ID: "you"}
	coordinator := events.Actor{Kind: events.Agent, ID: "coordinator"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "t", "repo": "/r", "coordinator": "coordinator"}},
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/task-1", "branch": "b", "port": 7901}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "coordinator", "hash": "h"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "coordinator", "report": "r"}},
		{Stream: "task-1", Actor: coordinator, Type: events.TaskAssigned, Data: map[string]any{"agent": "engineer", "instructions": "go"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-2",
			Data: map[string]any{"agent": "engineer", "hash": events.Hash("Read .agentry/turn-2.md and do what it says.")}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	return s
}

func hookCall(event, payload string) hookcall.Call {
	return hookcall.Call{Task: "task-1", Turn: "turn-2", Agent: "engineer", Engine: "claude", Event: event, Payload: json.RawMessage(payload)}
}

func bash(id, command string) string {
	return `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_use_id":"` + id + `","tool_input":{"command":"` + command + `"}}`
}

func taskIn(t *testing.T, s *store.Store) *tasks.Task {
	stream, _ := s.Stream("task-1")
	return tasks.FromEvents(stream)
}

func decisionOf(reply hookcall.Reply) string {
	var out struct {
		HookSpecificOutput struct{ PermissionDecision string }
	}
	json.Unmarshal([]byte(reply.Stdout), &out)
	return out.HookSpecificOutput.PermissionDecision
}

func TestReportsBecomeEvents(t *testing.T) {
	s := engineerAtWork(t)
	h := NewHandler(s, judgeByCommand)
	h.Handle(context.Background(), hookCall("UserPromptSubmit", `{"hook_event_name":"UserPromptSubmit","prompt":"Read .agentry/turn-2.md and do what it says."}`))
	h.Handle(context.Background(), hookCall("Stop", `{"hook_event_name":"Stop","last_assistant_message":"Removed 3 TODOs."}`))

	task := taskIn(t, s)
	turn := task.Turn("turn-2")
	if !turn.Acknowledged || !turn.Ended || turn.Report != "Removed 3 TODOs." {
		t.Errorf("turn: %+v", turn)
	}
}

func TestTheGateFollowsThePolicy(t *testing.T) {
	s := engineerAtWork(t)
	h := NewHandler(s, judgeByCommand)
	if reply := h.Handle(context.Background(), hookCall("PreToolUse", bash("toolu_1", "ls -la"))); decisionOf(reply) != "allow" {
		t.Errorf("ls: %+v", reply)
	}
	if reply := h.Handle(context.Background(), hookCall("PreToolUse", bash("toolu_2", "sudo rm x"))); decisionOf(reply) != "deny" || !strings.Contains(reply.Stdout, "never sudo") {
		t.Errorf("sudo: %+v", reply)
	}
	task := taskIn(t, s)
	if task.Action("toolu_1").Decision != tasks.Approved || task.Action("toolu_2").Decision != tasks.Denied || task.Status() != tasks.Working {
		t.Errorf("actions: %+v %+v, status %s", task.Action("toolu_1"), task.Action("toolu_2"), task.Status())
	}
}

func TestAnAskedActionWaitsForTheUser(t *testing.T) {
	s := engineerAtWork(t)
	h := NewHandler(s, judgeByCommand)

	go func() {
		for task := taskIn(t, s); len(task.WaitingForYou()) == 0; task = taskIn(t, s) {
			time.Sleep(10 * time.Millisecond)
		}
		if taskIn(t, s).Status() != tasks.NeedsYou {
			t.Error("an asked action should put the task in Needs you")
		}
		s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Human, ID: "you"},
			Type: events.ActionApproved, Correlation: "toolu_3", Data: map[string]any{"reason": "fine"}})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	reply := h.Handle(ctx, hookCall("PreToolUse", bash("toolu_3", "git push origin HEAD:dev")))
	if decisionOf(reply) != "allow" || !strings.Contains(reply.Stdout, "you approved") {
		t.Errorf("reply: %+v", reply)
	}
}

func TestAnAskThatTimesOutIsDenied(t *testing.T) {
	s := engineerAtWork(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	reply := NewHandler(s, judgeByCommand).Handle(ctx, hookCall("PreToolUse", bash("toolu_4", "curl example.com")))
	task := taskIn(t, s)
	if decisionOf(reply) != "deny" || task.Action("toolu_4").Decision != tasks.Denied || task.Status() != tasks.Working {
		t.Errorf("reply %+v, action %+v, status %s", reply, task.Action("toolu_4"), task.Status())
	}
}

func TestOnlyTheWorkingAgentPassesTheGate(t *testing.T) {
	s := engineerAtWork(t)
	call := hookCall("PreToolUse", bash("toolu_5", "ls"))
	call.Agent = "reviewer"
	if reply := NewHandler(s, judgeByCommand).Handle(context.Background(), call); decisionOf(reply) != "deny" {
		t.Errorf("a call from an agent that isn't working must be denied: %+v", reply)
	}
}

func TestTheHTTPEndpointNeedsTheHookToken(t *testing.T) {
	s := engineerAtWork(t)
	server := httptest.NewServer(HTTP(NewHandler(s, judgeByCommand), "secret"))
	defer server.Close()
	body, _ := json.Marshal(hookCall("PreToolUse", bash("toolu_6", "ls")))

	for token, want := range map[string]int{"wrong": http.StatusUnauthorized, "secret": http.StatusOK} {
		request, _ := http.NewRequest(http.MethodPost, server.URL, bytes.NewReader(body))
		request.Header.Set(hookcall.TokenHeader, token)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != want {
			t.Errorf("token %q: HTTP %d, want %d", token, response.StatusCode, want)
		}
	}
}

func TestARestartDeniesAsksNoHookIsWaitingFor(t *testing.T) {
	s := engineerAtWork(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for len(taskIn(t, s).WaitingForYou()) == 0 {
			time.Sleep(10 * time.Millisecond)
		}
		cancel() // the hook call dies, as when the server stops
	}()
	NewHandler(s, judgeByCommand).Handle(ctx, hookCall("PreToolUse", bash("toolu_7", "npm install left-pad")))
	if task := taskIn(t, s); task.Status() != tasks.Working {
		t.Fatalf("a hook call that dies denies its own ask, got %s", task.Status())
	}

	// An ask left open (the server was killed outright) is denied at the next start.
	s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "engineer"}, Type: events.ActionRequested,
		Correlation: "toolu_8", Data: map[string]any{"tool": "Bash", "command": "curl x"}})
	s.Propose(events.Proposal{Stream: "task-1", Actor: ruleset, Type: events.ActionAsked, Correlation: "toolu_8",
		Data: map[string]any{"reason": "may reach outside"}})
	if err := DenyOrphanedAsks(s); err != nil {
		t.Fatal(err)
	}
	if task := taskIn(t, s); task.Status() != tasks.Working || task.Action("toolu_8").Decision != tasks.Denied {
		t.Errorf("status %s, action %+v", task.Status(), task.Action("toolu_8"))
	}
}

func TestTheGateUsesTheWorkingAgentsProfile(t *testing.T) {
	s := engineerAtWork(t)
	var asked string
	judge := func(profile string, call policy.Call) policy.Verdict {
		asked = profile
		return policy.Verdict{Decision: policy.Allow, Rule: "test"}
	}
	NewHandler(s, judge).Handle(context.Background(), hookCall("PreToolUse", bash("toolu_9", "ls")))
	if asked != "developer" {
		t.Errorf("the engineer's profile is developer, the judge got %q", asked)
	}
}
