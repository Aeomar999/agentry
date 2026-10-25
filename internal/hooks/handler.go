// Package hooks is the server side of `agentry hook`: it turns each hook call
// into events, and for gate calls decides (or waits for the user to decide)
// whether the tool call may run.
package hooks

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/engines"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// Judge gives the policy's verdict on a call by an agent with a profile.
type Judge func(profile string, call policy.Call) policy.Verdict

type Handler struct {
	store      *store.Store   // where it records what agents do
	state      *current.State // the tasks and agents it reads
	judge      Judge
	redactions *Redactions // the secrets to redact again from what running turns learn
}

func NewHandler(s *store.Store, state *current.State, judge Judge) *Handler {
	return &Handler{store: s, state: state, judge: judge, redactions: newRedactions(state)}
}

// Redactions is where a redaction of the agents' knowledge holds its secret
// for the turns running as it commits.
func (h *Handler) Redactions() *Redactions { return h.redactions }

var ruleset = events.Actor{Kind: events.Policy, ID: "rules"}

// Handle answers one hook call. A gate call that can't be decided is denied:
// the gate fails closed.
func (h *Handler) Handle(ctx context.Context, call hookcall.Call) hookcall.Reply {
	if wait := answerWithin(call.Deadline); wait > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, wait)
		defer cancel()
	}
	engine, ok := engines.Get(call.Engine)
	if !ok {
		return hookcall.Reply{ExitCode: 2} // a gate blocks; a report's `|| true` ignores it
	}
	signals, err := engine.ParseHook(call.Event, call.Payload, time.Time{}) // the start is the session machine's, in call.Ending
	if err != nil {
		return h.refuse(engine, call, "Agentry couldn't read the hook payload: "+err.Error())
	}

	h.resumeIfLimited(call, signals)
	h.recordPlan(call)
	for _, signal := range signals {
		if signal.Kind == engines.ActionRequested && engine.IsGate(call.Event) {
			decision, reason := h.gate(ctx, call, signal)
			return replyOf(engine.GateReply(call.Event, decision, reason))
		}
		h.report(call, signal)
	}
	if engine.IsGate(call.Event) {
		return h.refuse(engine, call, "Agentry got a gate call with no action in it")
	}
	return hookcall.Reply{}
}

// replyMargin is how long before the hook's deadline the server stops
// waiting, to record the hold and get the reply to the hook in time.
const replyMargin = 5 * time.Second

// answerWithin is how long the server may wait on a call whose hook gives
// up after deadline: replyMargin less, or a quarter less when the deadline
// is too short to spare it. 0 means no limit, as when the hook sent none.
func answerWithin(deadline time.Duration) time.Duration {
	if deadline <= 0 {
		return 0
	}
	return deadline - min(replyMargin, deadline/4)
}

// NoTokensReported is why a turn's tokens are unknown when the machine
// running it didn't say, as a worker older than the reason doesn't.
const NoTokensReported = "no tokens reported"

// report records what a reporting hook says happened. A refusal is recorded
// by the store as proposal.rejected; the engine is never blocked by it.
func (h *Handler) report(call hookcall.Call, signal engines.Signal) {
	p := events.Proposal{Stream: call.Task, Actor: h.machineOf(call.Task), Correlation: call.Turn}
	switch signal.Kind {
	case engines.SessionStarted:
		p.Type, p.Data = events.SessionStarted, map[string]any{"engineSession": signal.EngineSession}
	case engines.PromptReceived:
		p.Type, p.Data = events.PromptAcknowledged, map[string]any{"hash": events.Hash(signal.Prompt)}
	case engines.ActionFinished:
		p.Type, p.Correlation = events.ActionResult, cmp.Or(signal.ToolUseID, actionID(call.Turn, signal))
	case engines.Waiting:
		p.Type, p.Data = events.AgentWaiting, map[string]any{"question": signal.Question}
	case engines.TurnEnded:
		tokens, split, unknown, contextTokens, report := signal.Tokens.Total(), &signal.Tokens, signal.TokensUnknown, signal.Context, signal.Report
		if call.Ending != nil { // read where the transcript is
			tokens, split, unknown, contextTokens, report = call.Ending.Tokens, call.Ending.Split, call.Ending.TokensUnknown, call.Ending.Context, cmp.Or(report, call.Ending.Report)
		}
		p.Type, p.Data = events.TurnEnded, map[string]any{"agent": call.Agent, "report": report, "tokens": tokens}
		switch {
		case tokens == 0: // a turn always uses some, so 0 is never a count
			p.Data["tokensUnknown"] = cmp.Or(unknown, NoTokensReported)
		case split != nil: // an older worker sends only the total
			p.Data["inputTokens"], p.Data["cacheWriteTokens"] = split.Input, split.CacheWrite
			p.Data["cacheReadTokens"], p.Data["outputTokens"] = split.CacheRead, split.Output
		}
		if contextTokens > 0 { // unknown, from Codex or an older worker
			p.Data["contextTokens"] = contextTokens
		}
	case engines.UsageLimited:
		resetsAt := ""
		if !signal.ResetsAt.IsZero() {
			resetsAt = signal.ResetsAt.UTC().Format(time.RFC3339)
		}
		p.Type, p.Data = events.TurnLimited, map[string]any{"agent": call.Agent, "error": signal.Error, "details": signal.Details, "resetsAt": resetsAt}
		if reason := h.stillLimited(call); reason != "" {
			p.Type, p.Correlation, p.Data = events.IssueRaised, "", map[string]any{"reason": reason}
		}
	case engines.TurnFailed:
		// An issue ends whichever turn is open, so a late call from a turn
		// that already ended must not end the next one.
		if task, err := h.task(call.Task); err != nil || task.OpenTurn() == nil || task.OpenTurn().ID != call.Turn {
			return
		}
		p.Type, p.Correlation, p.Data = events.IssueRaised, "", map[string]any{"reason": failedTurnReason(signal)}
	default:
		return
	}
	h.store.Propose(p)
	if signal.Kind == engines.TurnEnded {
		h.endTurn(call)
	}
}

