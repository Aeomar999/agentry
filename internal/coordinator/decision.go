// Package coordinator runs the loop: every task goes to its coordinator, the
// coordinator's decision starts the next agent, and every agent's report (or
// any issue) goes back to the coordinator, until it says the task is done.
package coordinator

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Decision is what the coordinator's reply ends with.
type Decision struct {
	Action       Action `json:"action"`
	Agent        string `json:"agent,omitempty"`
	Instructions string `json:"instructions,omitempty"`
	Summary      string `json:"summary,omitempty"`
	Question     string `json:"question,omitempty"`
}

type Action string

const (
	Assign Action = "assign" // hand the task to Agent with Instructions
	Finish Action = "done"   // the task is done; Summary says what was done
	Ask    Action = "ask"    // the user must answer Question first
)

// DecisionFormat closes every coordinator brief. It is code, not part of the
// coordinator's prompt, so editing the prompt can't break the parser.
const DecisionFormat = "End your reply with exactly one decision: a JSON object, on its own, as the very last thing you write. One of:\n\n" +
	`{"action": "assign", "agent": "<agent name>", "instructions": "<what to do, what done looks like, what it needs to know>"}` + "\n" +
	`{"action": "done", "summary": "<what was done, for the user>"}` + "\n" +
	`{"action": "ask", "question": "<what you need the user to decide>"}` + "\n"

// ParseDecision finds the decision at the end of a coordinator's reply.
// The error explains what was wrong, so the coordinator can be told.
func ParseDecision(reply string) (Decision, error) {
	text := strings.TrimRight(reply, " \t\r\n`")
	for i := strings.LastIndex(text, "{"); i >= 0; i = strings.LastIndex(text[:i], "{") {
		var decision Decision
		decoder := json.NewDecoder(strings.NewReader(text[i:]))
		if decoder.Decode(&decision) != nil || strings.TrimSpace(text[i+int(decoder.InputOffset()):]) != "" {
			continue
		}
		return decision, decision.check()
	}
	return Decision{}, errors.New("the reply did not end with a decision in JSON")
}

func (d Decision) check() error {
	switch d.Action {
	case Assign:
		if d.Agent == "" || d.Instructions == "" {
			return errors.New(`an "assign" decision needs "agent" and "instructions"`)
		}
	case Finish:
		if d.Summary == "" {
			return errors.New(`a "done" decision needs a "summary"`)
		}
	case Ask:
		if d.Question == "" {
			return errors.New(`an "ask" decision needs a "question"`)
		}
	default:
		return fmt.Errorf(`unknown action %q: use "assign", "done" or "ask"`, d.Action)
	}
	return nil
}
