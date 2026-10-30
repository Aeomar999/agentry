package tasksapi_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks/tasksapi"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

func TestCreateListAndShowATask(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/tasks", `{"title":"Remove the TODOs","description":"in src/"}`); code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	if code, _ := call(t, server, "POST", "/api/tasks", `{"title":"  "}`); code != http.StatusBadRequest {
		t.Errorf("a task without a title: %d", code)
	}
	other := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Second","repo":"`+other+`"}`)
	if code, _ := call(t, server, "POST", "/api/tasks", `{"title":"Third","repo":"/no/such/repo"}`); code != http.StatusBadRequest {
		t.Errorf("a repo that isn't there: %d", code)
	}

	_, body := call(t, server, "GET", "/api/tasks", "")
	var list []taskSummary
	json.Unmarshal([]byte(body), &list)
	if len(list) != 2 || list[0].ID != "task-2" || list[1].Repo != "/src/app" || list[0].Repo != other || list[1].Status != "working" {
		t.Errorf("list (newest first, default repo): %+v", list)
	}

	_, body = call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if detail.Coordinator != "tech-lead" || detail.Description != "in src/" || len(detail.Trail) != 1 || detail.Trail[0].To != "tech-lead" {
		t.Errorf("detail: %+v", detail)
	}
	if code, _ := call(t, server, "GET", "/api/tasks/task-9", ""); code != http.StatusNotFound {
		t.Errorf("a missing task: %d", code)
	}
}

func TestApproveOnlyWhatWasAsked(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Deploy"}`)
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"}, Type: events.ActionRequested, Correlation: "toolu_1",
			Data: map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Policy, ID: "rules"}, Type: events.ActionAsked, Correlation: "toolu_1",
			Data: map[string]any{"reason": "every push"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	if !strings.Contains(body, `"needsYou":"Waiting for you to approve the push to dev"`) || !strings.Contains(body, `"approvals":[{`) {
		t.Errorf("the task should show the approval it needs:\n%s", body)
	}
	if code, body := call(t, server, "POST", "/api/tasks/task-1/actions/toolu_1/approve", `{"reason":"looks right"}`); code != 200 {
		t.Errorf("approve: %d %s", code, body)
	}
	if code, body := call(t, server, "POST", "/api/tasks/task-1/actions/toolu_1/deny", ""); code != http.StatusConflict || !strings.Contains(body, "already approved") {
		t.Errorf("deciding twice: %d %s", code, body)
	}
}

// A task can be just a description: the title stands in from its first
// line until the coordinator names it.
func TestATaskNeedsOnlyADescription(t *testing.T) {
	server, _ := newTestServer(t)
	long := "When a webhook delivery fails with a 503 the sender gives up at once, retry it once after a second\nMore detail."
	for _, c := range []struct{ description, wantTitle string }{
		{"Fix the login redirect\nIt loops after sign-in.", "Fix the login redirect"},
		{long, "When a webhook delivery fails with a 503 the sender gives up at once..."},
	} {
		body, _ := json.Marshal(map[string]string{"description": c.description})
		if code, reply := call(t, server, "POST", "/api/tasks", string(body)); code != 200 {
			t.Fatalf("create: %d %s", code, reply)
		}
	}
	_, body := call(t, server, "GET", "/api/tasks", "")
	var list []taskSummary
	json.Unmarshal([]byte(body), &list)
	if len(list) != 2 || list[1].Title != "Fix the login redirect" || list[0].Title != "When a webhook delivery fails with a 503 the sender gives up at once..." {
		t.Errorf("titles: %+v", list)
	}
	if code, _ := call(t, server, "POST", "/api/tasks", `{"description":"  "}`); code != http.StatusBadRequest {
		t.Errorf("an empty task: %d", code)
	}
}

