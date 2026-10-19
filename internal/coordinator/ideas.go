package coordinator

import (
	"fmt"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// A Visionaries session ends with the features it suggests, on its done
// decision: Agentry keeps each as an idea, about the task's repo, for you to
// accept into your Backlog or dismiss. Its brief names the ideas you
// dismissed there, so they aren't suggested again.

// VisionariesTeam is the team whose coordinators suggest ideas.
const VisionariesTeam = "Visionaries"

func isVisionaries(coordinator agents.Agent) bool {
	return strings.EqualFold(coordinator.TeamName(), VisionariesTeam)
}

// Idea is one feature a done decision suggests.
type Idea struct {
	Title     string   `json:"title"`
	Problem   string   `json:"problem"`
	Proposal  string   `json:"proposal"` // the feature
	WhyNow    string   `json:"whyNow,omitempty"`
	Size      string   `json:"size"` // S, M or L
	Risks     string   `json:"risks,omitempty"`
	BackedBy  []string `json:"backedBy,omitempty"`
	DoubtedBy []string `json:"doubtedBy,omitempty"`
	Epic      string   `json:"epic,omitempty"` // an epic it follows up
	Kind      string   `json:"kind,omitempty"` // "" for a story, "epic" for an epic
}

// IdeasFormat is the done decision's ideas field, for a Visionaries brief.
const IdeasFormat = `Your "done" decision carries the ideas the session agreed on, best first, in "ideas": ` +
	`{"action": "done", "summary": "<what the session found>", "ideas": [{"title": "Search the board", ` +
	`"problem": "Finding a task takes scrolling", "proposal": "A search box above the board", "whyNow": "Boards have 200 tasks", ` +
	`"size": "M", "risks": "Slow on big logs", "backedBy": ["<agent>"], "doubtedBy": ["<agent>"], ` +
	`"epic": "<optional: the epic it follows up>", "kind": "<optional: \"epic\" when it is too big for one story>"}]}. ` +
	`Each needs a title, the problem, the proposal and a size of S, M or L. The user accepts or dismisses each one.`

// suggest keeps the decision's ideas, each an idea from the task about its
// repo, and says which were refused and why. An idea already kept from the
// task under the same title isn't kept again, so a done applied twice, or
// one naming an idea twice, suggests each once.
func (l *Loop) suggest(task *tasks.Task, suggested []Idea) string {
	list, err := ideas.List(l.store, task.Repo)
	if err != nil {
		return "Agentry couldn't keep these ideas, as it couldn't read the ones it has: " + sentence(err)
	}
	kept := map[string]bool{}
	for _, idea := range list {
		kept[idea.Title] = kept[idea.Title] || idea.Task == task.ID
	}
	var refused []string
	for _, idea := range suggested {
		if kept[idea.Title] {
			continue
		}
		_, rejection, err := l.store.Create(events.IdeaPrefix, events.Proposal{Actor: agentry, Type: events.IdeaSuggested, Data: map[string]any{
			"title": idea.Title, "problem": idea.Problem, "proposal": idea.Proposal, "whyNow": idea.WhyNow, "size": idea.Size,
			"risks": idea.Risks, "backedBy": listOf(idea.BackedBy), "doubtedBy": listOf(idea.DoubtedBy), "epic": idea.Epic,
			"kind": idea.Kind, "task": task.ID, "repo": task.Repo}})
		switch {
		case err != nil:
			refused = append(refused, fmt.Sprintf("%q: Agentry couldn't write it: %v", idea.Title, err))
		case rejection != nil:
			refused = append(refused, fmt.Sprintf("%q: %s", idea.Title, rejection.Reason))
		default:
			kept[idea.Title] = true
		}
	}
	if len(refused) == 0 {
		return ""
	}
	return "Agentry couldn't keep these ideas:\n- " + strings.Join(refused, "\n- ")
}

// dismissedFor are the ideas you dismissed in the task's repo, for a
// Visionaries coordinator's brief; nil for any other.
func (l *Loop) dismissedFor(task *tasks.Task, coordinator agents.Agent) []*ideas.Idea {
	if !isVisionaries(coordinator) {
		return nil
	}
	list, err := ideas.List(l.store, task.Repo)
	if err != nil {
		return nil
	}
	return ideas.Dismissals(list)
}

// dismissedText lists the dismissed ideas, with what you said of each.
func dismissedText(dismissed []*ideas.Idea) string {
	var b strings.Builder
	for _, idea := range dismissed {
		if idea.Note == "" {
			fmt.Fprintf(&b, "- %s\n", idea.Title)
		} else {
			fmt.Fprintf(&b, "- %s: %s\n", idea.Title, idea.Note)
		}
	}
	return b.String() + "\nThe user dismissed these ideas for this repo. Don't suggest them again unless something has changed " +
		"that answers the reason."
}
