package web

import (
	"bytes"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

var update = flag.Bool("update", false, "rewrite the files in testdata from what the code gives now")

// The page reads these bytes, so they stay as they were whatever builds
// them: tasks in every state a summary shows, the list of them and an
// epic's detail with its stories, against testdata/tasks.json. Times are
// the only thing that changes from run to run, so they are blanked.
func TestTheTaskListKeepsItsJSON(t *testing.T) {
	server, s := newTestServer(t)
	propose := func(p events.Proposal) {
		t.Helper()
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatalf("%s on %s: %v %v", p.Type, p.Stream, err, rejection)
		}
	}
	create := func(body string) {
		t.Helper()
		if code, reply := call(t, server, "POST", "/api/tasks", body); code != 200 {
			t.Fatalf("create %s: %d %s", body, code, reply)
		}
	}
	you := events.Actor{Kind: events.Human, ID: "you"}
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	policy := events.Actor{Kind: events.Policy, ID: "policy"}
	work := func(id, agent string) {
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/" + id, "branch": "agentry/" + id, "port": 7901}})
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": agent, "hash": "h"}})
	}
	endTurn := func(id, agent string) {
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": agent, "report": "r", "tokens": 1200}})
	}

	create(`{"title":"An agent waits","description":"Profile the list","labels":["perf","web"]}`) // task-1
	propose(events.Proposal{Stream: "task-1", Actor: techLead, Type: events.TaskAssigned, Data: map[string]any{"agent": "engineer", "instructions": "Profile it."}})
	work("task-1", "engineer")
	propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.AgentWaiting, Correlation: "turn-1", Data: map[string]any{"question": "Which server?"}})

	create(`{"title":"A question for you"}`) // task-2
	work("task-2", "tech-lead")
	endTurn("task-2", "tech-lead")
	propose(events.Proposal{Stream: "task-2", Actor: techLead, Type: events.TaskNeedsYou, Data: map[string]any{"question": "Tabs or one page?", "options": []string{"Tabs", "One page"}}})

	create(`{"title":"An approval"}`) // task-3
	work("task-3", "tech-lead")
	propose(events.Proposal{Stream: "task-3", Actor: techLead, Type: events.ActionRequested, Correlation: "push-1", Data: map[string]any{"tool": "Bash", "command": "git push"}})
	propose(events.Proposal{Stream: "task-3", Actor: policy, Type: events.ActionAsked, Correlation: "push-1", Data: map[string]any{"reason": "every push asks"}})

	create(`{"title":"In review"}`) // task-4
	propose(events.Proposal{Stream: "task-4", Actor: techLead, Type: events.TaskAssigned, Data: map[string]any{"agent": "reviewer", "instructions": "Check it."}})

	create(`{"title":"Done","repo":""}`) // task-5, shipped
	work("task-5", "tech-lead")
	propose(events.Proposal{Stream: "task-5", Actor: techLead, Type: events.ActionRequested, Correlation: "push-1", Data: map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"}})
	propose(events.Proposal{Stream: "task-5", Actor: policy, Type: events.ActionApproved, Correlation: "push-1", Data: map[string]any{"reason": "otherwise"}})
	propose(events.Proposal{Stream: "task-5", Actor: worker, Type: events.ActionResult, Correlation: "push-1"})
	endTurn("task-5", "tech-lead")
	propose(events.Proposal{Stream: "task-5", Actor: techLead, Type: events.TaskDone, Data: map[string]any{"summary": "Shipped."}})

	create(`{"title":"Parked"}`) // task-6
	if code, reply := call(t, server, "POST", "/api/tasks/task-6/direct", `{"to":"backlog"}`); code != 200 {
		t.Fatalf("park: %d %s", code, reply)
	}

	create(`{"title":"Stopped at its limit","budget":1000,"maxTurns":1}`) // task-7
	work("task-7", "tech-lead")
	endTurn("task-7", "tech-lead")
	propose(events.Proposal{Stream: "task-7", Actor: theLoop, Type: events.TaskNeedsYou, Data: map[string]any{"question": "Go on?", "limit": "budget"}})

	create(`{"title":"First story","kind":"story"}`)            // task-8
	create(`{"title":"Second story","kind":"story"}`)           // task-9
	create(`{"title":"An epic","stories":["task-8","task-9"]}`) // task-10
	propose(events.Proposal{Stream: "task-9", Actor: you, Type: events.TaskLabeled, Data: map[string]any{"labels": []string{"later"}}})
	work("task-8", "tech-lead")
	endTurn("task-8", "tech-lead")
	propose(events.Proposal{Stream: "task-8", Actor: techLead, Type: events.TaskDone, Data: map[string]any{"summary": "The first is in."}})
	propose(events.Proposal{Stream: "task-10", Actor: techLead, Type: events.EpicWaits, Data: map[string]any{"toYou": "The first story is in.", "note": "Waiting."}})
	propose(events.Proposal{Stream: "task-11", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Third story", "repo": "/src/app",
		"coordinator": "tech-lead", "kind": "", "epic": "task-10", "after": []string{"task-8", "task-9"}}})

	create(`{"title":"Ready to close","kind":"epic"}`) // task-12
	propose(events.Proposal{Stream: "task-12", Actor: techLead, Type: events.TaskNeedsYou, Data: map[string]any{
		"question": "Close it?", "options": []string{"Close the epic"}, "closing": "All shipped."}})

	create(`{"title":"Shipping"}`) // task-13
	propose(events.Proposal{Stream: "task-13", Actor: techLead, Type: events.TaskAssigned, Data: map[string]any{"agent": "deployer", "instructions": "Ship it."}})

	create(`{"title":"Moved to Done"}`) // task-14
	if code, reply := call(t, server, "POST", "/api/tasks/task-14/direct", `{"to":"done"}`); code != 200 {
		t.Fatalf("move to done: %d %s", code, reply)
	}

	var got bytes.Buffer
	for _, path := range []string{"/api/tasks", "/api/tasks/task-10", "/api/tasks/task-1"} {
		code, body := call(t, server, "GET", path, "")
		if code != 200 {
			t.Fatalf("%s: %d %s", path, code, body)
		}
		got.WriteString(path + "\n" + body)
	}
	blanked := regexp.MustCompile(`"\d{4}-\d\d-\d\dT[^"]*"`).ReplaceAll(got.Bytes(), []byte(`"(a time)"`))

	golden := filepath.Join("testdata", "tasks.json")
	if *update {
		if err := os.WriteFile(golden, blanked, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(blanked, want) {
		t.Errorf("the JSON differs from %s:\n%s", golden, blanked)
	}
}