// A new task gets the server's budget and limit of turns, its own, or none.
func TestANewTaskHasItsLimits(t *testing.T) {
	server, _, _ := limitsServer(t)
	for _, body := range []string{
		`{"description":"Defaults"}`,
		`{"description":"Its own","budget":200000,"maxTurns":50}`,
		`{"description":"Unlimited","budget":-1,"maxTurns":-1}`,
		`{"description":"Defaults again","budget":0,"maxTurns":0}`,
	} {
		if code, reply := call(t, server, "POST", "/api/tasks", body); code != 200 {
			t.Fatalf("%s: %d %s", body, code, reply)
		}
	}
	want := map[string][2]int{"task-1": {5_000_000, 2}, "task-2": {200_000, 50}, "task-3": {0, 0}, "task-4": {5_000_000, 2}}
	for id, limits := range want {
		if d := detailOfTask(t, server, id); d.Budget != limits[0] || d.MaxTurns != limits[1] {
			t.Errorf("%s: budget %d, max turns %d, want %v", id, d.Budget, d.MaxTurns, limits)
		}
	}
	if code, _ := call(t, server, "POST", "/api/tasks", `{"description":"x","maxTurns":-5}`); code != http.StatusConflict {
		t.Errorf("a limit of -5 turns: %d", code)
	}
	_, body := call(t, server, "GET", "/api/config", "")
	if !strings.Contains(body, `"budget":5000000`) || !strings.Contains(body, `"maxTurns":2`) {
		t.Errorf("config: %s", body)
	}
}

// Raising the budget, or making it unlimited, on a task stopped at its
// budget answers for you, and the task carries on. A coordinator's own
// question still waits for your answer.
func TestRaisingTheBudgetLetsATaskGoOn(t *testing.T) {
	server, work, ask := limitsServer(t)
	for range 3 {
		call(t, server, "POST", "/api/tasks", `{"description":"Small budget","budget":200000,"maxTurns":-1}`)
	}
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		work(id, 1, 210_000)
	}

	ask("task-1", theLoop, atLimit("budget"))
	if code, body := call(t, server, "PUT", "/api/tasks/task-1/budget", `{"budget":1000000}`); code != 200 || body != `{"answered":true,"budget":1000000}`+"\n" {
		t.Fatalf("raise: %d %s", code, body)
	}
	if d := detailOfTask(t, server, "task-1"); d.Budget != 1_000_000 || d.Status != "working" || lastSaid(d) != "I raised the budget to 1M tokens. Carry on." {
		t.Errorf("after raising: budget %d, status %s, said %q", d.Budget, d.Status, lastSaid(d))
	}

	ask("task-2", theLoop, atLimit("budget"))
	call(t, server, "PUT", "/api/tasks/task-2/budget", `{"budget":0}`)
	if d := detailOfTask(t, server, "task-2"); d.Budget != 0 || d.Status != "working" || lastSaid(d) != "I made the budget unlimited. Carry on." {
		t.Errorf("after unlimiting: budget %d, status %s, said %q", d.Budget, d.Status, lastSaid(d))
	}

	ask("task-3", techLead, nil)
	call(t, server, "PUT", "/api/tasks/task-3/budget", `{"budget":-1}`)
	if d := detailOfTask(t, server, "task-3"); d.Budget != 0 || d.Status != "needs you" {
		t.Errorf("the coordinator's question: budget %d, status %s", d.Budget, d.Status)
	}
	if code, _ := call(t, server, "PUT", "/api/tasks/task-3/budget", `{"budget":-2}`); code != http.StatusConflict {
		t.Errorf("a budget of -2: %d", code)
	}
}

