// Package coordinator runs the loop: every task goes to its coordinator, the
// coordinator's decision starts the next agent, and every agent's report (or
// any issue) goes back to the coordinator, until it says the task is done.
package coordinator

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Decision is what the coordinator's reply ends with.
type Decision struct {
	Action       Action   `json:"action"`
	Agent        string   `json:"agent,omitempty"`
	Machine      string   `json:"machine,omitempty"` // for assign: where the agent works; "" lets Agentry choose
	Instructions string   `json:"instructions,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	Question     string   `json:"question,omitempty"`
	Title        string   `json:"title,omitempty"`   // any decision may rename the task
	Options      []string `json:"options,omitempty"` // for ask: answers the user can pick
	Page         string   `json:"page,omitempty"`    // for ask: a small HTML page that asks it
	ToYou        string   `json:"toYou,omitempty"`   // any decision: its answer to what the user told it
	Team         string   `json:"team,omitempty"`    // for transfer and request: the other team
	Reason       string   `json:"reason,omitempty"`  // for transfer: why the task is theirs
	Request      string   `json:"request,omitempty"` // for request: what this task needs from them
	Stories      []Story  `json:"stories,omitempty"` // for split: the stories to add
	Path         string   `json:"path,omitempty"`    // for fetch: the file or folder on Machine
	As           string   `json:"as,omitempty"`      // for fetch: the name to share it by; "" for its own
	Repo         string   `json:"repo,omitempty"`    // for move: the repo the user named: a folder, a git address or an archive link
}

// Story is one story a split adds to the epic.
type Story struct {
	Title string `json:"title"`
	Brief string `json:"brief"`          // what the story's coordinator needs to know: the goal, what done looks like
	Team  string `json:"team,omitempty"` // "" for the epic's own team
	// After are the stories it builds on: numbers count the stories in this
	// split from 1, and ids ("task-14") name the epic's stories already there.
	After []StoryRef `json:"after,omitempty"`
}

// StoryRef is a story another builds on: Number in this split, or ID.
type StoryRef struct {
	Number int
	ID     string
}

func (r *StoryRef) UnmarshalJSON(data []byte) error {
	if json.Unmarshal(data, &r.Number) == nil {
		return nil
	}
	if err := json.Unmarshal(data, &r.ID); err != nil {
		return err
	}
	if n, err := strconv.Atoi(r.ID); err == nil { // "2" is the second story too
		r.Number, r.ID = n, ""
	}
	return nil
}

// MaxStories is how many stories one split may add.
const MaxStories = 12

type Action string

const (
	Assign Action = "assign" // hand the task to Agent with Instructions
	Finish Action = "done"   // the task is done; Summary says what was done
	Ask    Action = "ask"    // the user must answer Question first

	Transfer     Action = "transfer" // the task is another team's work: their coordinator takes it, with Reason
	Request      Action = "request"  // this task needs something from another team: a task of theirs, which this one waits for
	Split        Action = "split"    // the task is too big for one story: it becomes an epic, with Stories
	AwaitStories Action = "wait"     // for an epic: nothing to do until more of its stories are done
	FetchFile    Action = "fetch"    // bring a file or folder from a machine into the task's shared files
	MoveRepo     Action = "move"     // the task's work goes in the repo the user named for it, with Repo
)

// DecisionFormat closes every coordinator brief. It is code, not part of the
// coordinator's prompt, so editing the prompt can't break the parser.
const DecisionFormat = "End your reply with exactly one decision: a JSON object, on its own, as the very last thing you write. One of:\n\n" +
	`{"action": "assign", "agent": "<agent name>", "instructions": "<what to do, what done looks like, what it needs to know>", "machine": "<optional: a machine from \"Machines\">"}` + "\n" +
	`{"action": "done", "summary": "<what was done, for the user>"}` + "\n" +
	`{"action": "ask", "question": "<what you need the user to decide>"}` + "\n" +
	`{"action": "transfer", "team": "<team>", "reason": "<why this task is that team's work>"}` + "\n" +
	`{"action": "request", "team": "<team>", "request": "<what this task needs from them, and what done looks like>"}` + "\n" +
	`{"action": "wait"} (for an epic, while its stories run and nothing needs you)` + "\n" +
	`{"action": "fetch", "machine": "<a machine>", "path": "<the file or folder there>", "as": "<optional: the name to share it by>"}` + "\n" +
	`{"action": "move", "repo": "<the repo the user named for this task: a folder on this machine, a git address or a link to an archive, exactly as they wrote it>"}` + "\n" +
	`{"action": "split", "stories": [{"title": "<the story>", "brief": "<its goal and what done looks like>", "team": "<optional: another team>", "after": [<optional: the stories it builds on: 1 for the first in this list, or an existing story's id>]}]}` + "\n\n" +
	`Use "transfer" and "request" only with a team named in "Other teams" in this brief.` + "\n\n" +
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

// WrapUpFormat closes the brief of a coordinator's last turn after the
// user moved its task: its summary is the only decision it has.
const WrapUpFormat = "End your reply with exactly one decision: a JSON object, on its own, as the very last thing you write:\n\n" +
	`{"action": "done", "summary": "<what was done, what was left, and where the work is, for the user>"}` + "\n\n" +
	`It may also carry "toYou": your reply to the user, when they have told you something since your last turn.` + "\n"

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
	case Transfer:
		if d.Team == "" || d.Reason == "" {
			return errors.New(`a "transfer" decision needs "team" and "reason"`)
		}
	case Request:
		if d.Team == "" || d.Request == "" {
			return errors.New(`a "request" decision needs "team" and "request"`)
		}
	case Split:
		return checkStories(d.Stories)
	case AwaitStories:
	case FetchFile:
		if d.Machine == "" || d.Path == "" {
			return errors.New(`a "fetch" decision needs "machine" and "path"`)
		}
	case MoveRepo:
		if strings.TrimSpace(d.Repo) == "" {
			return errors.New(`a "move" decision needs "repo"`)
		}
	default:
		return fmt.Errorf(`unknown action %q: use "assign", "done", "ask", "transfer", "request", "split", "wait", "fetch" or "move"`, d.Action)
	}
	return nil
}

func checkStories(stories []Story) error {
	if len(stories) == 0 || len(stories) > MaxStories {
		return fmt.Errorf(`a "split" decision needs "stories": 1 to %d of them`, MaxStories)
	}
	for i, story := range stories {
		if strings.TrimSpace(story.Title) == "" || strings.TrimSpace(story.Brief) == "" {
			return fmt.Errorf(`story %d needs a "title" and a "brief"`, i+1)
		}
		for _, after := range story.After {
			if after.ID == "" && (after.Number < 1 || after.Number > i) {
				return fmt.Errorf(`story %d builds on story %d: "after" names a story before it in the list, from 1, or an existing story's id`, i+1, after.Number)
			}
		}
	}
	return nil
}
