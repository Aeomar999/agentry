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
	ToYou        string   `json:"toYou,omitempty"`   // any decision: its answer to what the user told it
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
	`a self-contained HTML page (inline CSS and SVG, no links or scripts from elsewhere) that shows what you need ` +
	`them to see, such as a diagram of two designs. The page can call answer("text") to send that as the user's answer. ` +
	`Use a page only when words alone would be hard to follow.` + "\n\n" +
	PageDesign + "\n\n" +
	`Any decision may also carry "title": a better title for the task, when its title doesn't say what the task is about ` +
	`(people often write a vague one, or none, and then the start of the description stands in). ` +
	`Make it one plain imperative line under 70 characters, such as "Add a retry to the webhook sender". ` +
	`Leave "title" out when the title is already clear.` + "\n\n" +
	`Any decision may also carry "toYou": your reply to the user, when they have told you something since your last ` +
	`turn (the steps from "you" in what has happened). In one or two plain lines, say what you changed because of it, ` +
	`or why you didn't. The user reads it in their chat with you. Leave it out when they haven't said anything new.` + "\n"

// PageDesign is how a question's page should look. It is shown in the full
// width of the content area, next to the rest of Agentry, so it should read
// as part of it (QuestionPage gives it these variables and plain styles).
const PageDesign = `Design the page properly; it is how the user sees the choice. It is shown across the full width of ` +
	`the content area, 900 to 1400 pixels wide, and can be expanded to the whole window, so use that width: put options ` +
	`side by side in a CSS grid (grid-template-columns: repeat(auto-fit, minmax(280px, 1fr))) rather than one long ` +
	`column, and let diagrams fill their column (width: 100%). Start with a heading that says what is being decided, ` +
	`then one short paragraph. Give each option a bordered card (1px solid var(--border), padding 16px) with its name, ` +
	`what it means, its trade-offs, and a button that calls answer() with that choice. Use the page's colour ` +
	`variables, never fixed colours, so it works in light and dark: var(--bg), var(--panel), var(--raised), ` +
	`var(--border), var(--text), var(--text-2), var(--muted), var(--accent), var(--good), var(--warn), var(--bad), and ` +
	`fonts var(--sans) and var(--mono). Keep it flat: square corners (at most 4px), no shadows or gradients. Text at ` +
	`least 13px, labels in diagrams at least 12px. Plain HTML elements (h1, h2, p, table, button, pre) are already styled.`

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