// Raising the limit of turns above the turns had, or making it unlimited,
// on a task stopped at it answers for you, and the task carries on.
func TestRaisingTheTurnLimitLetsATaskGoOn(t *testing.T) {
	server, work, ask := limitsServer(t)
	for range 3 {
		call(t, server, "POST", "/api/tasks", `{"description":"Two turns"}`)
	}
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		work(id, 2, 1000)
	}

	ask("task-1", theLoop, atLimit("turns"))
	if _, body := call(t, server, "PUT", "/api/tasks/task-1/turns", `{"maxTurns":2}`); body != `{"answered":false,"maxTurns":2}`+"\n" {
		t.Errorf("not raised above the turns had, the reply: %s", body)
	}
	if d := detailOfTask(t, server, "task-1"); d.Status != "needs you" || d.TurnsSinceAnswer != 2 || !d.AtTurnLimit {
		t.Errorf("not raised above the turns had: status %s, turns %d", d.Status, d.TurnsSinceAnswer)
	}
	if code, body := call(t, server, "PUT", "/api/tasks/task-1/turns", `{"maxTurns":5}`); code != 200 {
		t.Fatalf("raise: %d %s", code, body)
	}
	if d := detailOfTask(t, server, "task-1"); d.MaxTurns != 5 || d.Status != "working" || lastSaid(d) != "I raised the turn limit to 5. Carry on." {
		t.Errorf("after raising: max turns %d, status %s, said %q", d.MaxTurns, d.Status, lastSaid(d))
	}

	ask("task-2", theLoop, atLimit("turns"))
	call(t, server, "PUT", "/api/tasks/task-2/turns", `{"maxTurns":-1}`)
	if d := detailOfTask(t, server, "task-2"); d.MaxTurns != 0 || d.Status != "working" || lastSaid(d) != "I made the turns unlimited. Carry on." {
		t.Errorf("after unlimiting: max turns %d, status %s, said %q", d.MaxTurns, d.Status, lastSaid(d))
	}

	ask("task-3", techLead, nil)
	call(t, server, "PUT", "/api/tasks/task-3/turns", `{"maxTurns":0}`)
	if d := detailOfTask(t, server, "task-3"); d.MaxTurns != 0 || d.Status != "needs you" {
		t.Errorf("the coordinator's question: max turns %d, status %s", d.MaxTurns, d.Status)
	}
	if code, _ := call(t, server, "PUT", "/api/tasks/task-3/turns", `{"maxTurns":-2}`); code != http.StatusConflict {
		t.Errorf("a limit of -2: %d", code)
	}
}