// endTurn keeps what the agent left in its knowledge file, if the hook could
// read it, with each secret you redacted while the turn ran redacted again.
// If one would still show, none of it is kept; nor is anything from a turn
// on an archived task, which an older log can hold: the state shows no
// turn of it, so no redaction held it.
func (h *Handler) endTurn(call hookcall.Call) {
	var learned string
	if call.Ending != nil && call.Ending.Knowledge != nil {
		learned = strings.ToValidUTF8(*call.Ending.Knowledge, "\uFFFD") // as recordWhatWasLearned reads it
	}
	learned, clean := h.redactions.Ended(Turn{call.Task, call.Turn}, learned)
	switch {
	case call.Ending == nil || call.Ending.Knowledge == nil:
	case h.archived(call.Task):
		slog.Warn("hooks: a turn ended on an archived task, so nothing it learned is kept", "task", call.Task, "turn", call.Turn)
	case !clean:
		slog.Warn("hooks: a redacted secret would still show in what a turn learned, so none of it is kept", "task", call.Task, "turn", call.Turn)
	default:
		h.recordWhatWasLearned(call.Agent, strings.TrimSpace(learned))
	}
}

// archived says whether the state holds the task as its summary alone.
func (h *Handler) archived(id string) bool {
	_, err := h.state.Task(id)
	return errors.Is(err, current.ErrArchived)
}

// stillLimited says why a turn that hits the usage limit again, straight
// after Agentry retried it at the reset, is an issue: it is retried once,
// never waited for again and again. A turn that called a tool since the
// retry did work, so its new limit is waited out as any other. "" when
// the limit is one to wait for.
func (h *Handler) stillLimited(call hookcall.Call) string {
	task, err := h.task(call.Task)
	if err != nil {
		return ""
	}
	turn := task.OpenTurn()
	if turn == nil || turn.ID != call.Turn || turn.RetriedAt.IsZero() {
		return ""
	}
	return fmt.Sprintf("%s's %s is still at the usage limit after the retry at ~%s", turn.Agent, turn.ID, turn.RetriedAt.Local().Format("15:04"))
}

// failedTurnReason says what stopped the turn, in the engine's own words
// when it gave some.
func failedTurnReason(signal engines.Signal) string {
	reason := "the turn stopped at an API error (" + signal.Error + ")"
	if details := strings.TrimSpace(signal.Details); details != "" {
		reason += ": " + details
	}
	return reason
}

// resumeIfLimited records that a turn waiting for a usage limit goes on when
// its engine works again: the session carried on by itself, so the turn is
// watched as a working one again. A session starting or ending, or the turn
// stopping at the limit or an error, isn't work.
func (h *Handler) resumeIfLimited(call hookcall.Call, signals []engines.Signal) {
	if !slices.ContainsFunc(signals, isWork) {
		return
	}
	task, err := h.task(call.Task)
	if err != nil {
		return
	}
	if turn := task.OpenTurn(); turn == nil || turn.ID != call.Turn || turn.Limited == "" {
		return
	}
	h.store.Propose(events.Proposal{Stream: call.Task, Actor: h.machineOf(call.Task), Type: events.TurnResumed,
		Correlation: call.Turn, Data: map[string]any{"agent": call.Agent, "reason": "engine"}})
}

