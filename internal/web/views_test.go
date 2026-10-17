package web

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

func TestEveryStepLeadsToItsTurn(t *testing.T) {
	at := func(minute int) time.Time { return time.Date(2026, 10, 17, 9, minute, 0, 0, time.UTC) }
	task := &tasks.Task{Coordinator: "coordinator", Turns: []*tasks.Turn{
		{ID: "turn-1", Agent: "coordinator", StartedAt: at(1)},
		{ID: "turn-2", Agent: "dev", StartedAt: at(3)},
		{ID: "turn-3", Agent: "coordinator", StartedAt: at(6)},
	}}
	for _, c := range []struct {
		name string
		step tasks.Step
		want string
	}{
		{"you ask: the coordinator's next turn", tasks.Step{At: at(0), From: tasks.You, To: "coordinator"}, "turn-1"},
		{"a handoff: the agent's turn it started", tasks.Step{At: at(2), From: "coordinator", To: "dev"}, "turn-2"},
		{"a report: the coordinator's turn reading it", tasks.Step{At: at(5), From: "dev", To: "coordinator"}, "turn-3"},
		{"an issue: the coordinator's turn reading it", tasks.Step{At: at(4), From: tasks.Agentry, To: "coordinator"}, "turn-3"},
		{"a question to you: the turn that asked it", tasks.Step{At: at(8), From: "coordinator", To: tasks.You}, "turn-3"},
		{"you answer: no turn yet", tasks.Step{At: at(9), From: tasks.You, To: "coordinator"}, ""},
	} {
		if got := turnOf(task, c.step); got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}
