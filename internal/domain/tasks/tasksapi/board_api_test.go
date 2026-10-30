package tasksapi_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// The board's columns are the work's stages: a card goes onto any of them,
// and the coordinator still makes the move.
func TestCardsMoveThroughTheStages(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"Fix the login redirect"}`)
	show := func() taskDetail {
		_, body := call(t, server, "GET", "/api/tasks/task-1", "")
		var detail taskDetail
		json.Unmarshal([]byte(body), &detail)
		return detail
	}
	lastStep := func(d taskDetail) string { return d.Trail[len(d.Trail)-1].Text }
	move := func(to, note string) int {
		code, _ := call(t, server, "POST", "/api/tasks/task-1/direct", `{"to":"`+to+`","note":"`+note+`"}`)
		return code
	}

	move("backlog", "")
	if d := show(); d.Status != "held" || d.Parked != "backlog" {
		t.Errorf("onto Backlog: %s, parked %q", d.Status, d.Parked)
	}
	move("todo", "")
	if d := show(); d.Parked != "todo" {
		t.Errorf("onto To do: parked %q", d.Parked)
	}
	steps := len(show().Trail)
	move("in-progress", "")
	if d := show(); d.Parked != "" || d.Status != "working" || len(d.Trail) != steps {
		t.Errorf("started: parked %q, %s, %d steps (want %d: nothing to say)", d.Parked, d.Status, len(d.Trail), steps)
	}
	move("review", "Check the redirect on Safari.")
	if d := show(); lastStep(d) != "Have this task reviewed next. Check the redirect on Safari." {
		t.Errorf("onto Review: %q", lastStep(d))
	}
	if code := move("sideways", ""); code != http.StatusBadRequest {
		t.Errorf("a column that doesn't exist: %d", code)
	}

	// Done, then back for more work.
	worker, coordinator := events.Actor{Kind: events.Worker, ID: "laptop"}, events.Actor{Kind: events.Agent, ID: "tech-lead"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "report": "r"}},
		{Stream: "task-1", Actor: coordinator, Type: events.TaskDone, Data: map[string]any{"summary": "Fixed."}},
	} {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatalf("%s: %v %v", p.Type, err, rejection)
		}
	}
	if code := move("done", ""); code != http.StatusConflict {
		t.Errorf("a done task onto Done: %d", code)
	}
	move("in-progress", "The fix broke sign-out.")
	if d := show(); d.Status != "working" || d.Worktree != nil || lastStep(d) != "Reopen this task. This needs more work: hand it back to an agent that builds it. The fix broke sign-out." {
		t.Errorf("reopened: %s, worktree %v, %q", d.Status, d.Worktree, lastStep(d))
	}
}

// Your moves to Done, To do and the Backlog stand in the same request: the
// turn in progress stops, and the coordinator only wraps up.
func TestYourMovesOnTheBoardStand(t *testing.T) {
	server, s := newTestServer(t)
	worker := events.Actor{Kind: events.Worker, ID: "laptop"}
	working := func(id string) {
		for _, p := range []events.Proposal{
			{Stream: id, Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/" + id, "branch": "b", "port": 7901}},
			{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		} {
			if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
				t.Fatalf("%s: %v %v", p.Type, err, rejection)
			}
		}
	}
	move := func(id, to string) {
		if code, body := call(t, server, "POST", "/api/tasks/"+id+"/direct", `{"to":"`+to+`","note":"Enough."}`); code != 200 {
			t.Fatalf("%s onto %s: %d %s", id, to, code, body)
		}
	}
	load := func(id string) *tasks.Task {
		stream, _ := s.Stream(id)
		return tasks.FromEvents(stream)
	}

	call(t, server, "POST", "/api/tasks", `{"description":"Fix the login redirect"}`)
	working("task-1")
	move("task-1", "done")
	if task := load("task-1"); task.Status() != tasks.Done || !task.DoneByYou || !task.Turns[0].Stopped || task.WrapUp == nil {
		t.Errorf("onto Done: %s, by you %v, turn stopped %v, wrap-up %+v", task.Status(), task.DoneByYou, task.Turns[0].Stopped, task.WrapUp)
	}

	call(t, server, "POST", "/api/tasks", `{"description":"Tidy the README"}`)
	working("task-2")
	move("task-2", "todo")
	if task := load("task-2"); task.Status() != tasks.Held || task.Parked != "todo" || !task.Turns[0].Stopped || task.WrapUp == nil {
		t.Errorf("onto To do: %s, parked %q, turn stopped %v, wrap-up %+v", task.Status(), task.Parked, task.Turns[0].Stopped, task.WrapUp)
	}

	call(t, server, "POST", "/api/tasks", `{"title":"Ship it","kind":"epic"}`)
	move("task-3", "done")
	if task := load("task-3"); !task.IsDone {
		t.Errorf("an epic onto Done should close: %s", task.Status())
	}

	// An epic onto Done parks its open stories, in the same request.
	call(t, server, "POST", "/api/tasks", `{"title":"Add the store","kind":"story"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Add the command","kind":"story"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Ship the tool","stories":["task-4","task-5"]}`)
	working("task-4")
	move("task-6", "done")
	if task := load("task-6"); !task.IsDone {
		t.Errorf("the epic should close: %s", task.Status())
	}
	if task := load("task-4"); task.Parked != "backlog" || !task.Turns[0].Stopped || task.WrapUp == nil {
		t.Errorf("the story at work: parked %q, turn stopped %v, wrap-up %+v", task.Parked, task.Turns[0].Stopped, task.WrapUp)
	}
	if task := load("task-5"); task.Parked == "" || task.WrapUp != nil {
		t.Errorf("the story not started: parked %q, wrap-up %+v", task.Parked, task.WrapUp)
	}
}

// Your moves to In progress and Review stand in the same request, from
// any column, until the coordinator hands the task on: then its column
// follows the agent that has it.
func TestYourMovesToInProgressAndReviewStand(t *testing.T) {
	server, s := newTestServer(t)
	worker, lead := events.Actor{Kind: events.Worker, ID: "laptop"}, events.Actor{Kind: events.Agent, ID: "tech-lead"}
	propose := func(p events.Proposal) *events.Rejection {
		_, rejection, err := s.Propose(p)
		if err != nil {
			t.Fatal(err)
		}
		return rejection
	}
	handOn := func(id, turn, agent string) *events.Rejection {
		for _, p := range []events.Proposal{
			{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
			{Stream: id, Actor: worker, Type: events.TurnEnded, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "report": "r"}},
		} {
			if rejection := propose(p); rejection != nil {
				t.Fatalf("%s: %v", p.Type, rejection)
			}
		}
		return propose(events.Proposal{Stream: id, Actor: lead, Type: events.TaskAssigned,
			Data: map[string]any{"agent": agent, "instructions": "go", "checks": agent == "reviewer"}})
	}
	move := func(id, to string) {
		if code, body := call(t, server, "POST", "/api/tasks/"+id+"/direct", `{"to":"`+to+`"}`); code != 200 {
			t.Fatalf("%s onto %s: %d %s", id, to, code, body)
		}
	}
	column := func(id string) string {
		_, body := call(t, server, "GET", "/api/tasks", "")
		var list []taskSummary
		json.Unmarshal([]byte(body), &list)
		for _, summary := range list {
			if summary.ID == id {
				return summary.Column
			}
		}
		return "missing"
	}

	call(t, server, "POST", "/api/tasks", `{"description":"Fix the login redirect"}`)
	propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}})
	if rejection := handOn("task-1", "turn-1", "engineer"); rejection != nil || column("task-1") != "in-progress" {
		t.Fatalf("with the engineer: %v, column %s", rejection, column("task-1"))
	}
	move("task-1", "review")
	if c := column("task-1"); c != "review" {
		t.Errorf("onto Review: column %s", c)
	}
	if rejection := handOn("task-1", "turn-2", "engineer"); rejection == nil ||
		rejection.Reason != "the user moved this task to Review; hand it to an agent that checks work" {
		t.Errorf("handed back to the engineer: %v", rejection)
	}
	if rejection := propose(events.Proposal{Stream: "task-1", Actor: lead, Type: events.TaskAssigned,
		Data: map[string]any{"agent": "reviewer", "instructions": "check it", "checks": true}}); rejection != nil || column("task-1") != "review" {
		t.Errorf("to the reviewer: %v, column %s", rejection, column("task-1"))
	}
	if rejection := handOn("task-1", "turn-3", "engineer"); rejection != nil || column("task-1") != "in-progress" {
		t.Errorf("your move is over: the next handoff is the coordinator's: %v, column %s", rejection, column("task-1"))
	}
	handOn("task-1", "turn-4", "reviewer")
	move("task-1", "in-progress")
	if c := column("task-1"); c != "in-progress" {
		t.Errorf("sent back from Review: column %s", c)
	}
	if rejection := handOn("task-1", "turn-5", "tester"); rejection == nil ||
		rejection.Reason != "the user moved this task back to In progress; hand it to an agent that builds it" {
		t.Errorf("handed to the tester: %v", rejection)
	}
	if rejection := propose(events.Proposal{Stream: "task-1", Actor: lead, Type: events.TaskAssigned,
		Data: map[string]any{"agent": "engineer", "instructions": "fix it"}}); rejection != nil || column("task-1") != "in-progress" {
		t.Errorf("to the engineer: %v, column %s", rejection, column("task-1"))
	}

	// From Done: reopened, in the column you chose.
	call(t, server, "POST", "/api/tasks", `{"description":"Tidy the README"}`)
	propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w2", "branch": "b", "port": 7902}})
	handOn("task-2", "turn-1", "reviewer")
	propose(events.Proposal{Stream: "task-2", Actor: lead, Type: events.TaskDone, Data: map[string]any{"summary": "Tidied."}})
	move("task-2", "in-progress")
	if c := column("task-2"); c != "in-progress" {
		t.Errorf("from Done onto In progress: column %s", c)
	}
	move("task-2", "done")
	move("task-2", "review")
	if c := column("task-2"); c != "review" {
		t.Errorf("from Done onto Review: column %s", c)
	}

	// From To do: started, in the column you chose.
	call(t, server, "POST", "/api/tasks", `{"description":"Add a logo"}`)
	move("task-3", "todo")
	move("task-3", "review")
	if c := column("task-3"); c != "review" {
		t.Errorf("from To do onto Review: column %s", c)
	}
	move("task-3", "backlog")
	if c := column("task-3"); c != "backlog" {
		t.Errorf("parked again: column %s", c)
	}
}

// Moving done work, or work a checker had before you parked it, back to
// In progress sends it back for more work: not to a checker again.
func TestBackToInProgressFromDoneOrParkedNeedsMoreWork(t *testing.T) {
	server, s := newTestServer(t)
	worker, lead := events.Actor{Kind: events.Worker, ID: "laptop"}, events.Actor{Kind: events.Agent, ID: "tech-lead"}
	propose := func(p events.Proposal) *events.Rejection {
		_, rejection, err := s.Propose(p)
		if err != nil {
			t.Fatal(err)
		}
		return rejection
	}
	handOn := func(id, turn, agent string) *events.Rejection {
		for _, p := range []events.Proposal{
			{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
			{Stream: id, Actor: worker, Type: events.TurnEnded, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "report": "r"}},
		} {
			if rejection := propose(p); rejection != nil {
				t.Fatalf("%s: %v", p.Type, rejection)
			}
		}
		return propose(events.Proposal{Stream: id, Actor: lead, Type: events.TaskAssigned,
			Data: map[string]any{"agent": agent, "instructions": "go", "checks": agent == "reviewer"}})
	}
	move := func(id, to string) {
		if code, body := call(t, server, "POST", "/api/tasks/"+id+"/direct", `{"to":"`+to+`"}`); code != 200 {
			t.Fatalf("%s onto %s: %d %s", id, to, code, body)
		}
	}
	load := func(id string) *tasks.Task {
		stream, _ := s.Stream(id)
		return tasks.FromEvents(stream)
	}
	const moreWork = "This needs more work: hand it back to an agent that builds it."

	call(t, server, "POST", "/api/tasks", `{"description":"Fix the login redirect"}`)
	propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}})
	handOn("task-1", "turn-1", "engineer")
	propose(events.Proposal{Stream: "task-1", Actor: lead, Type: events.TaskDone, Data: map[string]any{"summary": "Fixed."}})
	move("task-1", "in-progress")
	if task := load("task-1"); !task.SentBack || !strings.Contains(task.Handoff.Text, moreWork) {
		t.Errorf("from Done: sent back %v, handoff %+v", task.SentBack, task.Handoff)
	}
	propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}})
	if rejection := handOn("task-1", "turn-2", "reviewer"); rejection == nil ||
		rejection.Reason != "the user moved this task back to In progress; hand it to an agent that builds it" {
		t.Errorf("reopened, handed to the reviewer: %v", rejection)
	}

	call(t, server, "POST", "/api/tasks", `{"description":"Tidy the README"}`)
	propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w2", "branch": "b", "port": 7902}})
	handOn("task-2", "turn-1", "reviewer")
	move("task-2", "todo")
	move("task-2", "in-progress")
	if task := load("task-2"); !task.SentBack || task.Trail[len(task.Trail)-1].Text != moreWork {
		t.Errorf("parked after review: sent back %v, last step %+v", task.SentBack, task.Trail[len(task.Trail)-1])
	}
}

// A task an agent that ships work has shows in Shipping. Moving a card
// there ships it now: the move stands at once, until the coordinator hands
// it to an agent of its team that ships work; a team with none refuses it.
// Moved out again, it keeps the other columns' meaning.
func TestYourMoveToShippingShipsItNow(t *testing.T) {
	server, s := newTestServer(t)
	worker, lead := events.Actor{Kind: events.Worker, ID: "laptop"}, events.Actor{Kind: events.Agent, ID: "tech-lead"}
	propose := func(p events.Proposal) *events.Rejection {
		_, rejection, err := s.Propose(p)
		if err != nil {
			t.Fatal(err)
		}
		return rejection
	}
	handOn := func(id, turn, agent string) *events.Rejection {
		for _, p := range []events.Proposal{
			{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
			{Stream: id, Actor: worker, Type: events.TurnEnded, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "report": "r"}},
		} {
			if rejection := propose(p); rejection != nil {
				t.Fatalf("%s: %v", p.Type, rejection)
			}
		}
		return propose(events.Proposal{Stream: id, Actor: lead, Type: events.TaskAssigned,
			Data: map[string]any{"agent": agent, "instructions": "go", "checks": agent == "reviewer", "ships": agent == "deployer"}})
	}
	move := func(id, to string) {
		if code, body := call(t, server, "POST", "/api/tasks/"+id+"/direct", `{"to":"`+to+`"}`); code != 200 {
			t.Fatalf("%s onto %s: %d %s", id, to, code, body)
		}
	}
	load := func(id string) *tasks.Task {
		stream, _ := s.Stream(id)
		return tasks.FromEvents(stream)
	}
	summary := func(id string) taskSummary {
		_, body := call(t, server, "GET", "/api/tasks", "")
		var list []taskSummary
		json.Unmarshal([]byte(body), &list)
		for _, summary := range list {
			if summary.ID == id {
				return summary
			}
		}
		return taskSummary{Column: "missing"}
	}
	column := func(id string) string { return summary(id).Column }

	// The coordinator hands it to the deployer: Shipping, with no move of yours.
	call(t, server, "POST", "/api/tasks", `{"description":"Fix the login redirect"}`)
	propose(events.Proposal{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}})
	handOn("task-1", "turn-1", "deployer")
	if c, task := column("task-1"), load("task-1"); c != "shipping" || task.Column != "" {
		t.Errorf("with the deployer: column %s, your column %q", c, task.Column)
	}
	if !summary("task-1").CanShip {
		t.Error("a team with a deployer can ship")
	}

	// Shipped now, from In progress: your move, written at once.
	call(t, server, "POST", "/api/tasks", `{"description":"Tidy the README"}`)
	propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w2", "branch": "b", "port": 7902}})
	handOn("task-2", "turn-1", "engineer")
	before, _ := s.Stream("task-2")
	move("task-2", "shipping")
	stream, _ := s.Stream("task-2")
	first := stream[len(before)]
	if first.Actor.Kind != events.Human || first.Text("column") != "shipping" || column("task-2") != "shipping" {
		t.Errorf("onto Shipping: first event %s by %s with column %q, board column %s", first.Type, first.Actor.Kind, first.Text("column"), column("task-2"))
	}
	if text := load("task-2").Trail[len(load("task-2").Trail)-1].Text; !strings.Contains(text, "hand it to deployer") ||
		!strings.Contains(text, "maybe before it was reviewed or tested") || !strings.Contains(text, "needs no earlier review") || !strings.Contains(text, "a failure stops the push") {
		t.Errorf("the coordinator is told: %q", text)
	}
	if rejection := handOn("task-2", "turn-2", "engineer"); rejection == nil ||
		rejection.Reason != "the user moved this task to Shipping; hand it to an agent that ships work" {
		t.Errorf("handed to the engineer: %v", rejection)
	}
	if rejection := handOn("task-2", "turn-3", "reviewer"); rejection == nil || column("task-2") != "shipping" {
		t.Errorf("handed to the reviewer: %v, column %s", rejection, column("task-2"))
	}
	if rejection := propose(events.Proposal{Stream: "task-2", Actor: lead, Type: events.TaskAssigned,
		Data: map[string]any{"agent": "deployer", "instructions": "ship it", "ships": true}}); rejection != nil {
		t.Fatalf("to the deployer: %v", rejection)
	}
	if task := load("task-2"); column("task-2") != "shipping" || task.Column != "" || !task.Handoff.Ships {
		t.Errorf("with the deployer you asked for: column %s, your column %q, handoff %+v", column("task-2"), task.Column, task.Handoff)
	}
	propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.TurnStarted, Correlation: "turn-4", Data: map[string]any{"agent": "deployer", "hash": "h"}})
	if task := load("task-2"); !task.Turns[len(task.Turns)-1].Shipped {
		t.Errorf("the deployer's turn ships it for you: %+v", task.Turns[len(task.Turns)-1])
	}
	propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.TurnEnded, Correlation: "turn-4", Data: map[string]any{"agent": "deployer", "report": "Shipped."}})

	// Out of Shipping: back to In progress is more work.
	move("task-2", "in-progress")
	if task := load("task-2"); !task.SentBack || task.Column != "in-progress" {
		t.Errorf("sent back from Shipping: sent back %v, column %q", task.SentBack, task.Column)
	}
	if rejection := handOn("task-2", "turn-5", "deployer"); rejection == nil ||
		rejection.Reason != "the user moved this task back to In progress; hand it to an agent that builds it" {
		t.Errorf("sent back, handed to the deployer: %v", rejection)
	}
	handOn("task-2", "turn-6", "engineer")
	move("task-2", "shipping")
	move("task-2", "review")
	if task := load("task-2"); column("task-2") != "review" || task.SentBack {
		t.Errorf("from Shipping onto Review: column %s, sent back %v", column("task-2"), task.SentBack)
	}
	move("task-2", "shipping")
	move("task-2", "done")
	if task := load("task-2"); !task.DoneByYou || task.WrapUp == nil || column("task-2") != "done" {
		t.Errorf("from Shipping onto Done: by you %v, wrap-up %+v, column %s", task.DoneByYou, task.WrapUp, column("task-2"))
	}

	// From Done and from To do: reopened or started, in Shipping.
	move("task-2", "shipping")
	if task := load("task-2"); task.IsDone || column("task-2") != "shipping" {
		t.Errorf("from Done onto Shipping: done %v, column %s", task.IsDone, column("task-2"))
	}
	move("task-2", "todo")
	move("task-2", "shipping")
	if task := load("task-2"); task.Parked != "" || column("task-2") != "shipping" {
		t.Errorf("from To do onto Shipping: parked %q, column %s", task.Parked, column("task-2"))
	}

	// A team with no agent that ships work refuses the move, and writes nothing.
	if code, body := call(t, server, "POST", "/api/teams", `{"template":"research"}`); code != 200 {
		t.Fatalf("adding the Research team: %d %s", code, body)
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Size the market","team":"Research"}`)
	before, _ = s.Stream("task-3")
	code, body := call(t, server, "POST", "/api/tasks/task-3/direct", `{"to":"shipping"}`)
	after, _ := s.Stream("task-3")
	if code != http.StatusConflict || !strings.Contains(body, "Research has no agent that ships work") || len(after) != len(before) {
		t.Errorf("a team with no shipper: %d %q, %d events written", code, body, len(after)-len(before))
	}
	if _, body := call(t, server, "GET", "/api/tasks", ""); summary("task-3").CanShip || !strings.Contains(body, `"canShip":false`) {
		t.Errorf("a team with no shipper says so, so the board offers no Shipping:\n%s", body)
	}
}