// A limit is kept though its answer to the task's stop is refused, as when
// the stop was answered as you saved: the reply says why, with a 200.
func TestALimitIsKeptThoughItsAnswerIsRefused(t *testing.T) {
	answeredFirst := func(p events.Proposal, state rules.State, catalogue *events.Catalogue) *events.Rejection {
		if p.Type == events.TaskAnswered {
			return &events.Rejection{Check: "order", Reason: "there's no open question"}
		}
		return rules.Check(p, state, catalogue)
	}
	server, work, ask := limitsServerChecked(t, answeredFirst)
	call(t, server, "POST", "/api/tasks", `{"description":"Budget","budget":200000,"maxTurns":-1}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Turns"}`)
	work("task-1", 1, 210_000)
	work("task-2", 2, 1000)
	ask("task-1", theLoop, atLimit("budget"))
	ask("task-2", theLoop, atLimit("turns"))

	if code, body := call(t, server, "PUT", "/api/tasks/task-1/budget", `{"budget":1000000}`); code != 200 ||
		body != `{"answered":false,"budget":1000000,"refused":"there's no open question"}`+"\n" {
		t.Errorf("the budget: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/tasks/task-2/turns", `{"maxTurns":5}`); code != 200 ||
		body != `{"answered":false,"maxTurns":5,"refused":"there's no open question"}`+"\n" {
		t.Errorf("the turns: %d %s", code, body)
	}
	if d := detailOfTask(t, server, "task-1"); d.Budget != 1_000_000 {
		t.Errorf("the budget kept: %d", d.Budget)
	}
	if d := detailOfTask(t, server, "task-2"); d.MaxTurns != 5 {
		t.Errorf("the turns kept: %d", d.MaxTurns)
	}
}

// On a done task nothing is kept, and the reply is a 409.
func TestADoneTasksLimitsStay(t *testing.T) {
	server, _, ask := limitsServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Done","kind":"epic","budget":200000,"maxTurns":5}`)
	ask("task-1", techLead, map[string]any{"options": []string{"Close the epic"}, "closing": "All shipped."})
	if code, body := call(t, server, "POST", "/api/tasks/task-1/answer", `{"answer":"Close the epic"}`); code != 200 {
		t.Fatalf("close: %d %s", code, body)
	}
	for _, change := range [][2]string{{"budget", `{"budget":1000000}`}, {"turns", `{"maxTurns":10}`}} {
		if code, body := call(t, server, "PUT", "/api/tasks/task-1/"+change[0], change[1]); code != http.StatusConflict || !strings.Contains(body, "is done") {
			t.Errorf("the %s: %d %s", change[0], code, body)
		}
	}
	if d := detailOfTask(t, server, "task-1"); d.Budget != 200_000 || d.MaxTurns != 5 {
		t.Errorf("after: budget %d, max turns %d", d.Budget, d.MaxTurns)
	}
}

// Raising a limit answers only the stop for that limit: not the other
// limit's, though the task is at both, and not the loop's other questions,
// such as copying a file, though the task is over that limit too.
func TestALimitAnswersOnlyItsOwnStop(t *testing.T) {
	server, work, ask := limitsServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"At both","budget":200000}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Copy, over budget","budget":200000,"maxTurns":-1}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Copy, past its turns","budget":-1}`)
	work("task-1", 2, 210_000)
	work("task-2", 1, 210_000)
	work("task-3", 2, 1000)

	ask("task-1", theLoop, atLimit("turns"))
	call(t, server, "PUT", "/api/tasks/task-1/budget", `{"budget":1000000}`)
	if d := detailOfTask(t, server, "task-1"); d.Budget != 1_000_000 || d.Status != "needs you" || d.TurnsSinceAnswer != 2 {
		t.Errorf("the budget, at the turn stop: budget %d, status %s, turns %d", d.Budget, d.Status, d.TurnsSinceAnswer)
	}
	call(t, server, "PUT", "/api/tasks/task-1/turns", `{"maxTurns":5}`)
	if d := detailOfTask(t, server, "task-1"); d.Status != "working" || lastSaid(d) != "I raised the turn limit to 5. Carry on." {
		t.Errorf("the turns, at the turn stop: status %s, said %q", d.Status, lastSaid(d))
	}

	for id, change := range map[string][2]string{
		"task-2": {"budget", `{"budget":1000000}`},
		"task-3": {"turns", `{"maxTurns":5}`},
	} {
		ask(id, theLoop, copyQuestion)
		if code, body := call(t, server, "PUT", "/api/tasks/"+id+"/"+change[0], change[1]); code != 200 {
			t.Fatalf("%s %s: %d %s", id, change[0], code, body)
		}
		if d := detailOfTask(t, server, id); d.Status != "needs you" || !strings.HasPrefix(d.NeedsYou, "Copy") {
			t.Errorf("%s, the %s with the copy question open: status %s, needs you %q", id, change[0], d.Status, d.NeedsYou)
		}
	}
}

// A stop asked before stops said their limit is known by its words, so it
// is answered by its own limit's endpoint, though the task is at both.
func TestAnOlderStopIsKnownByItsWords(t *testing.T) {
	server, work, ask := limitsServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"At both","budget":200000}`)
	call(t, server, "POST", "/api/tasks", `{"description":"At both","budget":200000}`)
	work("task-1", 2, 210_000)
	work("task-2", 2, 210_000)
	ask("task-1", theLoop, map[string]any{"question": "This task has had 2 turns without finishing. Answer with any guidance and the coordinator carries on."})
	ask("task-2", theLoop, map[string]any{"question": "This task has used 210k tokens of its 200k budget. Raise the budget to go on, or tell the coordinator to wrap up."})

	call(t, server, "PUT", "/api/tasks/task-1/budget", `{"budget":1000000}`)
	if d := detailOfTask(t, server, "task-1"); d.Status != "needs you" || d.StoppedAt != "turns" {
		t.Errorf("the budget, at the old turn stop: status %s, stopped at %q", d.Status, d.StoppedAt)
	}
	call(t, server, "PUT", "/api/tasks/task-1/turns", `{"maxTurns":-1}`)
	if d := detailOfTask(t, server, "task-1"); d.Status != "working" || lastSaid(d) != "I made the turns unlimited. Carry on." {
		t.Errorf("the turns, at the old turn stop: status %s, said %q", d.Status, lastSaid(d))
	}
	call(t, server, "PUT", "/api/tasks/task-2/turns", `{"maxTurns":5}`)
	if d := detailOfTask(t, server, "task-2"); d.Status != "needs you" || d.StoppedAt != "budget" {
		t.Errorf("the turns, at the old budget stop: status %s, stopped at %q", d.Status, d.StoppedAt)
	}
	call(t, server, "PUT", "/api/tasks/task-2/budget", `{"budget":1000000}`)
	if d := detailOfTask(t, server, "task-2"); d.Status != "working" || lastSaid(d) != "I raised the budget to 1M tokens. Carry on." {
		t.Errorf("the budget, at the old budget stop: status %s, said %q", d.Status, lastSaid(d))
	}
}

