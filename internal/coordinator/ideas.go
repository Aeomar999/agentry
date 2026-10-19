package coordinator

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// A Visionaries session ends with the features it suggests, on its done
// decision: Agentry keeps each as an idea, about the task's repo, for you to
// accept into your Backlog or dismiss. Its brief names the ideas you
// dismissed there, so they aren't suggested again. When you turn it on,
// Agentry accepts for you each idea every Visionary agreed on, and can
// start its task too.

// isVisionaries says whether the coordinator's team was made from a
// template that brings ideas, whatever the team is called.
func isVisionaries(coordinator agents.Agent) bool {
	template, found := agents.TemplateWithID(coordinator.Template)
	return found && template.Lists(ideas.Feature)
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
// one naming an idea twice, suggests each once. The ideas it keeps every
// Visionary agreed on, Agentry may accept for you (acceptAgreed).
func (l *Loop) suggest(task *tasks.Task, suggested []Idea) string {
	kept := map[string]bool{}
	for _, idea := range ideas.List(l.state, task.Repo) {
		kept[idea.Title] = kept[idea.Title] || idea.Task == task.ID
	}
	var refused, added []string
	for _, idea := range suggested {
		if kept[idea.Title] {
			continue
		}
		created, rejection, err := l.store.Create(events.IdeaPrefix, events.Proposal{Actor: agentry, Type: events.IdeaSuggested, Data: map[string]any{
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
			added = append(added, created.Stream)
		}
	}
	l.acceptAgreed(task.Repo, added)
	if len(refused) == 0 {
		return ""
	}
	return "Agentry couldn't keep these ideas:\n- " + strings.Join(refused, "\n- ")
}

// AutoAccepted is the note on an idea Agentry accepted for you.
const AutoAccepted = "auto-accepted: all Visionaries agreed"

// acceptAgreed accepts for you, when you turned that on, each of the ideas
// just suggested in repo that every Visionary agreed on, as your Accept
// does, and starts its task when you turned that on too. An idea under the
// title of one you dismissed, or of one that is a task already, stays open
// for you, as does one Agentry couldn't accept: the done goes on. Agreed
// trusts backedBy and doubtedBy as the Visionaries coordinator wrote them
// in its done; nothing checks them against what each viewpoint said.
func (l *Loop) acceptAgreed(repo string, added []string) {
	now := l.state.Settings()
	if !now.AutoAcceptIdeas || len(added) == 0 {
		return
	}
	list := ideas.List(l.state, repo)
	roster := l.state.Roster()
	for _, id := range added {
		i := slices.IndexFunc(list, func(idea *ideas.Idea) bool { return idea.ID == id })
		if i < 0 || !ideas.Agreed(list[i], roster, ideas.VisionariesTeam) {
			continue
		}
		if why := ideas.Repeats(list[i], list, l.madeFromIdea); why != "" {
			slog.Info("coordinator loop: an agreed idea is left for you", "idea", id, "why", why)
			continue
		}
		got, err := ideas.Accept(l.store, l.state, id, ideas.Choice{Note: AutoAccepted, Defaults: l.Defaults, By: agentry})
		if err != nil || !got.Accepted {
			slog.Warn("coordinator loop: accepting an agreed idea", "idea", id, "task", got.Task, "refused", got.Refused, "err", err)
			continue
		}
		if now.AutoStartIdeas {
			l.startIdea(got.Task)
		}
	}
}

// madeFromIdea says whether a task was made from the idea.
func (l *Loop) madeFromIdea(id string) bool {
	return slices.ContainsFunc(l.state.Tasks(), func(task *tasks.Task) bool { return task.Idea == id })
}

// startIdea starts a task made from an idea Agentry accepted, out of the
// Backlog, as your Start does: moved to In progress, for its coordinator.
func (l *Loop) startIdea(id string) {
	l.propose(id, agentry, events.TaskHeld, map[string]any{"place": "", "note": "", "column": "in-progress", "sentBack": false})
}

// dismissedFor are the ideas you dismissed in the task's repo, for a
// Visionaries coordinator's brief; nil for any other.
func (l *Loop) dismissedFor(task *tasks.Task, coordinator agents.Agent) []*ideas.Idea {
	if !isVisionaries(coordinator) {
		return nil
	}
	return ideas.Dismissals(ideas.List(l.state, task.Repo))
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