// isWork says the signal comes from an engine that is working.
func isWork(signal engines.Signal) bool {
	switch signal.Kind {
	case engines.PromptReceived, engines.ActionRequested, engines.ActionFinished, engines.TurnEnded, engines.Waiting:
		return true
	}
	return false
}

// recordPlan keeps the turn's plan when the agent has changed it. It goes
// before what the call reports, so a plan ticked off at the end is kept
// before the turn ends.
func (h *Handler) recordPlan(call hookcall.Call) {
	if call.Plan == nil {
		return
	}
	plan := strings.TrimSpace(*call.Plan)
	task, err := h.task(call.Task)
	if err != nil {
		return
	}
	if turn := task.Turn(call.Turn); turn == nil || turn.Ended || turn.Plan == plan {
		return
	}
	h.store.Propose(events.Proposal{Stream: call.Task, Actor: h.machineOf(call.Task), Type: events.PlanUpdated,
		Correlation: call.Turn, Data: map[string]any{"plan": plan}})
}

// machineOf is the worker actor for the machine the task's session runs on,
// which reports what happens in it.
func (h *Handler) machineOf(id string) events.Actor {
	machine := workers.Local
	if task, err := h.task(id); err == nil {
		machine = task.Worktree.Worker
	}
	return events.Actor{Kind: events.Worker, ID: machine}
}

// recordWhatWasLearned keeps what the agent wrote in its knowledge file this
// turn, if it changed anything.
func (h *Handler) recordWhatWasLearned(name, learned string) {
	if h.state.Broken(events.AgentsStream) != nil {
		return
	}
	// The stored knowledge went through JSON, which makes it valid UTF-8, so
	// text with a bad byte would never equal it and every turn would write
	// another full copy.
	learned = strings.ToValidUTF8(learned, "\uFFFD")
	agent, ok := h.state.Roster().Get(name)
	if ok && learned != strings.TrimSpace(agent.Knowledge) {
		h.keepLearned(agent, learned)
	}
}

// keepLearned proposes learned as the agent's knowledge, with agent as the
// hook read it. The rules refuse a patch made from a version replaced since,
// as when you edit the knowledge on the page meanwhile, and then a full copy
// goes instead.
func (h *Handler) keepLearned(agent agents.Agent, learned string) {
	p := learning(agent, learned)
	if _, rejection, err := h.store.Propose(p); err == nil && rejection != nil && agents.IsPatch(p) {
		h.store.Propose(agent.Learned(p.Actor, learned))
	}
}

// learning is the event that sets the agent's knowledge to learned: a patch
// on what it knows, or a full copy for its first version and whenever the
// patches since its last full copy would add up to more than the text.
// Rebuilding any version then reads at most its last full copy and its own
// length again (Target 9 in docs/design/log-growth.md).
func learning(agent agents.Agent, learned string) events.Proposal {
	by := events.Actor{Kind: events.Agent, ID: agent.Name}
	patch := agents.Diff(agent.Knowledge, learned)
	if agent.Knowledge == "" || agent.Patched()+len(patch.Middle) > len(learned) {
		return agent.Learned(by, learned)
	}
	return agent.Patching(by, patch)
}

func (h *Handler) refuse(engine engines.Engine, call hookcall.Call, reason string) hookcall.Reply {
	if !engine.IsGate(call.Event) {
		return hookcall.Reply{}
	}
	return replyOf(engine.GateReply(call.Event, engines.Deny, reason))
}

func replyOf(r engines.Reply) hookcall.Reply {
	return hookcall.Reply{Stdout: r.Stdout, ExitCode: r.ExitCode}
}

// gate records the request, asks the policy, and returns the decision the
// agent gets. When the policy asks the user, it waits for their answer.
func (h *Handler) gate(ctx context.Context, call hookcall.Call, signal engines.Signal) (engines.Decision, string) {
	action := cmp.Or(signal.ToolUseID, actionID(call.Turn, signal))
	requested := events.Proposal{Stream: call.Task, Actor: events.Actor{Kind: events.Agent, ID: call.Agent},
		Type: events.ActionRequested, Correlation: action, Data: map[string]any{"tool": signal.Tool, "command": signal.Command}}
	if _, rejection, err := h.store.Propose(requested); err != nil || rejection != nil {
		return engines.Deny, fmt.Sprintf("Agentry refused the request: %v", firstError(err, rejection))
	}

	task, err := h.task(call.Task)
	if err != nil {
		return h.decide(call.Task, action, refusal("Agentry couldn't read the task: "+err.Error()))
	}
	verdict := h.judge(h.profileOf(call.Agent), policy.Call{Tool: signal.Tool, Input: signal.Command, Workspace: task.Worktree.Path})
	if verdict.Decision != policy.Ask {
		return h.decide(call.Task, action, verdict)
	}
	if pass := task.PassFor(task.Action(action)); pass != nil {
		if answer, reason, ok := h.usePass(call.Task, action, pass.ID); ok {
			return answer, reason
		}
	}
	return h.askTheUser(ctx, call.Task, action, verdict)
}

