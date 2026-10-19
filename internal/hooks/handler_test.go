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
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
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

// engineerAtWork is a store and its state with a task whose engineer turn
// (turn-2) is open.
func engineerAtWork(t *testing.T) (*store.Store, *current.State) {
	t.Helper()
	return engineerAtWorkIn(t, "/w/task-1")
}

func engineerAtWorkIn(t *testing.T, worktree string) (*store.Store, *current.State) {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	you := events.Actor{Kind: events.Human, ID: "you"}
	coordinator := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "t", "repo": "/r", "coordinator": "tech-lead"}},
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": worktree, "branch": "b", "port": 7901}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "report": "r"}},
		{Stream: "task-1", Actor: coordinator, Type: events.TaskAssigned, Data: map[string]any{"agent": "engineer", "instructions": "go"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-2",
			Data: map[string]any{"agent": "engineer", "hash": events.Hash("Read .agentry/turn-2.md and do what it says.")}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	return s, state
}

var worker = events.Actor{Kind: events.Worker, ID: "local"}

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
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	h.Handle(context.Background(), hookCall("UserPromptSubmit", `{"hook_event_name":"UserPromptSubmit","prompt":"Read .agentry/turn-2.md and do what it says."}`))
	h.Handle(context.Background(), hookCall("Stop", `{"hook_event_name":"Stop","last_assistant_message":"Removed 3 TODOs."}`))

	task := taskIn(t, s)
	turn := task.Turn("turn-2")
	if !turn.Acknowledged || !turn.Ended || turn.Report != "Removed 3 TODOs." {
		t.Errorf("turn: %+v", turn)
	}
}

func TestTheGateFollowsThePolicy(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
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
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)

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

func TestAnAskThatTimesOutIsHeldForTheUser(t *testing.T) {
	s, state := engineerAtWork(t)
	reply := timedOut(t, NewHandler(s, state, judgeByCommand), "toolu_4", "curl example.com")
	task := taskIn(t, s)
	action := task.Action("toolu_4")
	if decisionOf(reply) != "deny" || !strings.Contains(reply.Stdout, "held for the user's approval") {
		t.Errorf("reply %+v, want a denial that says it is held", reply)
	}
	if !action.Held || action.Decision != tasks.Undecided || task.Status() != tasks.NeedsYou || len(task.HooksWaiting()) != 0 {
		t.Errorf("action %+v, status %s, hooks waiting %+v", action, task.Status(), task.HooksWaiting())
	}
}

// timedOut is the reply to a gate call whose ask gets no answer in time.
func timedOut(t *testing.T, h *Handler, id, command string) hookcall.Reply {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	return h.Handle(ctx, hookCall("PreToolUse", bash(id, command)))
}

func TestALateApprovalLetsTheSameCommandThroughOnce(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	timedOut(t, h, "toolu_4", "git push origin HEAD:dev")
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Human, ID: "you"},
		Type: events.ActionApproved, Correlation: "toolu_4", Data: map[string]any{"reason": "fine"}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	last := taskIn(t, s).Trail[len(taskIn(t, s).Trail)-1]
	if last.To != "tech-lead" || !strings.Contains(last.Text, "git push origin HEAD:dev") {
		t.Errorf("the coordinator should be handed the full command back: %+v", last)
	}

	if reply := timedOut(t, h, "toolu_5", "git push origin HEAD:main"); decisionOf(reply) == "allow" {
		t.Errorf("another command must not use the pass: %+v", reply)
	}
	if reply := h.Handle(context.Background(), hookCall("PreToolUse", bash("toolu_6", "git push origin HEAD:dev"))); decisionOf(reply) != "allow" ||
		!strings.Contains(reply.Stdout, "after it timed out") {
		t.Errorf("the same command should pass once: %+v", reply)
	}
	timedOut(t, h, "toolu_7", "git push origin HEAD:dev")
	if task := taskIn(t, s); !task.Action("toolu_4").PassUsed || !task.Action("toolu_7").Asked {
		t.Errorf("the pass is used once, then the command is asked about again: %+v %+v", task.Action("toolu_4"), task.Action("toolu_7"))
	}
}

