package tasks

import "testing"

func TestAnUnfinishedPlanCarriesOn(t *testing.T) {
	task := &Task{Turns: []*Turn{
		{ID: "turn-1", Agent: "tech-lead", Plan: "- [x] build\n- [ ] review"},
		{ID: "turn-2", Agent: "reviewer", Plan: "- [x] read\n- [ ] run the tests"},
		{ID: "turn-3", Agent: "engineer", Plan: "- [x] fix\n- [x] commit"},
		{ID: "turn-4", Agent: "reviewer"}, // stalled before it wrote a plan
	}}
	for _, c := range []struct{ agent, plan, from string }{
		{"tech-lead", "- [x] build\n- [ ] review", "turn-1"},
		{"reviewer", "- [x] read\n- [ ] run the tests", "turn-2"}, // its last plan, past the turn with none
		{"engineer", "", ""}, // finished: the next turn starts fresh
		{"tester", "", ""},   // never had one
	} {
		if plan, from := task.PlanToContinue(c.agent); plan != c.plan || from != c.from {
			t.Errorf("%s: got %q from %q, want %q from %q", c.agent, plan, from, c.plan, c.from)
		}
	}
}
