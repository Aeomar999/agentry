package hooks

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// You approve a held push while the coordinator is deciding; not having
// heard, it gives the engineer other work first. Once it hears, it hands
// the push back, as the trail tells it: the pass should still work then.
func TestAPassSurvivesATurnTheCoordinatorAssignedBeforeHearing(t *testing.T) {
	s, state := engineerAtWork(t)
	h := NewHandler(s, state, judgeByCommand)
	timedOut(t, h, "toolu_4", "git push origin HEAD:dev")
	coordinator := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-2", Data: map[string]any{"agent": "engineer", "report": "held"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-3", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	approveLate(t, s, "toolu_4") // while tech-lead decides
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: worker, Type: events.TurnEnded, Correlation: "turn-3", Data: map[string]any{"agent": "tech-lead", "report": "r"}},
		{Stream: "task-1", Actor: coordinator, Type: events.TaskAssigned, Data: map[string]any{"agent": "engineer", "instructions": "write the docs"}},
		{Stream: "task-1", Actor: worker, Type: events.TurnStarted, Correlation: "turn-4", Data: map[string]any{"agent": "engineer", "hash": "h"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	handBackTo(t, s, "engineer", "turn-5", "turn-6") // tech-lead has heard now, and hands the push back
	if reply := timedOut(t, h, "toolu_5", "git push origin HEAD:dev"); decisionOf(reply) != "allow" {
		t.Errorf("the handed-back push is asked about again: %+v; pass %+v", reply, taskIn(t, s).Action("toolu_4"))
	}
}
