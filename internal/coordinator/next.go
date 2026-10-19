package coordinator

import (
	"fmt"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// The loop's safety stops: without them a coordinator that keeps sending work
// back, or an agent that keeps failing, would loop forever. Each task has its
// own limit of turns, tasks.Task.MaxTurns, set when it is made
// (tasks.NewLimits). DefaultMaxTurns is the server's default unless you start
// it with another, and the limit of a task made before each had its own.
const (
	DefaultMaxTurns = tasks.DefaultMaxTurns
	MaxIssuesRun    = 3 // issues in a row with no turn ending normally
)

// Move is the one thing the loop should do next for a task.
type Move struct {
	Kind     MoveKind
	Agent    agents.Agent // for StartTurn
	Handoff  string       // for StartTurn: what the agent is given
	Machine  string       // for StartTurn: where the coordinator wants it to work, "" for Agentry to choose
	Decision Decision     // for ApplyDecision, and EndWrapUp with a summary
	Reason   string       // for RaiseIssue and AskYou, and EndWrapUp without a summary
	Limit    string       // for AskYou: the limit that stopped the task, tasks.LimitBudget or tasks.LimitTurns, "" for any other stop
}

type MoveKind string

const (
	Wait            MoveKind = "wait" // an agent is working, or you are deciding
	PrepareWorktree MoveKind = "prepare the worktree"
	StartTurn       MoveKind = "start a turn"
	ApplyDecision   MoveKind = "apply the coordinator's decision"
	RaiseIssue      MoveKind = "raise an issue"
	AskYou          MoveKind = "ask you"
	EndWrapUp       MoveKind = "end the wrap-up" // with the coordinator's summary, or why there is none
)

// Next decides what should happen next for a task, from its state alone, so
// the loop can stop and restart at any point and pick up where it was.
func Next(task *tasks.Task, roster agents.Roster) Move {
	switch {
	case task.Question != "", task.OpenTurn() != nil:
		return Move{Kind: Wait}
	case task.WrapUp != nil:
		return wrapUp(task, roster)
	case task.IsDone, task.Parked != "", task.WaitingOn != "":
		return Move{Kind: Wait}
	case task.Worktree == nil:
		return Move{Kind: PrepareWorktree}
	case task.PendingDecision != "":
		return decide(task, roster)
	case task.Handoff == nil:
		return Move{Kind: Wait}
	case task.AtTurnLimit():
		return Move{Kind: AskYou, Limit: tasks.LimitTurns, Reason: fmt.Sprintf(
			"This task has had %d of its %d turns without finishing. Raise the limit, or answer with any guidance, and the coordinator carries on.",
			task.TurnsSinceAnswer, task.MaxTurns)}
	case task.AtBudget() && task.TurnsSinceAnswer > 0:
		// Once you answer, one more turn runs, so "wrap up" can be done.
		return Move{Kind: AskYou, Limit: tasks.LimitBudget, Reason: fmt.Sprintf(
			"This task has used %s tokens of its %s budget. Raise the budget to go on, or tell the coordinator to wrap up.",
			Tokens(task.TokensUsed), Tokens(task.Budget))}
	case task.IssuesInARow >= MaxIssuesRun:
		return Move{Kind: AskYou, Reason: fmt.Sprintf(
			"%d issues in a row, the last: %s. Fix what's wrong and answer, and the coordinator carries on.", task.IssuesInARow, task.Handoff.Text)}
	}

	agent, exists := roster.Get(task.Handoff.To)
	if !exists && task.Handoff.To == task.Coordinator {
		return Move{Kind: AskYou, Reason: "The task's coordinator, " + task.Coordinator + ", no longer exists."}
	}
	if !exists {
		return Move{Kind: RaiseIssue, Reason: "there is no agent called " + task.Handoff.To}
	}
	return Move{Kind: StartTurn, Agent: agent, Handoff: task.Handoff.Text, Machine: task.Handoff.Machine}
}

// wrapUp is the coordinator's one last turn after you moved the task to
// Done, To do or the Backlog, and then its summary. Whatever else it
// decides, or if the turn fails, the wrap-up ends there: your move stands.
func wrapUp(task *tasks.Task, roster agents.Roster) Move {
	coordinator, exists := roster.Get(task.Coordinator)
	switch {
	case task.WrapUp.Turn != "" && task.PendingDecision != "":
		decision, err := ParseDecision(task.PendingDecision)
		switch {
		case err != nil:
			return Move{Kind: EndWrapUp, Reason: "the coordinator's reply had no usable decision: " + err.Error()}
		case decision.Action != Finish:
			return Move{Kind: EndWrapUp, Reason: "the coordinator decided \"" + string(decision.Action) + "\", and " + movedText(task) +
				", so only its summary is kept"}
		}
		return Move{Kind: EndWrapUp, Decision: decision}
	case task.WrapUp.Turn != "":
		return Move{Kind: EndWrapUp, Reason: "the coordinator's wrap-up turn ended without a reply"}
	case !exists:
		return Move{Kind: EndWrapUp, Reason: "the task's coordinator, " + task.Coordinator + ", no longer exists"}
	case task.Handoff == nil || task.Handoff.From == tasks.Agentry:
		return Move{Kind: EndWrapUp, Reason: "the coordinator's wrap-up turn couldn't start"}
	case task.Worktree == nil:
		return Move{Kind: PrepareWorktree}
	}
	return Move{Kind: StartTurn, Agent: coordinator, Handoff: task.Handoff.Text}
}

// movedText says where you moved the task, as the coordinator is told.
func movedText(task *tasks.Task) string {
	if task.WrapUp.To == "done" {
		return "the user moved this task to Done: it stays done"
	}
	return "the user moved this task to " + tasks.PlaceName(task.WrapUp.To) + ": it waits there until they move it on"
}

// MaxRequestDepth is how many requests deep tasks may go: a task asking a
// team, whose task asks another, and so on. It stops two teams asking each
// other round and round.
const MaxRequestDepth = 3

// otherTeam says why a transfer or request can't go to the team it names, or "".
func otherTeam(task *tasks.Task, roster agents.Roster, d Decision) string {
	own, _ := roster.Get(task.Coordinator)
	switch {
	case strings.EqualFold(d.Team, own.TeamName()):
		return "the coordinator named its own team, " + d.Team + "; " + string(d.Action) + " goes to another team"
	case !hasTeam(roster, d.Team):
		return "there is no team called " + d.Team + "; the teams are " + strings.Join(roster.Teams(), ", ")
	case d.Action == Request && task.Depth >= MaxRequestDepth:
		return fmt.Sprintf("this task is already %d requests deep; do the work in this team, or ask the user", task.Depth)
	}
	if _, ok := roster.Coordinator(teamNamed(roster, d.Team)); !ok {
		return "the " + d.Team + " team has no coordinator"
	}
	return ""
}

// epicOrStory says why the decision doesn't fit what the task is, or "".
// An epic's work is done in its stories, and it is done once they all are.
func epicOrStory(task *tasks.Task, roster agents.Roster, d Decision) string {
	if task.Kind == tasks.Epic {
		switch d.Action {
		case Assign:
			return "this task is an epic: its work is done in its stories, so add a story with split, rather than assigning it"
		case Request:
			return "this task is an epic: to have another team do part of it, split it with a story for that team"
		case Transfer:
			if len(task.Stories) > 0 {
				return "this epic has stories already, so it stays with its team"
			}
		case Finish:
			if open := openStories(task); len(open) > 0 {
				return "the epic can't be done while its stories aren't: " + strings.Join(open, ", ") + " still going"
			}
		}
	}
	if d.Action == AwaitStories {
		switch {
		case task.Kind != tasks.Epic || len(task.Stories) == 0:
			return "only an epic with stories can wait; decide what happens next"
		case len(openStories(task)) == 0 && task.Closing == "":
			// Asked once: after that it stays open, quietly, until the user closes it.
			return "every story of this epic is done: finish it with a summary, and the user decides whether to close it"
		}
	}
	if d.Action != Split {
		return ""
	}
	switch {
	case task.Epic != "":
		return "this task is a story of " + task.Epic + ", and a story can't be split: finish this part, and say in your summary what else is needed, for the epic's coordinator to add"
	case task.Kind == tasks.Story && task.KindChosen:
		return "the user made this task a story, so it isn't split; if it is too big for one, ask them"
	}
	for i, story := range d.Stories {
		if story.Team != "" && !hasTeam(roster, story.Team) {
			return fmt.Sprintf("story %d is for the %s team, and there is no such team; the teams are %s", i+1, story.Team, strings.Join(roster.Teams(), ", "))
		}
		for _, after := range story.After {
			if after.ID != "" && !slices.Contains(task.Stories, after.ID) {
				return fmt.Sprintf("story %d builds on %s, which is not one of this epic's stories", i+1, after.ID)
			}
		}
	}
	return ""
}

// openStories are an epic's stories that haven't reported done.
func openStories(task *tasks.Task) []string {
	var open []string
	for _, id := range task.Stories {
		if !slices.Contains(task.StoriesDone, id) {
			open = append(open, id)
		}
	}
	return open
}

func hasTeam(roster agents.Roster, team string) bool { return teamNamed(roster, team) != "" }

// teamNamed is the team's name as the roster has it, matched without case.
func teamNamed(roster agents.Roster, team string) string {
	for _, t := range roster.Teams() {
		if strings.EqualFold(t, strings.TrimSpace(team)) {
			return t
		}
	}
	return ""
}

func decide(task *tasks.Task, roster agents.Roster) Move {
	decision, err := ParseDecision(task.PendingDecision)
	if err != nil {
		return Move{Kind: RaiseIssue, Reason: "the coordinator's reply had no usable decision: " + err.Error()}
	}
	if why := epicOrStory(task, roster, decision); why != "" {
		return Move{Kind: RaiseIssue, Reason: why}
	}
	if decision.Action == Assign {
		if _, exists := roster.Get(decision.Agent); !exists {
			return Move{Kind: RaiseIssue, Reason: "the coordinator assigned the task to " + decision.Agent + ", and there is no agent by that name"}
		}
		if why := task.UndoesYourColumn(decision.Agent, roster); why != "" {
			return Move{Kind: RaiseIssue, Reason: why}
		}
	}
	if decision.Action == Finish {
		if task.Column == "shipping" && tasks.TeamHas(roster, task.Coordinator, agents.Ships) {
			return Move{Kind: RaiseIssue, Reason: "the user moved this task to Shipping; hand it to an agent that ships work before you finish"}
		}
		if why := unchecked(task, roster); why != "" {
			return Move{Kind: RaiseIssue, Reason: why}
		}
	}
	if decision.Action == Transfer || decision.Action == Request {
		if reason := otherTeam(task, roster, decision); reason != "" {
			return Move{Kind: RaiseIssue, Reason: reason}
		}
	}
	if decision.Action == MoveRepo && !namedByYou(task, strings.TrimSpace(decision.Repo)) {
		return Move{Kind: RaiseIssue, Reason: "the coordinator can move the task only to a repo the user named for it, " +
			"as they wrote it, and " + decision.Repo + " isn't one; to work somewhere else, ask the user"}
	}
	return Move{Kind: ApplyDecision, Decision: decision}
}

// namedByYou says whether the user named repo for the task: in its title,
// its description, or something they told its coordinator. A coordinator
// moves a task only where the user asked for it to work.
func namedByYou(task *tasks.Task, repo string) bool {
	said := []string{task.Title, task.Description}
	for _, step := range task.Trail {
		if step.From == tasks.You {
			said = append(said, step.Text)
		}
	}
	for _, text := range said {
		if mentions(text, repo) {
			return true
		}
	}
	return false
}

// mentions says whether text names name whole, not as the start of a longer
// path or address: it stands alone, or in quotes, backticks, <> or (), and
// may end a sentence.
func mentions(text, name string) bool {
	if name == "" {
		return false
	}
	for from := 0; ; {
		i := strings.Index(text[from:], name)
		if i < 0 {
			return false
		}
		start, end := from+i, from+i+len(name)
		if opensName(text[:start]) && closesName(text[end:]) {
			return true
		}
		from = start + 1
	}
}

// opensName says whether a name may start just after before.
func opensName(before string) bool {
	last, _ := utf8.DecodeLastRuneInString(before)
	return before == "" || unicode.IsSpace(last) || strings.ContainsRune("\"'`<(‘“", last)
}

// closesName says whether a name may end just before after.
func closesName(after string) bool {
	after = strings.TrimLeft(after, ".,;:")
	first, _ := utf8.DecodeRuneInString(after)
	return after == "" || unicode.IsSpace(first) || strings.ContainsRune("\"'`>)’”", first)
}

// Tokens writes a count of tokens as people read it: 950, 12.4k, 3.1M.
func Tokens(n int) string {
	switch {
	case n >= 1_000_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e6), ".0") + "M"
	case n >= 1_000:
		return strings.TrimSuffix(fmt.Sprintf("%.1f", float64(n)/1e3), ".0") + "k"
	}
	return fmt.Sprint(n)
}
