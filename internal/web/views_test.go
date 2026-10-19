package web

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

func TestEveryStepLeadsToItsTurn(t *testing.T) {
	at := func(minute int) time.Time { return time.Date(2026, 10, 17, 9, minute, 0, 0, time.UTC) }
	task := &tasks.Task{Coordinator: "tech-lead", Turns: []*tasks.Turn{
		{ID: "turn-1", Agent: "tech-lead", StartedAt: at(1)},
		{ID: "turn-2", Agent: "dev", StartedAt: at(3)},
		{ID: "turn-3", Agent: "tech-lead", StartedAt: at(6)},
	}}
	for _, c := range []struct {
		name string
		step tasks.Step
		want string
	}{
		{"you ask: the coordinator's next turn", tasks.Step{At: at(0), From: tasks.You, To: "tech-lead"}, "turn-1"},
		{"a handoff: the agent's turn it started", tasks.Step{At: at(2), From: "tech-lead", To: "dev"}, "turn-2"},
		{"a report: the coordinator's turn reading it", tasks.Step{At: at(5), From: "dev", To: "tech-lead"}, "turn-3"},
		{"an issue: the coordinator's turn reading it", tasks.Step{At: at(4), From: tasks.Agentry, To: "tech-lead"}, "turn-3"},
		{"a question to you: the turn that asked it", tasks.Step{At: at(8), From: "tech-lead", To: tasks.You}, "turn-3"},
		{"you answer: no turn yet", tasks.Step{At: at(9), From: tasks.You, To: "tech-lead"}, ""},
	} {
		if got := turnOf(task, c.step); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

// The task page says why a turn's tokens are unknown, and leaves it out
// where they were counted.
func TestATurnSaysWhyItsTokensAreUnknown(t *testing.T) {
	task := &tasks.Task{Turns: []*tasks.Turn{
		{ID: "turn-1", Ended: true, TokensUnknown: "transcript not saved"},
		{ID: "turn-2", Ended: true, Tokens: 1234},
	}}
	body, _ := json.Marshal(detailOf(task, agents.Roster{}).Turns)
	want := `"tokens":0,"tokensUnknown":"transcript not saved"`
	if !strings.Contains(string(body), want) || strings.Count(string(body), "tokensUnknown") != 1 {
		t.Errorf("turns %s, want turn-1 with %s and turn-2 without a reason", body, want)
	}
}

// The task page says on how many turns the tokens couldn't be counted, and
// why, grouped by reason.
func TestTheTaskSaysWhereItsTokensAreUnknown(t *testing.T) {
	withReasons := func(reasons ...string) *tasks.Task {
		task := &tasks.Task{}
		for _, reason := range reasons {
			task.Turns = append(task.Turns, &tasks.Turn{Ended: true, TokensUnknown: reason})
		}
		return task
	}
	for _, c := range []struct {
		task *tasks.Task
		want string
	}{
		{withReasons(), ""},
		{withReasons("", ""), ""},
		{withReasons("", "transcript not saved"), "Tokens unknown on 1 turn: transcript not saved"},
		{withReasons("transcript not saved", "", "transcript not saved", "transcript not saved"), "Tokens unknown on 3 turns: transcript not saved"},
		{withReasons("transcript not saved", "no tokens reported", "transcript not saved"), "Tokens unknown on 3 turns: transcript not saved (2), no tokens reported (1)"},
	} {
		if got := detailOf(c.task, agents.Roster{}).TokensUnknown; got != c.want {
			t.Errorf("%d turns: %q, want %q", len(c.task.Turns), got, c.want)
		}
	}
}
