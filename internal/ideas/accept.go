package ideas

import (
	"regexp"
	"strconv"
	"strings"
	"sync"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

// Choice is what you said when you accepted an idea.
type Choice struct {
	Kind     string // "task" (or "story") or "epic"; "" takes the one the idea suggests
	Note     string
	Defaults tasks.Limits // the server's budget and turns for the new task, unless it follows up an epic, which gives its own
	Budget   *int         // the token budget you chose for the new task, 0 for no limit; nil takes its limits' budget
	Start    bool         // start the task now instead of leaving it in the Backlog; only you may
	By       events.Actor // who accepts it: you when unset, or Agentry for an idea the Visionaries agreed on by your rule
	Anyway   bool         // make the task even though the idea looks like an open one (OpenTaskLike); Agentry never does
}

// Acceptance says what Accept did. The task can be made and the idea still
// not be marked accepted, as when you dismissed it a moment before: then
// Task names the task and Refused says why.
type Acceptance struct {
	Task      string // the task made from the idea, "" when none was
	Column    string // Task's column on the board once Accept is done (tasks.Task.BoardColumn), "" when it can't be read
	Accepted  bool   // the idea is marked accepted, pointing at Task
	Refused   string // why it stopped short, "" once Accepted
	Duplicate string // the open task the idea looks like, when that stopped it and nothing was written
}

// Board is what Accept reads of every idea, every task and the agents, as
// kept from the log (current.State). Accept writes to the log itself.
type Board interface {
	Source
	TaskIDs() []string
	Task(id string) (*tasks.Task, error)
	Roster() agents.Roster
}

// Log is what Accept writes to the log and reads of it: a feature's
// Proposer and its read-only EventLog, as its Core gives them.
// *store.Store is both.
type Log interface {
	features.Proposer
	features.EventLog
}

// Accept makes an open idea a task in your Backlog, or started when you
// chose to start it, in the idea's repo, for the first team's coordinator,
// then marks the idea accepted with it. The task is made first, and the
// rules accept an idea only for a task made from it, so a crash in between
// leaves a task and an idea still open, never an accepted idea that points
// at no task. The next Accept finds that task and only marks the idea, so
// the task stays where it was made, started or not, whatever that Accept
// chose; Column says where it is. An idea that looks like an open task
// (OpenTaskLike) makes none unless the choice says to anyway: Duplicate
// names that task and nothing is written. Accepts run one at a time, so a double
// click or the UI and MCP at once still make one task. The error is for the
// log failing, not for a refusal.
func Accept(log Log, board Board, id string, choice Choice) (Acceptance, error) {
	accepting.Lock()
	defer accepting.Unlock()
	got, err := accept(log, board, id, choice)
	if err == nil && got.Task != "" {
		got.Column = columnOf(board, got.Task)
	}
	return got, err
}

// accept is Accept, under its lock, before the column is read.
func accept(log Log, board Board, id string, choice Choice) (Acceptance, error) {
	idea, err := Get(board, id)
	switch {
	case err != nil:
		return Acceptance{}, err
	case idea == nil:
		return Acceptance{Refused: "there is no idea " + id}, nil
	case idea.State != Open:
		return Acceptance{Refused: "idea " + id + " was " + string(idea.State) + " already"}, nil
	}
	kind, known := kindOf(choice.Kind, idea)
	if !known {
		return Acceptance{Refused: "an idea becomes a task or an epic, not " + choice.Kind}, nil
	}
	made, err := madeFrom(log, board, idea.ID)
	if err != nil || made.Refused != "" {
		return made, err
	}
	if made.Task == "" {
		if like := OpenTaskLike(board, idea); like != nil && !choice.Anyway {
			return Acceptance{Duplicate: like.ID, Refused: idea.ID + " looks like the open task " + like.ID + " " + strconv.Quote(like.Title)}, nil
		}
		if made, err = makeTask(log, board, idea, kind, choice); err != nil || made.Task == "" {
			return made, err
		}
	}
	beforeMarking()
	_, rejection, err := log.Propose(events.Proposal{Stream: idea.ID, Actor: choice.acceptor(), Type: IdeaAccepted,
		Data: map[string]any{"task": made.Task, "note": choice.Note}})
	switch {
	case err != nil:
		return made, err
	case rejection != nil:
		made.Refused = made.Task + " was made, but the idea wasn't marked accepted: " + rejection.Reason
		return made, nil
	}
	made.Accepted = true
	return made, nil
}

func (choice Choice) acceptor() events.Actor {
	if choice.By.ID == "" {
		return httpapi.You
	}
	return choice.By
}

// columnOf is the task's column on the board, "" when it can't be read.
func columnOf(board Board, taskID string) string {
	task, err := board.Task(taskID)
	if err != nil || task == nil {
		return ""
	}
	return task.BoardColumn(board.Roster())
}

// accepting runs one Accept at a time.
var accepting sync.Mutex

// beforeMarking runs between making the task and marking the idea; tests
// use it to act in that gap.
var beforeMarking = func() {}

// madeFrom finds a task an earlier Accept made from the idea but stopped
// before marking it. A task the state can't hand out, such as a broken one, is
// skipped unless its task.created names the idea: then it can't be marked,
// and Refused says so, but no second task is made from the idea either.
func madeFrom(log features.EventLog, board Board, id string) (Acceptance, error) {
	for _, taskID := range board.TaskIDs() {
		task, err := board.Task(taskID)
		if err == nil {
			if task != nil && task.MadeFrom == id {
				return Acceptance{Task: taskID}, nil
			}
			continue
		}
		from, readErr := ideaOf(log, taskID)
		if readErr != nil {
			return Acceptance{}, readErr
		}
		if from == id {
			return Acceptance{Task: taskID, Refused: taskID + " was made from " + id + " but can't be read: " + err.Error()}, nil
		}
	}
	return Acceptance{}, nil
}

// ideaOf is the stream a task was made from, read from its task.created in
// the log, "" for none. An archived task's task.created went with its
// rows, so its marker's summary says.
func ideaOf(log features.EventLog, taskID string) (string, error) {
	stream, err := log.Stream(taskID)
	if err != nil {
		return "", err
	}
	if summary, archived := tasks.Archived(stream); archived {
		return summary.MadeFrom, nil
	}
	for _, e := range stream {
		if e.Type == events.TaskCreated {
			return tasks.MadeFromOf(e.Proposal), nil
		}
	}
	return "", nil
}

// makeTask makes the idea's task, of the kind given, parked in the Backlog;
// one you chose to start isn't parked, so it starts as a task you make does,
// with no second event. The rules refuse Agentry a task that isn't parked:
// it starts one only by moving it, as you turned on. Task is "" and Refused
// says why when it can't.
func makeTask(log features.Proposer, board Board, idea *Idea, kind tasks.Kind, choice Choice) (Acceptance, error) {
	if reason := worktrees.IsRepo(idea.Repo); reason != "" {
		return Acceptance{Refused: "the idea's repo can't take a task: " + reason}, nil
	}
	coordinator, ok := board.Roster().Coordinator("")
	if !ok {
		return Acceptance{Refused: "there is no coordinator in the first team"}, nil
	}
	epic, err := epicOf(board, idea)
	switch {
	case err != nil:
		return Acceptance{Refused: "the epic " + idea.Epic + " can't be read, so its limits can't go with the task: " + err.Error()}, nil
	case idea.Epic != "" && epic == nil && choice.acceptor().Kind == events.System:
		return Acceptance{Refused: "the epic " + idea.Epic + " it follows up wasn't found, so its limits can't go with the task: it is left for you"}, nil
	}
	limits := tasks.NewLimits(choice.Defaults, epic)
	if choice.Budget != nil {
		limits.Budget = *choice.Budget
	}
	data := map[string]any{"title": idea.Title, "description": description(idea, choice.Note, epic), "repo": idea.Repo,
		"coordinator": coordinator.Name, "budget": limits.Budget, "maxTurns": limits.MaxTurns, "kind": string(kind), "kindChosen": true,
		"from": idea.ID, "idea": idea.ID}
	if !choice.Start {
		data["parked"] = "backlog"
	}
	created, rejection, err := log.Create("task-", events.Proposal{Actor: choice.acceptor(), Type: events.TaskCreated, Data: data})
	switch {
	case err != nil:
		return Acceptance{}, err
	case rejection != nil:
		return Acceptance{Refused: "the task wasn't made: " + rejection.Reason}, nil
	}
	return Acceptance{Task: created.Stream}, nil
}

// kindOf is the kind of task you chose, or the idea's suggestion.
func kindOf(chosen string, idea *Idea) (tasks.Kind, bool) {
	switch chosen {
	case "":
		return idea.Kind, true
	case "task", "story":
		return tasks.Story, true
	case "epic":
		return tasks.Epic, true
	}
	return "", false
}

// epicOf is the epic the idea follows up, nil when it names none or the
// one it names isn't there. Agentry leaves such an idea for you; your
// accept makes the task, which says the limits are missing.
func epicOf(board Board, idea *Idea) (*tasks.Task, error) {
	if idea.Epic == "" {
		return nil, nil
	}
	return board.Task(idea.Epic)
}

// description tells the coordinator the idea in full, and links back to it.
// An idea that follows up an epic carries the epic's rules (its limits
// block), as its stories would, or says the epic wasn't found. Its budget
// and turns come from tasks.NewLimits.
func description(idea *Idea, note string, epic *tasks.Task) string {
	parts := []string{"From " + idea.ID + ", the idea \"" + idea.Title + "\", suggested in " + idea.Task + "."}
	for _, part := range [][2]string{{"Problem", idea.Problem}, {"Proposal", idea.Proposal}, {"Why now", idea.WhyNow},
		{"Risks", idea.Risks}, {"Follows up", idea.Epic}, {"What you said", note}} {
		if text := strings.TrimSpace(part[1]); text != "" {
			parts = append(parts, part[0]+": "+text)
		}
	}
	switch limits := limitsOf(epic); {
	case limits != "":
		parts = append(parts, "Limits from "+epic.ID+" (copied from the epic):\n\n"+limits)
	case idea.Epic != "" && epic == nil:
		parts = append(parts, "The epic "+idea.Epic+" this follows up wasn't found, so its limits weren't copied.")
	}
	return strings.Join(parts, "\n\n")
}

// longestLimits is how much of an epic with no block of limits is copied.
const longestLimits = 2000

// limitsOf is the epic's rules: the block of its description written as
// limits, constraints or rules, a line naming them and the bullets under it. An
// epic with no such block gives its whole description, cut short.
func limitsOf(epic *tasks.Task) string {
	if epic == nil {
		return ""
	}
	lines := strings.Split(strings.TrimSpace(epic.Description), "\n")
	for i, line := range lines {
		if isBullet(line) || !namesLimits.MatchString(line) {
			continue
		}
		if bullets := bulletsAfter(lines[i+1:]); len(bullets) > 0 {
			return strings.Join(append([]string{strings.TrimSpace(line)}, bullets...), "\n")
		}
	}
	return tasks.Shorten(strings.TrimSpace(epic.Description), longestLimits)
}

var namesLimits = regexp.MustCompile(`(?i)\b(limits?|constraints?|rules?)\b`)

func isBullet(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "- ") }

// bulletsAfter are the bullets the lines start with, past blank lines
// before the first, each with the indented lines that carry it on.
func bulletsAfter(lines []string) []string {
	var bullets []string
	for _, line := range lines {
		switch {
		case strings.TrimSpace(line) == "" && len(bullets) == 0:
		case isBullet(line):
			bullets = append(bullets, strings.TrimRight(line, " \t"))
		case len(bullets) > 0 && strings.TrimSpace(line) != "" && strings.TrimLeft(line, " \t") != line:
			bullets = append(bullets, strings.TrimRight(line, " \t"))
		default:
			return bullets
		}
	}
	return bullets
}
