package ideas

import (
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
	Budget int // the new task's token budget; 0 is no limit
}

// Acceptance says what Accept did. The task can be made and the idea still
// not be marked accepted, as when you dismissed it a moment before: then
// Task names the task and Refused says why.
type Acceptance struct {
	Task     string // the task made from the idea, "" when none was
	Accepted bool   // the idea is marked accepted, pointing at Task
	Refused  string // why it stopped short, "" once Accepted
}

// Accept makes an open idea a task in your Backlog, in the idea's repo, for
// the first team's coordinator, then marks the idea accepted with it. The
// task is made first, and the rules accept an idea only for a task made from
// it, so a crash in between leaves a task and an idea still open, never an
// accepted idea that points at no task. The next Accept finds that task and
// only marks the idea, and Accepts run one at a time, so a double click or
// the UI and MCP at once still make one task. The error is for the log
// failing, not for a refusal.
func Accept(log *store.Store, id string, choice Choice) (Acceptance, error) {
	accepting.Lock()
	defer accepting.Unlock()
	idea, err := Load(log, id)
	switch {
	case err != nil:
		return Acceptance{}, err
	case idea == nil:
		return Acceptance{Refused: "there is no idea " + id}, nil
	case idea.State != Open:
		return Acceptance{Refused: "idea " + id + " was " + string(idea.State) + " already"}, nil
	}
	made, err := madeFrom(log, idea.ID)
	if err != nil {
		return Acceptance{}, err
	}
	if made.Task == "" {
		if made, err = makeTask(log, idea, choice); err != nil || made.Task == "" {
			return made, err
		}
	}
	beforeMarking()
	_, rejection, err := log.Propose(events.Proposal{Stream: idea.ID, Actor: you, Type: events.IdeaAccepted,
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

// accepting runs one Accept at a time.
var accepting sync.Mutex

// beforeMarking runs between making the task and marking the idea; tests
// use it to act in that gap.
var beforeMarking = func() {}

// madeFrom finds a task an earlier Accept made from the idea but stopped
// before marking it.
func madeFrom(log *store.Store, id string) (Acceptance, error) {
	ids, err := log.TaskIDs()
	if err != nil {
		return Acceptance{}, err
	}
	for _, taskID := range ids {
		stream, err := log.Stream(taskID)
		if err != nil {
			return Acceptance{}, err
		}
		if task := tasks.FromEvents(stream); task != nil && task.Idea == id {
			return Acceptance{Task: task.ID}, nil
		}
	}
	return Acceptance{}, nil
}

// makeTask makes the idea's task, parked in the Backlog. Task is "" and
// Refused says why when it can't.
func makeTask(log *store.Store, idea *Idea, choice Choice) (Acceptance, error) {
	kind, known := kindOf(choice.Kind, idea)
	if !known {
		return Acceptance{Refused: "an idea becomes a task or an epic, not " + choice.Kind}, nil
	}
	if reason := worktrees.IsRepo(idea.Repo); reason != "" {
		return Acceptance{Refused: "the idea's repo can't take a task: " + reason}, nil
	}
	agentEvents, err := log.Stream(events.AgentsStream)
	if err != nil {
		return Acceptance{}, err
	}
	coordinator, ok := agents.FromEvents(agentEvents).Coordinator("")
	if !ok {
		return Acceptance{Refused: "there is no coordinator in the first team"}, nil
	}
	created, rejection, err := log.Create("task-", events.Proposal{Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": idea.Title, "description": description(idea, choice.Note), "repo": idea.Repo, "coordinator": coordinator.Name,
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

// description tells the coordinator the idea in full, and links back to it.
func description(idea *Idea, note string) string {
	parts := []string{"From " + idea.ID + ", the idea \"" + idea.Title + "\", suggested in " + idea.Task + "."}
	for _, part := range [][2]string{{"Problem", idea.Problem}, {"Proposal", idea.Proposal}, {"Why now", idea.WhyNow},
		{"Risks", idea.Risks}, {"Follows up", idea.Epic}, {"What you said", note}} {
		if text := strings.TrimSpace(part[1]); text != "" {
			parts = append(parts, part[0]+": "+text)
		}
	}
	return strings.Join(parts, "\n\n")
}