// The loop's other stops carry no limit: lowering a limit below what the
// task has used doesn't make them a limit's stop, and raising it again
// doesn't answer them.
func TestALimitNeverAnswersTheLoopsOtherStops(t *testing.T) {
	server, work, ask := limitsServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"No coordinator","maxTurns":10}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Issues","budget":2000000}`)
	work("task-1", 2, 1000)
	work("task-2", 1, 300_000)
	ask("task-1", theLoop, map[string]any{"question": "The task's coordinator, tech-lead, no longer exists."})
	ask("task-2", theLoop, map[string]any{"question": "3 issues in a row, the last: no agent. Fix what's wrong and answer, and the coordinator carries on."})

	kindOf := func(id string) string {
		_, body := call(t, server, "GET", "/api/questions", "")
		var questions []tasksapi.Question
		json.Unmarshal([]byte(body), &questions)
		for _, q := range questions {
			if q.Task == id {
				return q.Kind
			}
		}
		return ""
	}
	for id, change := range map[string][3]string{
		"task-1": {"turns", `{"maxTurns":1}`, `{"maxTurns":10}`},
		"task-2": {"budget", `{"budget":200000}`, `{"budget":2000000}`},
	} {
		call(t, server, "PUT", "/api/tasks/"+id+"/"+change[0], change[1])
		if kind, d := kindOf(id), detailOfTask(t, server, id); kind != "question" || d.StoppedAt != "" {
			t.Errorf("%s, the %s lowered below what's used: kind %q, stopped at %q", id, change[0], kind, d.StoppedAt)
		}
		call(t, server, "PUT", "/api/tasks/"+id+"/"+change[0], change[2])
		if d := detailOfTask(t, server, id); d.Status != "needs you" || lastSaid(d) == "" || strings.HasPrefix(lastSaid(d), "I ") {
			t.Errorf("%s, the %s raised again: status %s, said %q", id, change[0], d.Status, lastSaid(d))
		}
	}
}

// The questions list marks a stop at the budget or the limit of turns, with
// its numbers, and no other question, though the task is over a limit.
func TestQuestionsShowWhichLimitStopped(t *testing.T) {
	server, work, ask := limitsServer(t)
	call(t, server, "POST", "/api/tasks", `{"description":"Turns","budget":200000}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Budget","budget":200000,"maxTurns":-1}`)
	call(t, server, "POST", "/api/tasks", `{"description":"Copy","budget":200000,"maxTurns":-1}`)
	work("task-1", 2, 210_000)
	work("task-2", 1, 210_000)
	work("task-3", 1, 210_000)
	ask("task-1", theLoop, atLimit("turns"))
	ask("task-2", theLoop, atLimit("budget"))
	ask("task-3", theLoop, copyQuestion)

	_, body := call(t, server, "GET", "/api/questions", "")
	var questions []tasksapi.Question
	json.Unmarshal([]byte(body), &questions)
	kinds := map[string]tasksapi.Question{}
	for _, q := range questions {
		kinds[q.Task] = q
	}
	if q := kinds["task-1"]; q.Kind != "turns" || q.TurnsSinceAnswer != 2 || q.MaxTurns != 2 || q.Budget != 0 {
		t.Errorf("the turn stop: %+v", q)
	}
	if q := kinds["task-2"]; q.Kind != "budget" || q.TokensUsed != 210_000 || q.Budget != 200_000 || q.MaxTurns != 0 {
		t.Errorf("the budget stop: %+v", q)
	}
	if q := kinds["task-3"]; q.Kind != "question" || q.Budget != 0 {
		t.Errorf("the copy question: %+v", q)
	}
	for id, want := range map[string]string{"task-1": "turns", "task-2": "budget", "task-3": ""} {
		if d := detailOfTask(t, server, id); d.StoppedAt != want {
			t.Errorf("%s stopped at %q, want %q", id, d.StoppedAt, want)
		}
	}
}