// A push held for you in Shipping says where it goes, not the command.
func TestAHeldPushSaysWhereItGoes(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Ship the fix"}`)
	worker, deployer := events.Actor{Kind: events.Worker, ID: "local"}, events.Actor{Kind: events.Agent, ID: "deployer"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"}, Type: events.TaskAssigned,
			Data: map[string]any{"agent": "deployer", "instructions": "ship it", "ships": true}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "deployer", "hash": "h"}},
		{Stream: "task-1", Actor: deployer, Type: events.ActionRequested, Correlation: "toolu_1",
			Data: map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Policy, ID: "rules"}, Type: events.ActionAsked, Correlation: "toolu_1",
			Data: map[string]any{"reason": "every push"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	_, body := call(t, server, "GET", "/api/tasks", "")
	if !strings.Contains(body, `"column":"shipping"`) || !strings.Contains(body, `"needsYou":"Waiting for you to approve the push to dev"`) {
		t.Errorf("the held push should read plainly in Shipping:\n%s", body)
	}
}

// An epic you move to Done, whether it is asking you to close it or you
// left it open, is closed by the move: nothing is left to ask again.
func TestAnEpicOntoDoneLeavesNothingToClose(t *testing.T) {
	server, s := newTestServer(t)
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	ask := func(id string) {
		if _, rejection, err := s.Propose(events.Proposal{Stream: id, Actor: lead, Type: events.TaskNeedsYou, Data: map[string]any{
			"question": "Close it?", "options": []string{"Close the epic"}, "closing": "All shipped."}}); err != nil || rejection != nil {
			t.Fatal(err, rejection)
		}
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Ship it","kind":"epic"}`)
	ask("task-1")
	call(t, server, "POST", "/api/tasks", `{"title":"Ship that","kind":"epic"}`)
	ask("task-2")
	call(t, server, "POST", "/api/tasks/task-2/answer", `{"answer":"Leave it open"}`)

	for _, id := range []string{"task-1", "task-2"} {
		if code, body := call(t, server, "POST", "/api/tasks/"+id+"/direct", `{"to":"done"}`); code != 200 {
			t.Fatalf("%s onto Done: %d %s", id, code, body)
		}
		stream, _ := s.Stream(id)
		if task := tasks.FromEvents(stream); !task.IsDone || task.Closing != "" || task.Question != "" || task.Asked.By != "" {
			t.Errorf("%s: done %v, closing %q, question %q, asked by %q", id, task.IsDone, task.Closing, task.Question, task.Asked.By)
		}
		call(t, server, "POST", "/api/tasks/"+id+"/answer", `{"answer":"Close the epic"}`)
		stream, _ = s.Stream(id)
		if done := countOf(stream, events.TaskDone); done != 1 {
			t.Errorf("%s: closed %d times", id, done)
		}
	}
	if _, body := call(t, server, "GET", "/api/questions", ""); strings.Contains(body, "task-") {
		t.Errorf("a closed epic asks nothing: %s", body)
	}
}

// countOf is how many events of a kind a stream has.
// countOf mirrors internal/web/api_test.go's.
func countOf(stream []events.Event, kind events.Type) int {
	n := 0
	for _, e := range stream {
		if e.Type == kind {
			n++
		}
	}
	return n
}
