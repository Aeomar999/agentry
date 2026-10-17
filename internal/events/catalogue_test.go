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
			Data: map[string]any{"title": "Remove TODOs", "repo": "/src/app", "coordinator": "tech-lead"}}, ""},
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
		{"the policy may not create tasks", Proposal{Stream: "task-1", Actor: Actor{Kind: Policy, ID: "rules"}, Type: TaskCreated,
			Data: map[string]any{"title": "x", "repo": "y", "coordinator": "tech-lead"}}, "who"},
		{"a human may not assign: the coordinator does", Proposal{Stream: "task-1", Actor: human, Type: TaskAssigned,
			Data: map[string]any{"agent": "engineer", "instructions": "go"}}, "who"},
		{"nobody proposes a rejection", Proposal{Stream: "task-1", Actor: Actor{System, "x"}, Type: ProposalRejected}, "who"},
		{"a policy load", Proposal{Stream: PolicyStream, Actor: Actor{System, "policy"}, Type: PolicyLoaded,
			Data: map[string]any{"hash": "ab12", "path": "/home/dev/.agentry/policy.toml"}}, ""},
		{"a policy event in a task", Proposal{Stream: "task-1", Actor: Actor{System, "policy"}, Type: PolicyLoaded,
			Data: map[string]any{"hash": "ab12", "path": "/p"}}, "shape"},
		{"a task event in the policy stream", Proposal{Stream: PolicyStream, Actor: human, Type: TaskCreated,
			Data: map[string]any{"title": "x", "repo": "y", "coordinator": "c"}}, "shape"},
		{"a join code", Proposal{Stream: WorkersStream, Actor: human, Type: WorkerInvited,
			Data: map[string]any{"code": "ab12", "expires": "2026-10-17T12:00:00Z"}}, ""},
		{"an agent may not make join codes", Proposal{Stream: WorkersStream, Actor: coordinator, Type: WorkerInvited,
			Data: map[string]any{"code": "ab12", "expires": "2026-10-17T12:00:00Z"}}, "who"},
		{"a worker event in a task", Proposal{Stream: "task-1", Actor: Actor{System, "workers"}, Type: WorkerConnected,
			Data: map[string]any{"name": "laptop"}}, "shape"},
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
	all := []Type{AgentCreated, AgentChanged, AgentRemoved, AgentLearned, AgentRenamed, AgentRenamedInTask, TaskTransferred, TaskRequested, RequestAnswered, ConnectionInvited, ConnectionMade, ConnectionRevoked, FileAdded, FileRemoved, FilesCollected, FilesDelivered, FileFetching, FilesFetched, TaskSplit, StoryDone, EpicWaits, EpicJoined, EpicLeft, StoryAdded, StoryRemoved, TaskCreated, TaskRenamed, TaskDirected, TaskLabeled, TaskBudgeted, TaskRanked, TaskHeld, TaskReopened, RepoChanged, WorktreeReady, TaskAssigned, TurnStarted,
		PromptAcknowledged, SessionStarted, ActionRequested, ActionApproved, ActionAsked, ActionDenied,
		ActionResult, PlanUpdated, AgentWaiting, TurnEnded, IssueRaised, TurnStopped, TaskNeedsYou, TaskAnswered, TaskDone, ProposalRejected,
		PolicyLoaded, PolicyRefused, WorktreeMoved,
		WorkerInvited, WorkerJoined, WorkerConnected, WorkerDisconnected, WorkerRemoved}
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