func TestALateDenialGivesNoPass(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	timedOut(t, h, "toolu_4", "git push")
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Human, ID: "you"},
		Type: events.ActionDenied, Correlation: "toolu_4", Data: map[string]any{"reason": "no"}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if task := taskIn(t, s); task.PassFor("Bash", "git push") != nil || task.Status() != tasks.Working {
		t.Errorf("pass %+v, status %s", task.PassFor("Bash", "git push"), task.Status())
	}
	timedOut(t, h, "toolu_5", "git push")
	if !taskIn(t, s).Action("toolu_5").Asked {
		t.Error("after a denial the same command is asked about again")
	}
}

// In a task you moved to Done, a timed-out ask can't be held: it is denied.
func TestAnAskThatCantBeHeldIsDenied(t *testing.T) {
	s, state := engineerAtWork(t)
	go func() {
		for len(taskIn(t, s).WaitingForYou()) == 0 {
			time.Sleep(10 * time.Millisecond)
		}
		s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Human, ID: "you"}, Type: events.TaskDone,
			Data: map[string]any{"summary": "Moved.", "moved": true}})
	}()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	reply := NewHandler(s, state, judgeByCommand).Handle(ctx, hookCall("PreToolUse", bash("toolu_4", "curl example.com")))
	if action := taskIn(t, s).Action("toolu_4"); decisionOf(reply) != "deny" || action.Held || action.Decision != tasks.Denied {
		t.Errorf("reply %+v, action %+v", reply, action)
	}
}

func TestOnlyTheWorkingAgentPassesTheGate(t *testing.T) {
	s, state := engineerAtWork(t)
	call := hookCall("PreToolUse", bash("toolu_5", "ls"))
	call.Agent = "reviewer"
	if reply := NewHandler(s, state, judgeByCommand).Handle(context.Background(), call); decisionOf(reply) != "deny" {
		t.Errorf("a call from an agent that isn't working must be denied: %+v", reply)
	}
}

func TestTheHTTPEndpointNeedsTheHookToken(t *testing.T) {
	s, state := engineerAtWork(t)
	server := httptest.NewServer(HTTP(NewHandler(s, state, judgeByCommand), "secret"))
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

func TestARestartHoldsAsksNoHookIsWaitingFor(t *testing.T) {
	s, state := engineerAtWork(t)
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		for len(taskIn(t, s).WaitingForYou()) == 0 {
			time.Sleep(10 * time.Millisecond)
		}
		cancel() // the hook call dies, as when the server stops
	}()
	NewHandler(s, state, judgeByCommand).Handle(ctx, hookCall("PreToolUse", bash("toolu_7", "npm install left-pad")))
	if task := taskIn(t, s); !task.Action("toolu_7").Held {
		t.Fatalf("a hook call that dies holds its own ask for you: %+v", task.Action("toolu_7"))
	}

	// An ask left open (the server was killed outright) is held at the next start.
	s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "engineer"}, Type: events.ActionRequested,
		Correlation: "toolu_8", Data: map[string]any{"tool": "Bash", "command": "curl x"}})
	s.Propose(events.Proposal{Stream: "task-1", Actor: ruleset, Type: events.ActionAsked, Correlation: "toolu_8",
		Data: map[string]any{"reason": "may reach outside"}})
	if err := HoldOrphanedAsks(s, state); err != nil {
		t.Fatal(err)
	}
	if task := taskIn(t, s); !task.Action("toolu_8").Held || task.Action("toolu_8").Decision != tasks.Undecided || len(task.HooksWaiting()) != 0 {
		t.Errorf("the open ask %+v should be held for you, hooks waiting %+v", task.Action("toolu_8"), task.HooksWaiting())
	}
}

func TestTheGateUsesTheWorkingAgentsProfile(t *testing.T) {
	s, state := engineerAtWork(t)
	var asked string
	judge := func(profile string, call policy.Call) policy.Verdict {
		asked = profile
		return policy.Verdict{Decision: policy.Allow, Rule: "test"}
	}
	NewHandler(s, state, judge).Handle(context.Background(), hookCall("PreToolUse", bash("toolu_9", "ls")))
	if asked != "developer" {
		t.Errorf("the engineer's profile is developer, the judge got %q", asked)
	}
}