// Tasks list in your priority order; a new one starts at the top.
func TestTasksListInPriorityOrder(t *testing.T) {
	server, _ := newTestServer(t)
	for _, d := range []string{"first", "second", "third"} {
		call(t, server, "POST", "/api/tasks", `{"description":"`+d+`"}`)
	}
	order := func() string {
		_, body := call(t, server, "GET", "/api/tasks", "")
		var list []taskSummary
		json.Unmarshal([]byte(body), &list)
		ids := []string{}
		for _, task := range list {
			ids = append(ids, task.ID)
		}
		return strings.Join(ids, " ")
	}
	if got := order(); got != "task-3 task-2 task-1" {
		t.Errorf("newest first: %s", got)
	}
	_, body := call(t, server, "GET", "/api/tasks", "")
	var list []taskSummary
	json.Unmarshal([]byte(body), &list)
	// task-1 dropped between task-3 and task-2
	call(t, server, "PUT", "/api/tasks/task-1/rank", fmt.Sprintf(`{"rank":%f}`, (list[0].Rank+list[1].Rank)/2))
	if got := order(); got != "task-3 task-1 task-2" {
		t.Errorf("after the drop: %s", got)
	}
}

// A held action says so, and when, on the task and in Questions: its
// agent was told no long ago, and the command may no longer fit.
func TestAHeldActionSaysItWasHeld(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Deploy"}`)
	worker, policy := events.Actor{Kind: events.Worker, ID: "local"}, events.Actor{Kind: events.Policy, ID: "rules"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w", "branch": "b", "port": 7901}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"}, Type: events.ActionRequested, Correlation: "toolu_1",
			Data: map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"}},
		{Stream: "task-1", Actor: policy, Type: events.ActionAsked, Correlation: "toolu_1", Data: map[string]any{"reason": "every push"}},
		{Stream: "task-1", Actor: policy, Type: events.ActionHeld, Correlation: "toolu_1"},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}

	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if len(detail.Approvals) != 1 || !detail.Approvals[0].Held || detail.Approvals[0].HeldAt.IsZero() {
		t.Fatalf("the task should show the held approval:\n%s", body)
	}
	heldAt := detail.Approvals[0].HeldAt

	_, body = call(t, server, "GET", "/api/questions", "")
	var questions []tasksapi.Question
	json.Unmarshal([]byte(body), &questions)
	if len(questions) != 1 || !questions[0].Held || !questions[0].HeldAt.Equal(heldAt) || questions[0].Text != "git push origin HEAD:dev" {
		t.Errorf("Questions should show the held approval with its full command:\n%s", body)
	}
}

func TestATaskCanStartANewRepo(t *testing.T) {
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "t@example.com")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "t@example.com")
	server, _ := newTestServer(t)
	path := filepath.Join(t.TempDir(), "exchange-market-data-design")
	if code, body := call(t, server, "POST", "/api/tasks", `{"title":"Design market data","repo":"`+path+`","newRepo":true}`); code != 200 {
		t.Fatalf("a task in a new repo: %d %s", code, body)
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if detail.Repo != path {
		t.Errorf("the task's repo: %q", detail.Repo)
	}
	if out, err := exec.Command("git", "-C", path, "log", "--format=%s").Output(); err != nil || strings.TrimSpace(string(out)) != "Start: Design market data" {
		t.Errorf("the new repo's history: %q %v", out, err)
	}
	if code, _ := call(t, server, "POST", "/api/tasks", `{"title":"Again","repo":"`+path+`","newRepo":true}`); code != http.StatusBadRequest {
		t.Errorf("a new repo where one already is: %d", code)
	}
}

// A task, or a move, can name its repo by a git address: it is cloned here.
func TestATaskInARepoNamedByItsAddressWorksInAClone(t *testing.T) {
	server, _ := newTestServer(t)
	address := "file://" + filepath.ToSlash(gitRepo(t))
	if code, body := call(t, server, "POST", "/api/tasks", `{"title":"Make it a PWA","repo":"`+address+`"}`); code != 200 {
		t.Fatalf("a task in a repo by its address: %d %s", code, body)
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if !strings.Contains(detail.Repo, string(filepath.Separator)+"repos"+string(filepath.Separator)) {
		t.Fatalf("the task should work in a clone: %q", detail.Repo)
	}
	if out, err := exec.Command("git", "-C", detail.Repo, "remote", "get-url", "origin").Output(); err != nil || strings.TrimSpace(string(out)) != address {
		t.Errorf("the clone's origin: %q %v", out, err)
	}
	if code, _ := call(t, server, "POST", "/api/tasks", `{"title":"Nowhere","repo":"file:///no/such/repo"}`); code != http.StatusBadRequest {
		t.Errorf("an address with no repo: %d", code)
	}
}

// A task names its team in any case, and goes to that team's coordinator.
func TestATaskNamesItsTeamInAnyCase(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/teams", dataTeam); code != 200 {
		t.Fatalf("adding the Data team: %d %s", code, body)
	}
	for i, c := range []struct{ team, coordinator string }{{"data", "data-lead"}, {" software ", "tech-lead"}, {"DATA", "data-lead"}, {"", "tech-lead"}} {
		if code, body := call(t, server, "POST", "/api/tasks", `{"title":"Chart the sign-ups","team":"`+c.team+`"}`); code != 200 {
			t.Fatalf("team %q: %d %s", c.team, code, body)
		}
		_, body := call(t, server, "GET", "/api/tasks/task-"+strconv.Itoa(i+1), "")
		var detail taskDetail
		json.Unmarshal([]byte(body), &detail)
		if detail.Coordinator != c.coordinator {
			t.Errorf("team %q: the task went to %q, want %q", c.team, detail.Coordinator, c.coordinator)
		}
	}
	if code, body := call(t, server, "POST", "/api/tasks", `{"title":"x","team":"Datta"}`); code != http.StatusConflict || !strings.Contains(body, "there is no coordinator in the Datta team") {
		t.Errorf("an unknown team: %d %s", code, body)
	}
}