// actionID ties an action's request to its result when the engine gives no
// id of its own (Claude Code before 2.1): the turn, the tool and the command.
func actionID(turn string, signal engines.Signal) string {
	return fmt.Sprintf("%s-%s", turn, events.Hash(signal.Tool+signal.Command))
}

func refusal(reason string) policy.Verdict {
	return policy.Verdict{Decision: policy.Deny, Rule: reason}
}

// verdictData is what a decision records: the reason, and which policy decided.
func verdictData(v policy.Verdict) map[string]any {
	data := map[string]any{"reason": v.Reason()}
	if v.Policy != "" {
		data["policy"] = v.Policy
	}
	return data
}

// decide records the policy's own decision and returns it for the engine.
func (h *Handler) decide(task, action string, verdict policy.Verdict) (engines.Decision, string) {
	kind, answer := events.ActionApproved, engines.Allow
	if verdict.Decision != policy.Allow {
		kind, answer = events.ActionDenied, engines.Deny
	}
	h.store.Propose(events.Proposal{Stream: task, Actor: ruleset, Type: kind, Correlation: action, Data: verdictData(verdict)})
	return answer, "Agentry: " + verdict.Reason()
}

// usePass approves the action with the pass of a held action you approved
// late, in place of asking you again: the agent asks for the very command
// again, as its coordinator handed it back. It reports false if the pass
// was refused, and you are asked as usual.
func (h *Handler) usePass(task, action, pass string) (engines.Decision, string, bool) {
	const reason = "the user approved this earlier, after it timed out"
	_, rejection, err := h.store.Propose(events.Proposal{Stream: task, Actor: ruleset, Type: events.ActionApproved,
		Correlation: action, Data: map[string]any{"reason": reason, "pass": pass}})
	if err != nil || rejection != nil {
		return "", "", false
	}
	return engines.Allow, "Agentry: " + reason, true
}

// Held is what the agent is told when no answer came in time.
const Held = "Agentry: no answer yet; the command is held for the user's approval. Don't retry it now; carry on with " +
	"other work or report back. If the user approves it, your coordinator will hand it back."

// askTheUser records action.asked, which puts the task in Needs you, then waits
// for the user's decision. If the wait runs out (just before the deadline
// the hook sent, or the hook gave up or was killed), the agent is told no
// and the action is held for the user, so the task doesn't wait on it.
func (h *Handler) askTheUser(ctx context.Context, task, action string, verdict policy.Verdict) (engines.Decision, string) {
	asked, rejection, err := h.store.Propose(events.Proposal{Stream: task, Actor: ruleset, Type: events.ActionAsked,
		Correlation: action, Data: verdictData(verdict)})
	if err != nil || rejection != nil {
		return h.decide(task, action, refusal(fmt.Sprintf("couldn't ask you: %v", firstError(err, rejection))))
	}

	reader := h.store.NewReader(asked.Position)
	defer reader.Close()
	for {
		batch, err := reader.Next(ctx)
		if err != nil {
			return h.hold(task, action)
		}
		for _, e := range batch {
			if e.Stream != task || e.Correlation != action {
				continue
			}
			switch e.Type {
			case events.ActionApproved:
				return engines.Allow, "Agentry: you approved: " + e.Text("reason")
			case events.ActionDenied:
				return engines.Deny, "Agentry: you denied: " + e.Text("reason")
			}
		}
	}
}

// hold keeps the action waiting for you after the wait ran out. One
// that can't be held, as in a task you moved to Done, is denied.
func (h *Handler) hold(task, action string) (engines.Decision, string) {
	_, rejection, err := h.store.Propose(events.Proposal{Stream: task, Actor: ruleset, Type: events.ActionHeld, Correlation: action})
	if err != nil || rejection != nil {
		return h.decide(task, action, refusal("no decision before the hook's deadline; ask again"))
	}
	return engines.Deny, Held
}

// profileOf is the agent's policy profile, "" (the default) if it has none.
func (h *Handler) profileOf(name string) string {
	if h.state.Broken(events.AgentsStream) != nil {
		return ""
	}
	agent, _ := h.state.Roster().Get(name)
	return agent.Profile
}

func (h *Handler) task(id string) (*tasks.Task, error) {
	task, err := h.state.Task(id)
	if err != nil {
		return nil, err
	}
	if task == nil || task.Worktree == nil {
		return nil, fmt.Errorf("task %s has no worktree", id)
	}
	return task, nil
}

func firstError(err error, rejection *events.Rejection) error {
	if err != nil {
		return err
	}
	return rejection
}
