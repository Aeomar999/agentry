package hooks

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/platform/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
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
	if reply := timedOut(t, h, "toolu_1", "ls -la"); decisionOf(reply) != "allow" {
		t.Errorf("ls: %+v", reply)
	}
	if reply := timedOut(t, h, "toolu_2", "sudo rm x"); decisionOf(reply) != "deny" || !strings.Contains(reply.Stdout, "never sudo") {
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

// The server stops waiting a margin before the hook's deadline, so its
// reply gets there first; a short deadline spares a quarter of itself.
func TestTheServerAnswersBeforeTheHooksDeadline(t *testing.T) {
	for deadline, want := range map[time.Duration]time.Duration{
		0:                               0,
		-time.Second:                    0,
		time.Second:                     750 * time.Millisecond,
		20 * time.Second:                15 * time.Second,
		29*time.Minute + 50*time.Second: 29*time.Minute + 45*time.Second,
	} {
		if got := answerWithin(deadline); got != want {
			t.Errorf("answerWithin(%s) = %s, want %s", deadline, got, want)
		}
	}
}

func TestAnAskIsHeldBeforeTheDeadlineTheHookSent(t *testing.T) {
	s, state := engineerAtWork(t)
	call := hookCall("PreToolUse", bash("toolu_4", "curl example.com"))
	call.Deadline = time.Second
	// The test's own context ends after the deadline, so a late reply fails fast.
	ctx, cancel := context.WithTimeout(context.Background(), 2*call.Deadline)
	defer cancel()
	started := time.Now()
	reply := NewHandler(s, state, judgeByCommand).Handle(ctx, call)
	if elapsed := time.Since(started); elapsed >= call.Deadline {
		t.Errorf("answered after %s, past the hook's deadline of %s", elapsed, call.Deadline)
	}
	if decisionOf(reply) != "deny" || !strings.Contains(reply.Stdout, "Don't retry") || !taskIn(t, s).Action("toolu_4").Held {
		t.Errorf("reply %+v, want the action held", reply)
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
	approveLate(t, s, "toolu_4")
	last := taskIn(t, s).Trail[len(taskIn(t, s).Trail)-1]
	if last.To != "tech-lead" || !strings.Contains(last.Text, "git push origin HEAD:dev") {
		t.Errorf("the coordinator should be handed the full command back: %+v", last)
	}
	if reply := timedOut(t, h, "toolu_5", "git push origin HEAD:dev"); decisionOf(reply) == "allow" {
		t.Errorf("the pass waits for the engineer's next turn: %+v", reply)
	}

	handBackTo(t, s, "engineer", "turn-3", "turn-4")
	if reply := timedOut(t, h, "toolu_6", "git push origin HEAD:main"); decisionOf(reply) == "allow" {
		t.Errorf("another command must not use the pass: %+v", reply)
	}
	if reply := timedOut(t, h, "toolu_7", "git push origin HEAD:dev"); decisionOf(reply) != "allow" ||
		!strings.Contains(reply.Stdout, "the user approved this earlier") {
		t.Errorf("the same command should pass once: %+v", reply)
	}
	timedOut(t, h, "toolu_8", "git push origin HEAD:dev")
	if task := taskIn(t, s); !task.Action("toolu_4").PassUsed || !task.Action("toolu_8").Asked {
		t.Errorf("the pass is used once, then the command is asked about again: %+v %+v", task.Action("toolu_4"), task.Action("toolu_8"))
	}
}

// A pass skips only the question to the user: a policy that denies the
// command now still denies it, and leaves the pass for a later ask.
func TestAPassDoesntOverrideADenial(t *testing.T) {
	s, state := engineerAtWork(t)
	verdict := policy.Ask
	h := NewHandler(s, state, func(string, policy.Call) policy.Verdict {
		return policy.Verdict{Decision: verdict, Rule: "pushes are judged"}
	})
	timedOut(t, h, "toolu_4", "git push")
	approveLate(t, s, "toolu_4")
	handBackTo(t, s, "engineer", "turn-3", "turn-4")

	verdict = policy.Deny
	if reply := timedOut(t, h, "toolu_5", "git push"); decisionOf(reply) != "deny" {
		t.Errorf("a denial stays a denial: %+v", reply)
	}
	if taskIn(t, s).Action("toolu_4").PassUsed {
		t.Error("a denied request must leave the pass unused")
	}
	verdict = policy.Ask
	if reply := timedOut(t, h, "toolu_6", "git push"); decisionOf(reply) != "allow" {
		t.Errorf("a later ask can still use the pass: %+v", reply)
	}
}

func TestAPassIsOnlyForTheAgentThatAsked(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	timedOut(t, h, "toolu_4", "git push")
	approveLate(t, s, "toolu_4")
	handBackTo(t, s, "reviewer", "turn-3", "turn-4")

	call := hookCall("PreToolUse", bash("toolu_5", "git push"))
	call.Turn, call.Agent = "turn-4", "reviewer"
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if reply := h.Handle(ctx, call); decisionOf(reply) == "allow" {
		t.Errorf("another agent's identical request must not use the pass: %+v", reply)
	}
	if task := taskIn(t, s); !task.Action("toolu_5").Asked || task.Action("toolu_4").PassUsed {
		t.Errorf("the reviewer is asked as usual: %+v %+v", task.Action("toolu_5"), task.Action("toolu_4"))
	}
}

func TestAPassEndsWithTheAgentsNextTurn(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	timedOut(t, h, "toolu_4", "git push")
	approveLate(t, s, "toolu_4")
	handBackTo(t, s, "engineer", "turn-3", "turn-4") // the engineer doesn't push in turn-4
	handBackTo(t, s, "engineer", "turn-5", "turn-6")

	if reply := timedOut(t, h, "toolu_5", "git push"); decisionOf(reply) == "allow" {
		t.Errorf("the pass ended with turn-4: %+v", reply)
	}
	if task := taskIn(t, s); !task.Action("toolu_5").Asked || task.Action("toolu_4").PassUsed {
		t.Errorf("the request is asked about again: %+v %+v", task.Action("toolu_5"), task.Action("toolu_4"))
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
	if task := taskIn(t, s); task.Status() != tasks.Working {
		t.Errorf("status %s", task.Status())
	}
	handBackTo(t, s, "engineer", "turn-3", "turn-4")
	timedOut(t, h, "toolu_5", "git push")
	if !taskIn(t, s).Action("toolu_5").Asked {
		t.Error("after a denial the same command is asked about again")
	}
}

// approveLate is you approving a held action.
func approveLate(t *testing.T, s *store.Store, action string) {
	t.Helper()
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Human, ID: "you"},
		Type: events.ActionApproved, Correlation: action, Data: map[string]any{"reason": "fine"}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
}

// handBackTo ends the open turn, runs a coordinator turn (decide), and
// starts agent's turn (next).
func handBackTo(t *testing.T, s *store.Store, agent, decide, next string) {
	t.Helper()
	open := taskIn(t, s).OpenTurn()
	coordinator := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: open.ID, Data: map[string]any{"agent": open.Agent, "report": "r"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: decide, Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: decide, Data: map[string]any{"agent": "tech-lead", "report": "r"}},
		{Stream: "task-1", Actor: coordinator, Type: events.TaskAssigned, Data: map[string]any{"agent": agent, "instructions": "go"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: next, Data: map[string]any{"agent": agent, "hash": "h"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
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

// After the first version, the agent's knowledge is kept as patches, until
// they would add up to more than the text: then a full copy goes again.
func TestLaterVersionsOfWhatTheAgentLearnedArePatches(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	for _, c := range []struct {
		learned string
		patch   bool
	}{
		{"Use gofmt.", false}, // the first version
		{"Use gofmt.\nRun go vet.", true},
		{"Use gofmt -s.\nRun go vet.", true},      // patched 13 + 3, of 26
		{"Use gofmt -s.\nRun go vet ./...", true}, // and 5, of 30
		{"Run the race detector.", false},         // 22 more would pass the text's own length
		{"Run the race detector on CI.", true},
	} {
		h.recordWhatWasLearned("engineer", c.learned)
		stream, _ := s.Stream(events.AgentsStream)
		last := stream[len(stream)-1]
		if last.Type != events.AgentLearned || agents.IsPatch(last.Proposal) != c.patch {
			t.Errorf("%q: kept as %s %v, want a patch: %v", c.learned, last.Type, last.Data, c.patch)
		}
		if engineer, _ := state.Roster().Get("engineer"); engineer.Knowledge != c.learned {
			t.Errorf("%q: the engineer knows %q", c.learned, engineer.Knowledge)
		}
		if engineer, _ := agents.FromEvents(stream).Get("engineer"); engineer.Knowledge != c.learned {
			t.Errorf("%q: a replay knows %q", c.learned, engineer.Knowledge)
		}
	}
}

// You edit the knowledge on the page between the hook reading it and
// proposing what the agent learned: the patch made from what the hook read
// is refused, and a full copy of the agent's text lands instead.
func TestAStalePatchIsSentAgainAsAFullCopy(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	h.recordWhatWasLearned("engineer", "Use gofmt.")
	read, _ := state.Roster().Get("engineer")
	if _, rejection, err := s.Propose(read.Learned(events.Actor{Kind: events.Human, ID: "you"}, "Use tabs.")); err != nil || rejection != nil {
		t.Fatal(err, rejection)
	}
	h.keepLearned(read, "Use gofmt.\nRun go vet.")

	stream, _ := s.Log(events.AgentsStream)
	if rejected := stream[len(stream)-2]; rejected.Type != events.ProposalRejected {
		t.Errorf("before the full copy: %s %v, want the patch refused", rejected.Type, rejected.Data)
	}
	if last := stream[len(stream)-1]; last.Type != events.AgentLearned || agents.IsPatch(last.Proposal) || last.Actor.ID != "engineer" {
		t.Errorf("last: %s %v by %s, want the engineer's full copy", last.Type, last.Data, last.Actor)
	}
	if engineer, _ := state.Roster().Get("engineer"); engineer.Knowledge != "Use gofmt.\nRun go vet." {
		t.Errorf("the engineer knows %q", engineer.Knowledge)
	}
}

// A patch with a byte that isn't UTF-8 reaches the rules: they refuse it,
// and a full copy of the text lands instead.
func TestAPatchThatIsNotUTF8IsSentAgainAsAFullCopy(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	h.recordWhatWasLearned("engineer", "Use gofmt.")
	read, _ := state.Roster().Get("engineer")
	h.keepLearned(read, "Use gofmt.\nRun go vet \xff.")

	stream, _ := s.Log(events.AgentsStream)
	if rejected := stream[len(stream)-2]; rejected.Type != events.ProposalRejected || !strings.Contains(rejected.Text("reason"), "UTF-8") {
		t.Errorf("before the full copy: %s %v, want the patch refused for its UTF-8", rejected.Type, rejected.Data)
	}
	if last := stream[len(stream)-1]; last.Type != events.AgentLearned || agents.IsPatch(last.Proposal) || last.Actor.ID != "engineer" {
		t.Errorf("last: %s %v by %s, want the engineer's full copy", last.Type, last.Data, last.Actor)
	}
	if engineer, _ := state.Roster().Get("engineer"); engineer.Knowledge != "Use gofmt.\nRun go vet \uFFFD." {
		t.Errorf("the engineer knows %q", engineer.Knowledge)
	}
}

// The agent writes a byte that isn't UTF-8 in its knowledge file and leaves
// it there: the knowledge keeps U+FFFD in its place, and the next turn, with
// the same text, writes nothing.
func TestTextThatIsNotUTF8IsKeptOnce(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	h.recordWhatWasLearned("engineer", "Run go vet \xff.")
	before, _ := s.Log(events.AgentsStream)
	h.recordWhatWasLearned("engineer", "Run go vet \xff.")

	if after, _ := s.Log(events.AgentsStream); len(after) != len(before) {
		t.Errorf("the same text again added %v, want nothing", after[len(before):])
	}
	if engineer, _ := state.Roster().Get("engineer"); engineer.Knowledge != "Run go vet \uFFFD." {
		t.Errorf("the engineer knows %q", engineer.Knowledge)
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

// Claude Code's StopFailure at a usage limit keeps the turn open with when
// the limit resets; it raises no issue.
func TestAUsageLimitIsRecordedWithItsResetTime(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	h.Handle(context.Background(), hookCall("StopFailure",
		`{"hook_event_name":"StopFailure","error":"rate_limit","last_assistant_message":"You've hit your session limit · resets 3am (UTC)"}`))

	task := taskIn(t, s)
	turn := task.OpenTurn()
	if turn == nil || turn.ID != "turn-2" || turn.ResetsAt.IsZero() || turn.ResetsAt.Hour() != 3 || turn.ResetsAt.Location() != time.UTC {
		t.Fatalf("the turn should stay open, limited until 3am UTC: %+v", turn)
	}
	stream, _ := s.Stream("task-1")
	last := stream[len(stream)-1]
	if last.Type != events.TurnLimited || last.Text("resetsAt") != turn.ResetsAt.Format(time.RFC3339) || last.Text("error") != "rate_limit" {
		t.Errorf("last event: %+v", last)
	}
	if task.IssuesInARow != 0 {
		t.Errorf("issues in a row: %d", task.IssuesInARow)
	}
}

// A limited turn whose engine carries on by itself, with a tool call, a
// prompt or its end, resumes first, so it is watched as working again. A
// call that says nothing, or the limit again, leaves it limited.
func TestEngineActivityResumesALimitedTurn(t *testing.T) {
	limit := hookCall("StopFailure", `{"hook_event_name":"StopFailure","error":"rate_limit","error_details":"You've hit your session limit · resets 3am (UTC)"}`)
	cases := map[string]struct {
		call    hookcall.Call
		resumes bool
		then    events.Type // the call's own event, after the resume
	}{
		"a tool call":     {hookCall("PreToolUse", `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"ls"},"tool_use_id":"t1"}`), true, events.ActionRequested},
		"a prompt":        {hookCall("UserPromptSubmit", `{"hook_event_name":"UserPromptSubmit","prompt":"Carry on."}`), true, ""},
		"the end":         {hookCall("Stop", `{"hook_event_name":"Stop","last_assistant_message":"Done."}`), true, events.TurnEnded},
		"idle":            {hookCall("Notification", `{"hook_event_name":"Notification","notification_type":"idle_prompt"}`), false, ""},
		"the limit":       {limit, false, ""},
		"a session start": {hookCall("SessionStart", `{"hook_event_name":"SessionStart","session_id":"s2","source":"resume"}`), false, ""},
		"a session end":   {hookCall("SessionEnd", `{"hook_event_name":"SessionEnd","reason":"clear"}`), false, ""},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			s, state := engineerAtWork(t)
			h := NewHandler(s, state, judgeByCommand)
			h.Handle(context.Background(), limit)
			h.Handle(context.Background(), c.call)

			stream, _ := s.Stream("task-1")
			resumed := slices.IndexFunc(stream, func(e events.Event) bool { return e.Type == events.TurnResumed })
			if (resumed >= 0) != c.resumes {
				t.Fatalf("resumed at %d, want a resume: %v", resumed, c.resumes)
			}
			if !c.resumes {
				if taskIn(t, s).OpenTurn().Limited == "" {
					t.Errorf("the turn should still be limited")
				}
				return
			}
			if e := stream[resumed]; e.Text("reason") != "engine" || e.Text("agent") != "engineer" || e.Correlation != "turn-2" || e.Actor.Kind != events.Worker {
				t.Errorf("resumed: %+v", e)
			}
			if c.then != "" && !slices.ContainsFunc(stream[resumed+1:], func(e events.Event) bool { return e.Type == c.then }) {
				t.Errorf("the resume should come before the call's own %s: %+v", c.then, stream[resumed-1:])
			}
			if turn := taskIn(t, s).Turn("turn-2"); turn.Limited != "" || turn.Resumes != 1 {
				t.Errorf("turn %+v", turn)
			}
		})
	}
}

// An API error on a limited turn is its issue, not a sign the engine goes on.
func TestAnErrorOnALimitedTurnIsAnIssueNotAResume(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	h.Handle(context.Background(), hookCall("StopFailure", `{"hook_event_name":"StopFailure","error":"rate_limit","error_details":"You've hit your session limit · resets 3am (UTC)"}`))
	h.Handle(context.Background(), hookCall("StopFailure", `{"hook_event_name":"StopFailure","error":"server_error"}`))

	stream, _ := s.Stream("task-1")
	if slices.ContainsFunc(stream, func(e events.Event) bool { return e.Type == events.TurnResumed }) {
		t.Errorf("an error resumed the limited turn: %+v", stream)
	}
	if !slices.ContainsFunc(stream, func(e events.Event) bool { return e.Type == events.IssueRaised }) {
		t.Errorf("the error raised no issue: %+v", stream)
	}
}

// Any other StopFailure ends the turn at once as an issue with what Claude
// Code said, so nobody waits for the watchdog. A late one from a turn that
// already ended doesn't end the next.
func TestAnotherStopFailureIsAnIssue(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	stale := hookCall("StopFailure", `{"hook_event_name":"StopFailure","error":"overloaded"}`)
	stale.Turn = "turn-1"
	h.Handle(context.Background(), stale)
	if task := taskIn(t, s); task.OpenTurn() == nil || task.IssuesInARow != 0 {
		t.Fatalf("a call from an ended turn changed the open one: %+v", task.Turns)
	}

	h.Handle(context.Background(), hookCall("StopFailure",
		`{"hook_event_name":"StopFailure","error":"overloaded","last_assistant_message":"API Error: Overloaded"}`))
	task := taskIn(t, s)
	turn := task.Turn("turn-2")
	if !turn.Ended || turn.Issue != "the turn stopped at an API error (overloaded): API Error: Overloaded" || task.IssuesInARow != 1 {
		t.Errorf("turn %+v, issues in a row %d", turn, task.IssuesInARow)
	}
}

// retriedAtTheReset is the engineer's turn-2 stopped at a usage limit and
// retried by Agentry when it reset.
func retriedAtTheReset(t *testing.T) (*store.Store, *Handler) {
	t.Helper()
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	h.Handle(context.Background(), hookCall("StopFailure", `{"hook_event_name":"StopFailure","error":"rate_limit","error_details":"You've hit your session limit · resets 3am (UTC)"}`))
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.System, ID: "watchdog"},
		Type: events.TurnResumed, Correlation: "turn-2", Data: map[string]any{"agent": "engineer", "reason": "reset"}}); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	return s, h
}

// A turn retried at the reset that hits the limit again before doing any
// work is retried once only: it ends as an issue, not another wait.
func TestALimitStraightAfterTheRetryIsAnIssue(t *testing.T) {
	s, h := retriedAtTheReset(t)
	retried := taskIn(t, s).Turn("turn-2").RetriedAt
	h.Handle(context.Background(), hookCall("UserPromptSubmit", `{"hook_event_name":"UserPromptSubmit","prompt":"Your Claude usage limit has reset."}`))
	h.Handle(context.Background(), hookCall("StopFailure", `{"hook_event_name":"StopFailure","error":"rate_limit","error_details":"You've hit your session limit · resets 8am (UTC)"}`))

	task := taskIn(t, s)
	want := "engineer's turn-2 is still at the usage limit after the retry at ~" + retried.Local().Format("15:04")
	if turn := task.Turn("turn-2"); retried.IsZero() || !turn.Ended || turn.Issue != want || task.IssuesInARow != 1 {
		t.Errorf("turn %+v, issues in a row %d", turn, task.IssuesInARow)
	}
}

// A turn that called a tool after the retry did work, so a later limit is a
// new one, waited out as the first was.
func TestALimitAfterWorkSinceTheRetryIsWaitedOut(t *testing.T) {
	s, h := retriedAtTheReset(t)
	h.Handle(context.Background(), hookCall("PreToolUse", bash("toolu_9", "ls")))
	h.Handle(context.Background(), hookCall("StopFailure", `{"hook_event_name":"StopFailure","error":"rate_limit","error_details":"You've hit your session limit · resets 8am (UTC)"}`))

	task := taskIn(t, s)
	if turn := task.OpenTurn(); turn == nil || turn.Limited == "" || !turn.RetriedAt.IsZero() || task.IssuesInARow != 0 {
		t.Errorf("turn %+v, issues in a row %d", turn, task.IssuesInARow)
	}
}