// A turn whose tokens couldn't be counted says why, rather than a silent 0:
// the reason the hook sent, or that it sent none, as an older worker does.
// A turn.ended from before reasons were kept still applies, with none.
func TestATurnWithUnknownTokensSaysWhy(t *testing.T) {
	for _, c := range []struct {
		ending *hookcall.Ending
		want   string
	}{
		{&hookcall.Ending{TokensUnknown: "transcript not saved"}, "transcript not saved"},
		{&hookcall.Ending{}, NoTokensReported},
		{&hookcall.Ending{Tokens: 1234}, ""},
	} {
		s, state := engineerAtWork(t)
		stop := hookCall("Stop", `{"hook_event_name":"Stop","last_assistant_message":"Done."}`)
		stop.Ending = c.ending
		NewHandler(s, state, judgeByCommand).Handle(context.Background(), stop)
		task := taskIn(t, s)
		if turn := task.Turn("turn-2"); !turn.Ended || turn.TokensUnknown != c.want {
			t.Errorf("ending %+v: turn %+v, want the reason %q", c.ending, turn, c.want)
		}
		if old := task.Turn("turn-1"); !old.Ended || old.TokensUnknown != "" {
			t.Errorf("the turn from an old log: %+v", old)
		}
	}
}

// What the agent wrote in its knowledge file, which the hook sends with the
// call that ends the turn, is kept, and so are the tokens the hook counted.
func TestWhatTheAgentLearnedIsKept(t *testing.T) {
	s, state := engineerAtWorkIn(t, t.TempDir())
	h := NewHandler(s, state, judgeByCommand)
	learnedText := "This repo formats with gofmt -s.\n"
	stop := hookCall("Stop", `{"hook_event_name":"Stop","last_assistant_message":"Done."}`)
	stop.Ending = &hookcall.Ending{Tokens: 1234, Knowledge: &learnedText}
	h.Handle(context.Background(), stop)
	if task := taskIn(t, s); task.TokensUsed != 1234 {
		t.Errorf("tokens used: %d, want what the hook counted", task.TokensUsed)
	}

	stream, _ := s.Stream(events.AgentsStream)
	engineer, _ := agents.FromEvents(stream).Get("engineer")
	if engineer.Knowledge != "This repo formats with gofmt -s." {
		t.Errorf("knowledge: %q", engineer.Knowledge)
	}
	learned := 0
	for _, e := range stream {
		if e.Type == events.AgentLearned {
			learned++
			if e.Actor.ID != "engineer" {
				t.Errorf("recorded as %s, want the engineer", e.Actor)
			}
		}
	}
	if learned != 1 {
		t.Errorf("%d agent.learned events, want 1", learned)
	}
}

func TestThePlanFollowsTheFile(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	withPlan := func(call hookcall.Call, plan string) hookcall.Call {
		call.Plan = &plan
		return call
	}
	result := `{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_use_id":"t1"}`
	h.Handle(context.Background(), withPlan(hookCall("PostToolUse", result), "- [ ] read\n- [ ] fix\n"))
	h.Handle(context.Background(), withPlan(hookCall("PostToolUse", result), "- [ ] read\n- [ ] fix"))
	if plan := taskIn(t, s).Turn("turn-2").Plan; plan != "- [ ] read\n- [ ] fix" {
		t.Errorf("plan %q", plan)
	}

	stop := withPlan(hookCall("Stop", `{"hook_event_name":"Stop","last_assistant_message":"Done."}`), "- [x] read\n- [x] fix")
	h.Handle(context.Background(), stop)
	if turn := taskIn(t, s).Turn("turn-2"); !turn.Ended || turn.Plan != "- [x] read\n- [x] fix" {
		t.Errorf("ended %v with plan %q: the last plan should be kept before the turn ends", turn.Ended, turn.Plan)
	}

	stream, _ := s.Stream("task-1")
	updates := 0
	for _, e := range stream {
		if e.Type == events.PlanUpdated {
			updates++
		}
	}
	if updates != 2 {
		t.Errorf("%d plan.updated events, want 2: an unchanged plan isn't recorded again", updates)
	}
}
