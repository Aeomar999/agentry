package events

import "testing"

func TestCheckShapeAndWriter(t *testing.T) {
	human := Actor{Human, "you"}
	coordinator := Actor{Agent, "coordinator"}
	cases := []struct {
		name      string
		proposal  Proposal
		wantCheck string // "" means accepted
	}{
		{"a valid task", Proposal{Stream: "task-1", Actor: human, Type: TaskCreated,
			Data: map[string]any{"title": "Remove TODOs", "repo": "/src/app", "coordinator": "coordinator"}}, ""},
		{"unknown type", Proposal{Stream: "task-1", Actor: human, Type: "task.exploded"}, "shape"},
		{"missing field", Proposal{Stream: "task-1", Actor: human, Type: TaskCreated,
			Data: map[string]any{"title": "Remove TODOs"}}, "shape"},
		{"missing actor", Proposal{Stream: "task-1", Type: TaskCreated,
			Data: map[string]any{"title": "x", "repo": "y"}}, "shape"},
		{"missing correlation", Proposal{Stream: "task-1", Actor: Actor{Worker, "local"}, Type: TurnStarted,
			Data: map[string]any{"agent": "engineer", "hash": "ab"}}, "shape"},
		{"agent event outside the agents stream", Proposal{Stream: "task-1", Actor: human, Type: AgentCreated,
			Data: map[string]any{"name": "x", "prompt": "y", "engine": "claude"}}, "shape"},
		{"task event in the agents stream", Proposal{Stream: AgentsStream, Actor: human, Type: TaskCreated,
			Data: map[string]any{"title": "x", "repo": "y"}}, "shape"},
		{"an agent may not create tasks", Proposal{Stream: "task-1", Actor: coordinator, Type: TaskCreated,
			Data: map[string]any{"title": "x", "repo": "y", "coordinator": "coordinator"}}, "who"},
		{"a human may not assign: the coordinator does", Proposal{Stream: "task-1", Actor: human, Type: TaskAssigned,
			Data: map[string]any{"agent": "engineer", "instructions": "go"}}, "who"},
		{"nobody proposes a rejection", Proposal{Stream: "task-1", Actor: Actor{System, "x"}, Type: ProposalRejected}, "who"},
		{"a policy load", Proposal{Stream: PolicyStream, Actor: Actor{System, "policy"}, Type: PolicyLoaded,
			Data: map[string]any{"hash": "ab12", "path": "/home/dev/.agentry/policy.toml"}}, ""},
		{"a policy event in a task", Proposal{Stream: "task-1", Actor: Actor{System, "policy"}, Type: PolicyLoaded,
			Data: map[string]any{"hash": "ab12", "path": "/p"}}, "shape"},
		{"a task event in the policy stream", Proposal{Stream: PolicyStream, Actor: human, Type: TaskCreated,
			Data: map[string]any{"title": "x", "repo": "y", "coordinator": "c"}}, "shape"},
		{"the policy asks", Proposal{Stream: "task-1", Actor: Actor{Policy, "rules"}, Type: ActionAsked, Correlation: "toolu_1",
			Data: map[string]any{"reason": "may reach outside the worktree"}}, ""},
	}
	for _, c := range cases {
		rejection := CheckShape(c.proposal)
		if rejection == nil {
			rejection = CheckWriter(c.proposal)
		}
		got := ""
		if rejection != nil {
			got = rejection.Check
		}
		if got != c.wantCheck {
			t.Errorf("%s: got check %q (%v), want %q", c.name, got, rejection, c.wantCheck)
		}
	}
}

func TestEveryTypeIsInTheCatalogue(t *testing.T) {
	all := []Type{AgentCreated, AgentChanged, AgentRemoved, TaskCreated, TaskRenamed, TaskDirected, TaskLabeled, WorktreeReady, TaskAssigned, TurnStarted,
		PromptAcknowledged, SessionStarted, ActionRequested, ActionApproved, ActionAsked, ActionDenied,
		ActionResult, AgentWaiting, TurnEnded, IssueRaised, TaskNeedsYou, TaskAnswered, TaskDone, ProposalRejected,
		PolicyLoaded, PolicyRefused}
	for _, kind := range all {
		if _, ok := catalogue[kind]; !ok {
			t.Errorf("%s is not in the catalogue", kind)
		}
	}
	if len(catalogue) != len(all) {
		t.Errorf("catalogue has %d types, the test lists %d", len(catalogue), len(all))
	}
}

func TestDataAccessors(t *testing.T) {
	p := Proposal{Data: map[string]any{"title": "x", "port": float64(7901), "lead": true}}
	if p.Text("title") != "x" || p.Text("port") != "" || p.Number("port") != 7901 || !p.Flag("lead") || p.Flag("missing") {
		t.Errorf("accessors: %q %q %d %v", p.Text("title"), p.Text("port"), p.Number("port"), p.Flag("lead"))
	}
}
