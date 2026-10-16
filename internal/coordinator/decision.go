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
	Action       Action   `json:"action"`
	Agent        string   `json:"agent,omitempty"`
	Instructions string   `json:"instructions,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	Question     string   `json:"question,omitempty"`
	Title        string   `json:"title,omitempty"`   // any decision may rename the task
	Options      []string `json:"options,omitempty"` // for ask: answers the user can pick
	Page         string   `json:"page,omitempty"`    // for ask: a small HTML page that asks it
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
	`{"action": "ask", "question": "<what you need the user to decide>"}` + "\n\n" +
	`An "ask" may also carry "options": a short list of answers the user can pick with one click, and "page": ` +
	`a small self-contained HTML page (inline CSS and SVG, no links or scripts from elsewhere) that shows what you need ` +
	`them to see, such as a diagram of two designs. The page can call answer("text") to send that as the user's answer. ` +
	`Use a page only when words alone would be hard to follow.` + "\n\n" +
	`Any decision may also carry "title": a better title for the task, when its title doesn't say what the task is about ` +
	`(people often write a vague one, or none, and then the start of the description stands in). ` +
	`Make it one plain imperative line under 70 characters, such as "Add a retry to the webhook sender". ` +
	`Leave "title" out when the title is already clear.` + "\n"

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
