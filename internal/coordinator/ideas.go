package coordinator

import (
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/settings"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// A Visionaries session ends with the features it suggests, on its done
// decision: Agentry keeps each as an idea, about the task's repo, for you to
// accept into your Backlog or dismiss. Its brief names the ideas already
// accepted or dismissed there, so they aren't suggested again, and an idea
// that does raise one again names it in "repeats". When you turn it on,
// Agentry accepts for you each idea the Visionaries agreed on, by your
// rule, and can start its task too.

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
	Epic      string   `json:"epic,omitempty"`    // an epic it follows up
	Kind      string   `json:"kind,omitempty"`    // "" for a story, "epic" for an epic
	Repeats   string   `json:"repeats,omitempty"` // an idea already accepted or dismissed that it raises again
}

// IdeasFormat is the done decision's ideas field, for a Visionaries brief.
const IdeasFormat = `Your "done" decision carries the ideas the session agreed on, best first, in "ideas": ` +
	`{"action": "done", "summary": "<what the session found>", "ideas": [{"title": "Search the board", ` +
	`"problem": "Finding a task takes scrolling", "proposal": "A search box above the board", "whyNow": "Boards have 200 tasks", ` +
	`"size": "M", "risks": "Slow on big logs", "backedBy": ["<agent>"], "doubtedBy": ["<agent>"], ` +
	`"epic": "<optional: the epic it follows up>", "kind": "<optional: \"epic\" when it is too big for one story>", ` +
	`"repeats": "<optional: the id of an idea already accepted or dismissed that it raises again>"}]}. ` +
	`Each needs a title, the problem, the proposal and a size of S, M or L. The user accepts or dismisses each one.`

// suggest keeps the decision's ideas, each an idea from the task about its
// repo, and says which were refused and why. An idea already kept from the
// task under the same title isn't kept again, so a done applied twice, or
// one naming an idea twice, suggests each once. The ideas it keeps that the
// Visionaries agreed on, Agentry may accept for you (acceptAgreed).
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
			"kind": idea.Kind, "repeats": idea.Repeats, "task": task.ID, "repo": task.Repo}})
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

// autoAccepted is the note on an idea Agentry accepted for you under rule,
// with how many viewpoints of the roster backed it.
func autoAccepted(idea *ideas.Idea, roster agents.Roster, rule string) string {
	if rule != settings.NobodyDoubted {
		return "auto-accepted: all Visionaries agreed"
	}
	return fmt.Sprintf("auto-accepted: nobody doubted it, %d backed it", len(ideas.Backers(idea, roster, ideas.VisionariesTeam)))
}

// acceptAgreed accepts for you, when you turned that on, each of the ideas
// just suggested in repo that the Visionaries agreed on by your rule, as
// your Accept does, and starts its task when you turned that on too. An
// idea that repeats another (ideas.Duplicate) stays open for you, as does
// one Agentry couldn't accept: the done goes on. The rule trusts backedBy
// and doubtedBy as the Visionaries coordinator wrote them in its done;
// nothing checks them against what each viewpoint said.
func (l *Loop) acceptAgreed(repo string, added []string) {
	now := l.state.Settings()
	if !now.AutoAcceptIdeas || len(added) == 0 {
		return
	}
	list := ideas.List(l.state, repo)
	roster := l.state.Roster()
	for _, id := range added {
		i := slices.IndexFunc(list, func(idea *ideas.Idea) bool { return idea.ID == id })
		if i < 0 {
			continue
		}
		if why := ideas.NotAutoAccepted(list[i], list, roster, now.AutoAcceptRule); why != "" {
			slog.Info("coordinator loop: an idea is left for you", "idea", id, "why", why)
			continue
		}
		got, err := ideas.Accept(l.store, l.state, id, ideas.Choice{Note: autoAccepted(list[i], roster, now.AutoAcceptRule), Defaults: l.Defaults, By: agentry})
		if err != nil || !got.Accepted {
			slog.Warn("coordinator loop: accepting an agreed idea", "idea", id, "task", got.Task, "refused", got.Refused, "err", err)
			continue
		}
		if now.AutoStartIdeas {
			l.startIdea(got.Task)
		}
	}
}

// startIdea starts a task made from an idea Agentry accepted, out of the
// Backlog, as your Start does: moved to In progress, for its coordinator.
func (l *Loop) startIdea(id string) {
	l.propose(id, agentry, events.TaskHeld, map[string]any{"place": "", "note": "", "column": "in-progress", "sentBack": false})
}

// chosenFor are the ideas already accepted or dismissed in the task's
// repo, for a Visionaries coordinator's brief; nil for any other.
func (l *Loop) chosenFor(task *tasks.Task, coordinator agents.Agent) []*ideas.Idea {
	if !ideas.Suggests(coordinator) {
		return nil
	}
	return slices.DeleteFunc(ideas.List(l.state, task.Repo), func(idea *ideas.Idea) bool { return idea.State == ideas.Open })
}

// acceptedText lists the accepted ideas, with the task each became.
func acceptedText(accepted []*ideas.Idea) string {
	var b strings.Builder
	for _, idea := range accepted {
		fmt.Fprintf(&b, "- %s (%s): became %s\n", idea.Title, idea.ID, idea.Became)
	}
	return b.String() + "\nThese ideas for this repo were accepted already. Don't suggest them again, under any title. " +
		"If you really mean to raise one again, say how it differs and set \"repeats\" to its id."
}

// dismissedText lists the dismissed ideas, with what you said of each.
func dismissedText(dismissed []*ideas.Idea) string {
	var b strings.Builder
	for _, idea := range dismissed {
		if idea.Note == "" {
			fmt.Fprintf(&b, "- %s (%s)\n", idea.Title, idea.ID)
		} else {
			fmt.Fprintf(&b, "- %s (%s): %s\n", idea.Title, idea.ID, idea.Note)
		}
	}
	return b.String() + "\nThe user dismissed these ideas for this repo. Don't suggest them again unless something has changed " +
		"that answers the reason; then set \"repeats\" to its id."
}
