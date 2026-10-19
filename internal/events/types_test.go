package events

import "testing"

func TestEveryTypeIsInTheCatalogue(t *testing.T) {
	all := []Type{AgentCreated, AgentChanged, AgentRemoved, AgentLearned, AgentRenamed, AgentRenamedInTask, TaskTransferred, TaskRequested, RequestAnswered, ConnectionInvited, ConnectionMade, ConnectionRevoked, FileAdded, FileRemoved, FilesCollected, FilesDelivered, FileFetching, FilesFetched, TaskSplit, StoryDone, EpicWaits, EpicJoined, EpicLeft, StoryAdded, StoryRemoved, TaskCreated, TaskRenamed, TaskDirected, TaskLabeled, TaskBudgeted, TaskTurnsLimited, TaskRanked, TaskHeld, TaskWrappedUp, TaskReopened, RepoChanged, WorktreeReady, TaskAssigned, TurnStarted,
		PromptAcknowledged, SessionStarted, ActionRequested, ActionApproved, ActionAsked, ActionDenied,
		ActionResult, PlanUpdated, AgentWaiting, TurnEnded, IssueRaised, TurnStopped, TaskNeedsYou, TaskAnswered, TaskDone, ProposalRejected,
		PolicyLoaded, PolicyRefused, WorktreeMoved,
		WorkerInvited, WorkerJoined, WorkerConnected, WorkerDisconnected, WorkerRemoved,
		SettingsChanged, FollowUpsSkipped}
	for _, kind := range all {
		if _, ok := coreTypes[kind]; !ok {
			t.Errorf("%s is not in the catalogue", kind)
		}
	}
	if len(coreTypes) != len(all) {
		t.Errorf("the core has %d types, the test lists %d", len(coreTypes), len(all))
	}
}

func TestDataAccessors(t *testing.T) {
	p := Proposal{Data: map[string]any{"title": "x", "port": float64(7901), "lead": true}}
	if p.Text("title") != "x" || p.Text("port") != "" || p.Number("port") != 7901 || !p.Flag("lead") || p.Flag("missing") {
		t.Errorf("accessors: %q %q %d %v", p.Text("title"), p.Text("port"), p.Number("port"), p.Flag("lead"))
	}
}
