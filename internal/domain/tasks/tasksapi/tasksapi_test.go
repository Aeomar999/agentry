package tasksapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks/tasksapi"
)

func TestTasksCarryLabels(t *testing.T) {
	server, _ := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"Fix the login redirect","labels":["Bug"," auth ","bug",""]}`)
	labels := func() []string {
		_, body := call(t, server, "GET", "/api/tasks", "")
		var list []tasks.Summary
		json.Unmarshal([]byte(body), &list)
		return list[0].Labels
	}
	if got := labels(); strings.Join(got, ",") != "bug,auth" {
		t.Errorf("labels at creation: %v", got)
	}
	if code, body := call(t, server, "PUT", "/api/tasks/task-1/labels", `{"labels":["frontend"]}`); code != 200 {
		t.Fatalf("relabel: %d %s", code, body)
	}
	if got := labels(); strings.Join(got, ",") != "frontend" {
		t.Errorf("labels after relabelling: %v", got)
	}
	if code, _ := call(t, server, "PUT", "/api/tasks/task-1/labels", `{"labels":["`+strings.Repeat("x", 40)+`"]}`); code != http.StatusConflict {
		t.Errorf("a label too long: %d", code)
	}
}

// Everything waiting for you, from every task, in one list, oldest first.
func TestQuestionsGatherWhatWaitsForYou(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"Pick a layout"}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Ship it"}`)
	worker := events.Actor{Kind: events.Worker, ID: "laptop"}
	coordinator := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	propose := func(p events.Proposal) {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatalf("%s: %v %v", p.Type, err, rejection)
		}
	}
	for _, id := range []string{"task-1", "task-2"} {
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/" + id, "branch": "b", "port": 7901}})
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}})
	}
	propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "report": "r"}})
	propose(events.Proposal{Stream: "task-1", Actor: coordinator, Type: events.TaskNeedsYou,
		Data: map[string]any{"question": "Which layout?", "options": []string{"Tabs", "One page"}, "page": "<svg></svg>"}})
	propose(events.Proposal{Stream: "task-2", Actor: coordinator, Type: events.ActionRequested, Correlation: "push-1", Data: map[string]any{"tool": "Bash", "command": "git push"}})
	propose(events.Proposal{Stream: "task-2", Actor: events.Actor{Kind: events.Policy, ID: "policy"}, Type: events.ActionAsked, Correlation: "push-1", Data: map[string]any{"reason": "every push asks"}})

	_, body := call(t, server, "GET", "/api/questions", "")
	var questions []tasksapi.Question
	json.Unmarshal([]byte(body), &questions)
	if len(questions) != 2 {
		t.Fatalf("questions: %s", body)
	}
	ask, push := questions[0], questions[1]
	if ask.Kind != "question" || ask.Task != "task-1" || ask.Text != "Which layout?" || len(ask.Options) != 2 || ask.Page != "<svg></svg>" {
		t.Errorf("the question: %+v", ask)
	}
	if push.Kind != "approval" || push.Action != "push-1" || push.Text != "git push" || push.Reason != "every push asks" {
		t.Errorf("the approval: %+v", push)
	}
}

// Tasks are filed under epics, from the page or the desk: made with their
// stories, moved between epics, and taken out.
func TestTasksAreFiledUnderEpics(t *testing.T) {
	server, _ := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Add the store","kind":"story"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Add the command","kind":"story"}`)
	code, body := call(t, server, "POST", "/api/tasks", `{"title":"Ship the TODO tool","stories":["task-1","task-9"]}`)
	if code != 200 || !strings.Contains(body, `"stream":"task-3"`) || !strings.Contains(body, "task-9: no task task-9") {
		t.Fatalf("an epic made with its stories: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/tasks/task-2/epic", `{"epic":"task-3"}`); code != 200 {
		t.Fatalf("file a story: %d %s", code, body)
	}
	_, body = call(t, server, "GET", "/api/tasks/task-3", "")
	if !strings.Contains(body, `"kind":"epic"`) || !strings.Contains(body, `"stories":2`) || !strings.Contains(body, "I added task-2 to this epic") {
		t.Errorf("the epic should have both stories: %s", body)
	}
	if code, body := call(t, server, "PUT", "/api/tasks/task-3/epic", `{"epic":"task-1"}`); code != http.StatusConflict || !strings.Contains(body, "isn't an epic") {
		t.Errorf("filed under a story: %d %s", code, body)
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Another epic","kind":"epic"}`)
	if code, body := call(t, server, "PUT", "/api/tasks/task-2/epic", `{"epic":"task-4"}`); code != 200 {
		t.Fatalf("move a story: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/tasks/task-1/epic", `{"epic":""}`); code != 200 {
		t.Fatalf("take a story out: %d %s", code, body)
	}
	_, body = call(t, server, "GET", "/api/tasks/task-3", "")
	if !strings.Contains(body, `"stories":0`) {
		t.Errorf("the first epic should have no stories left: %s", body)
	}
	_, body = call(t, server, "GET", "/api/tasks/task-2", "")
	if !strings.Contains(body, `"epic":"task-4"`) {
		t.Errorf("task-2 should be in task-4: %s", body)
	}
}
