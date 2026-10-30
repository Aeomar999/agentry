package tasks

import "regexp"

// An agent's plan carries on across its turns until it is finished: a new
// turn starts on the plan the agent's last turn left unfinished, to continue
// or replace. The coordinator's plan is the task's, so it carries on from
// one coordinator turn to the next until the task is done.

// openStep is a checklist line not yet ticked: "- [ ] step".
var openStep = regexp.MustCompile(`(?m)^\s*(?:[-*+]|\d+[.)])\s+\[ \]\s`)

// PlanToContinue is the plan the agent's next turn starts on, and the turn
// it comes from: the agent's latest plan, when a step in it is still open.
// "" for both when there is none, or it was finished.
func (t *Task) PlanToContinue(agent string) (plan, from string) {
	for i := len(t.Turns) - 1; i >= 0; i-- {
		if turn := t.Turns[i]; turn.Agent == agent && turn.Plan != "" {
			if !openStep.MatchString(turn.Plan) {
				return "", ""
			}
			return turn.Plan, turn.ID
		}
	}
	return "", ""
}
