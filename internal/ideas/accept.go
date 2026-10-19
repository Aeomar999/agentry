package ideas

import (
	"regexp"
	"strings"
	"sync"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

var you = events.Actor{Kind: events.Human, ID: "you"}

// Choice is what you said when you accepted an idea.
type Choice struct {
	Kind   string // "task" (or "story") or "epic"; "" takes the one the idea suggests
	Note   string
	Budget int          // the new task's token budget; 0 is no limit
	By     events.Actor // who accepts it: you when unset, or Agentry for an idea every Visionary agreed on
}

// Acceptance says what Accept did. The task can be made and the idea still
// not be marked accepted, as when you dismissed it a moment before: then
// Task names the task and Refused says why.
type Acceptance struct {
	Task     string // the task made from the idea, "" when none was
	Accepted bool   // the idea is marked accepted, pointing at Task
	Refused  string // why it stopped short, "" once Accepted
}

// Board is what Accept reads of every idea, every task and the agents, as
// kept from the log (current.State). Accept writes to the log itself.
type Board interface {
	Source
	TaskIDs() []string
	Task(id string) (*tasks.Task, error)
	Roster() agents.Roster
}

// Accept makes an open idea a task in your Backlog, in the idea's repo, for
// the first team's coordinator, then marks the idea accepted with it. The
// task is made first, and the rules accept an idea only for a task made from
// it, so a crash in between leaves a task and an idea still open, never an
// accepted idea that points at no task. The next Accept finds that task and
// only marks the idea, and Accepts run one at a time, so a double click or
// the UI and MCP at once still make one task. The error is for the log
// failing, not for a refusal.
func Accept(log *store.Store, board Board, id string, choice Choice) (Acceptance, error) {
	accepting.Lock()
	defer accepting.Unlock()
	idea, err := board.Idea(id)
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
		if made, err = makeTask(log, board, idea, kind, choice); err != nil || made.Task == "" {
			return made, err
		}
	}
	beforeMarking()
	_, rejection, err := log.Propose(events.Proposal{Stream: idea.ID, Actor: choice.acceptor(), Type: events.IdeaAccepted,
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
		return you
	}
	return choice.By
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
func madeFrom(log *store.Store, board Board, id string) (Acceptance, error) {
	for _, taskID := range board.TaskIDs() {
		task, err := board.Task(taskID)
		if err == nil {
			if task != nil && task.Idea == id {
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

// ideaOf is the idea a task was made from, read from its task.created in
// the log, "" for none.
func ideaOf(log *store.Store, taskID string) (string, error) {
	stream, err := log.Stream(taskID)
	if err != nil {
		return "", err
	}
	for _, e := range stream {
		if e.Type == events.TaskCreated {
			return e.Text("idea"), nil
		}
	}
	return "", nil
}

// makeTask makes the idea's task, of the kind given, parked in the Backlog.
// Task is "" and Refused says why when it can't.
func makeTask(log *store.Store, board Board, idea *Idea, kind tasks.Kind, choice Choice) (Acceptance, error) {
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
	created, rejection, err := log.Create("task-", events.Proposal{Actor: choice.acceptor(), Type: events.TaskCreated, Data: map[string]any{
		"title": idea.Title, "description": description(idea, choice.Note, epic), "repo": idea.Repo, "coordinator": coordinator.Name,
		"budget": choice.Budget, "kind": string(kind), "kindChosen": true, "parked": "backlog", "idea": idea.ID}})
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
// An idea that follows up an epic carries the epic's limits, as its stories
// would, or says the epic wasn't found.
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

// limitsOf is the block of the epic's description that sets its limits: a
// line naming limits, constraints or rules, and the bullets under it. An
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