// You approve closing an epic, on the page; the desk can't for you, and
// any other answer goes to its coordinator.
func TestYouCloseAnEpic(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: token, DeskToken: "desk-secret", DefaultRepo: "/r"}, http.NotFoundHandler(), nil).Handler())
	defer server.Close()
	call(t, server, "POST", "/api/tasks", `{"title":"Ship it","kind":"epic"}`)
	lead := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	s.Propose(events.Proposal{Stream: "task-1", Actor: lead, Type: events.TaskNeedsYou, Data: map[string]any{
		"question": "Close it?", "options": []string{"Close the epic"}, "closing": "All shipped."}})

	request, _ := http.NewRequest("POST", server.URL+"/api/tasks/task-1/answer", strings.NewReader(`{"answer":"Close the epic"}`))
	request.Header.Set("Authorization", "Bearer desk-secret")
	if response, err := http.DefaultClient.Do(request); err != nil || response.StatusCode != http.StatusForbidden {
		t.Errorf("the desk closed an epic: %v %v", response.StatusCode, err)
	}
	if code, body := call(t, server, "POST", "/api/tasks/task-1/answer", `{"answer":"close the epic"}`); code != 200 {
		t.Fatalf("close: %d %s", code, body)
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	if !strings.Contains(body, `"status":"done"`) || !strings.Contains(body, `"summary":"All shipped."`) {
		t.Errorf("the epic should be closed: %s", body)
	}
}

// An epic you leave open, with every story done, isn't waiting for you, and
// you can still close it from the page.
func TestYouCloseAnEpicYouLeftOpen(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: token, DefaultRepo: "/r"}, http.NotFoundHandler(), nil).Handler())
	defer server.Close()
	call(t, server, "POST", "/api/tasks", `{"title":"Ship it","kind":"epic"}`)
	s.Propose(events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Agent, ID: "tech-lead"}, Type: events.TaskNeedsYou,
		Data: map[string]any{"question": "Close it?", "options": []string{"Close the epic"}, "closing": "All shipped."}})
	if code, body := call(t, server, "POST", "/api/tasks/task-1/answer", `{"answer":"Leave it open"}`); code != 200 {
		t.Fatalf("answer: %d %s", code, body)
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	if !strings.Contains(body, `"status":"working"`) || !strings.Contains(body, `"closing":"All shipped."`) {
		t.Errorf("the epic should be open, ready to close: %s", body)
	}
	if _, body := call(t, server, "GET", "/api/questions", ""); strings.Contains(body, "task-1") {
		t.Errorf("an epic left open isn't waiting for you: %s", body)
	}
	if code, body := call(t, server, "POST", "/api/tasks/task-1/answer", `{"answer":"Close the epic"}`); code != 200 {
		t.Fatalf("close: %d %s", code, body)
	}
	_, body = call(t, server, "GET", "/api/tasks/task-1", "")
	if !strings.Contains(body, `"status":"done"`) || !strings.Contains(body, `"summary":"All shipped."`) || !strings.Contains(body, `"closing":""`) {
		t.Errorf("the epic should be closed: %s", body)
	}
}
