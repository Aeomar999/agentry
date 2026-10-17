package web

import (
	"encoding/json"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

func TestAMessageStopsTheTurnForTheCoordinator(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Add a cache"}`)
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	coordinator := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "report": "r"}},
		{Stream: "task-1", Actor: coordinator, Type: events.TaskAssigned, Data: map[string]any{"agent": "engineer", "instructions": "go"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-2", Data: map[string]any{"agent": "engineer", "hash": "h"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}

	if code, body := call(t, server, "POST", "/api/tasks/task-1/message", `{"text":"Use Redis, not memory."}`); code != 200 {
		t.Fatalf("message: %d %s", code, body)
	}
	stream, _ := s.Stream("task-1")
	task := tasks.FromEvents(stream)
	turn := task.Turn("turn-2")
	if !turn.Ended || !turn.Stopped || turn.Issue != "" || task.IssuesInARow != 0 {
		t.Errorf("the engineer's turn should be stopped, not failed: %+v, issues in a row %d", turn, task.IssuesInARow)
	}
	trail := task.Trail[len(task.Trail)-2:]
	if trail[0].From != tasks.You || trail[0].Text != "Use Redis, not memory." || trail[1].To != "tech-lead" || !strings.Contains(trail[1].Text, "carry on") {
		t.Errorf("the coordinator should read what you said, then why the engineer stopped: %+v", trail)
	}
	if task.Handoff == nil || task.Handoff.To != "tech-lead" {
		t.Errorf("the coordinator should be next: %+v", task.Handoff)
	}

	// With no turn running, a message is only what you said.
	call(t, server, "POST", "/api/tasks/task-1/message", `{"text":"And keep it simple."}`)
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if last := detail.Trail[len(detail.Trail)-1]; last.Text != "And keep it simple." {
		t.Errorf("last step: %+v", last)
	}
}

func TestTheCoordinatorsReplyComesBeforeItsDecision(t *testing.T) {
	task := tasks.FromEvents([]events.Event{
		{Stream: "task-1", Type: events.TaskCreated, Actor: events.Actor{Kind: events.Human, ID: "you"},
			Data: map[string]any{"title": "t", "repo": "/r", "coordinator": "tech-lead"}},
		{Stream: "task-1", Type: events.TaskAssigned, Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"},
			Data: map[string]any{"agent": "engineer", "instructions": "go", "toYou": "Switched to Redis."}},
	})
	trail := task.Trail[1:]
	if len(trail) != 2 || trail[0].To != tasks.You || trail[0].Text != "Switched to Redis." || trail[1].To != "engineer" {
		t.Errorf("trail: %+v", trail)
	}
	if task.Handoff == nil || task.Handoff.To != "engineer" {
		t.Errorf("the handoff is still to the engineer: %+v", task.Handoff)
	}
}
