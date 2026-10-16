// Package hooks is the server side of `agentry hook`: it turns each hook call
// into events, and for gate calls decides (or waits for the user to decide)
// whether the tool call may run.
package hooks

import (
	"context"
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/engines"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// Judge gives the policy's verdict on a call by an agent with a profile.
type Judge func(profile string, call policy.Call) policy.Verdict

type Handler struct {
	store *store.Store
	judge Judge
}

func NewHandler(s *store.Store, judge Judge) *Handler {
	return &Handler{store: s, judge: judge}
}

var (
	worker  = events.Actor{Kind: events.Worker, ID: "local"}
	ruleset = events.Actor{Kind: events.Policy, ID: "rules"}
)

// Handle answers one hook call. A gate call that can't be decided is denied:
// the gate fails closed.
func (h *Handler) Handle(ctx context.Context, call hookcall.Call) hookcall.Reply {
	engine, ok := engines.Get(call.Engine)
	if !ok {
		return hookcall.Reply{ExitCode: 2} // a gate blocks; a report's `|| true` ignores it
	}
	signals, err := engine.ParseHook(call.Event, call.Payload)
	if err != nil {
		return h.refuse(engine, call, "Agentry couldn't read the hook payload: "+err.Error())
	}

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

// report records what a reporting hook says happened. A refusal is recorded
// by the store as proposal.rejected; the engine is never blocked by it.
func (h *Handler) report(call hookcall.Call, signal engines.Signal) {
	p := events.Proposal{Stream: call.Task, Actor: worker, Correlation: call.Turn}
	switch signal.Kind {
	case engines.SessionStarted:
		p.Type, p.Data = events.SessionStarted, map[string]any{"engineSession": signal.EngineSession}
	case engines.PromptReceived:
		p.Type, p.Data = events.PromptAcknowledged, map[string]any{"hash": events.Hash(signal.Prompt)}
	case engines.ActionFinished:
		p.Type, p.Correlation = events.ActionResult, signal.ToolUseID
	case engines.Waiting:
		p.Type, p.Data = events.AgentWaiting, map[string]any{"question": signal.Question}
	case engines.TurnEnded:
		p.Type, p.Data = events.TurnEnded, map[string]any{"agent": call.Agent, "report": signal.Report}
	default:
		return
	}
	h.store.Propose(p)
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
	action := signal.ToolUseID
	if action == "" {
		action = fmt.Sprintf("%s-%s-%s", call.Turn, call.Event, events.Hash(signal.Tool+signal.Command))
	}
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
	return h.askTheUser(ctx, call.Task, action, verdict)
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

// askTheUser records action.asked, which puts the task in Needs you, then waits
// for the user's decision. If the hook gives up first (its deadline, or the
// engine killed it), the action is denied so the task doesn't wait forever.
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
			return h.decide(task, action, refusal("no decision before the hook's deadline; ask again"))
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

// profileOf is the agent's policy profile, "" (the default) if it has none.
func (h *Handler) profileOf(name string) string {
	stream, err := h.store.Stream(events.AgentsStream)
	if err != nil {
		return ""
	}
	agent, _ := agents.FromEvents(stream).Get(name)
	return agent.Profile
}

func (h *Handler) task(id string) (*tasks.Task, error) {
	stream, err := h.store.Stream(id)
	if err != nil {
		return nil, err
	}
	task := tasks.FromEvents(stream)
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
