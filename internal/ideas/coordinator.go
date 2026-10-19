package ideas

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
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

// coordinatorHook is Ideas' part in the coordinator loop, for the
// coordinators that suggest ideas.
func coordinatorHook() *features.Coordinator {
	return &features.Coordinator{For: Suggests, Brief: brief, Decision: IdeasFormat, Field: "ideas", Check: check, OnDone: onDone}
}

// agentry is the coordinator loop, which keeps the ideas a done suggests.
var agentry = events.Actor{Kind: events.System, ID: tasks.Loop}

// suggestion is one feature a done decision suggests.
type suggestion struct {
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

// check refuses a done whose "ideas" field isn't a list of ideas, such as
// one with "backedBy" a name rather than a list, so the coordinator sends it
// again rather than the session's ideas being lost.
func check(field json.RawMessage) error {
	var suggested []suggestion
	return json.Unmarshal(field, &suggested)
}

// onDone keeps the ideas of a done decision's "ideas" field (suggest).
func onDone(c features.Core, task *tasks.Task, field json.RawMessage) string {
	var suggested []suggestion
	if err := json.Unmarshal(field, &suggested); err != nil {
		return "Agentry couldn't read these ideas: " + err.Error()
	}
	return suggest(c, task, suggested)
}

// suggest keeps the decision's ideas, each an idea from the task about its
// repo, and says which were refused and why. An idea already kept from the
// task under the same title isn't kept again, so a done applied twice, or
// one naming an idea twice, suggests each once. An idea that says it
// repeats one the repo doesn't have is kept as a new idea, as the rules
// would refuse it and the done can't be redone. The ideas it keeps that the
// Visionaries agreed on, Agentry may accept for you (acceptAgreed).
func suggest(c features.Core, task *tasks.Task, suggested []suggestion) string {
	list := List(c.Read, task.Repo)
	kept := map[string]bool{}
	for _, idea := range list {
		kept[idea.Title] = kept[idea.Title] || idea.Task == task.ID
	}
	var notes, refused, added []string
	for _, idea := range suggested {
		if kept[idea.Title] {
			continue
		}
		idea.Repeats = strings.TrimSpace(idea.Repeats)
		unknown := unknownRepeat(idea.Repeats, list)
		if unknown != "" {
			idea.Repeats = ""
		}
		created, rejection, err := c.Log.Create(Prefix, events.Proposal{Actor: agentry, Type: IdeaSuggested, Data: map[string]any{
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
			if unknown != "" {
				notes = append(notes, fmt.Sprintf("%q repeats %s, which doesn't exist; kept as a new idea.", idea.Title, unknown))
			}
		}
	}
	acceptAgreed(c, task.Repo, added)
	if len(refused) > 0 {
		notes = append(notes, "Agentry couldn't keep these ideas:\n- "+strings.Join(refused, "\n- "))
	}
	return strings.Join(notes, "\n")
}

// listOf is items, or an empty list for none, so the event says [] rather
// than null.
func listOf(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}

// unknownRepeat is the id an idea says it repeats when no idea in list has
// it, such as a typo or a title, "" when it names one or none.
func unknownRepeat(repeats string, list []*Idea) string {
	if repeats == "" || slices.ContainsFunc(list, func(idea *Idea) bool { return idea.ID == repeats }) {
		return ""
	}
	return repeats
}

// autoAccepted is the note on an idea Agentry accepted for you under rule,
// with how many viewpoints of the roster backed it.
func autoAccepted(idea *Idea, roster agents.Roster, rule string) string {
	if rule != settings.NobodyDoubted {
		return "auto-accepted: all Visionaries agreed"
	}
	return fmt.Sprintf("auto-accepted: nobody doubted it, %d backed it", len(Backers(idea, roster, VisionariesTeam)))
}

// acceptAgreed accepts for you, when you turned that on, each of the ideas
// just suggested in repo that the Visionaries agreed on by your rule, as
// your Accept does, and starts its task when you turned that on too. An
// idea that repeats another (Duplicate) or looks like an open task
// (LeftForYou) stays open for you, as does one Agentry couldn't accept:
// the done goes on. The rule trusts backedBy and doubtedBy as the
// Visionaries coordinator wrote them in its done; nothing checks them
// against what each viewpoint said.
func acceptAgreed(c features.Core, repo string, added []string) {
	now := c.Read.Settings()
	if !now.AutoAcceptIdeas || len(added) == 0 {
		return
	}
	list := List(c.Read, repo)
	roster := c.Read.Roster()
	log := logOf(c)
	for _, id := range added {
		i := slices.IndexFunc(list, func(idea *Idea) bool { return idea.ID == id })
		if i < 0 {
			continue
		}
		if why := LeftForYou(c.Read, list[i], list, roster, now.AutoAcceptRule); why != "" {
			slog.Info("coordinator loop: an idea is left for you", "idea", id, "why", why)
			continue
		}
		got, err := Accept(log, c.Read, id, Choice{Note: autoAccepted(list[i], roster, now.AutoAcceptRule), Defaults: c.Defaults, By: agentry})
		if err != nil || !got.Accepted {
			slog.Warn("coordinator loop: accepting an agreed idea", "idea", id, "task", got.Task, "refused", got.Refused, "err", err)
			continue
		}
		if now.AutoStartIdeas {
			startIdea(c.Log, got.Task)
		}
	}
}

// startIdea starts a task made from an idea Agentry accepted, out of the
// Backlog, as your Start does: moved to In progress, for its coordinator.
func startIdea(log features.Proposer, id string) {
	kind := events.TaskHeld
	_, rejection, err := log.Propose(events.Proposal{Stream: id, Actor: agentry, Type: kind,
		Data: map[string]any{"place": "", "note": "", "column": "in-progress", "sentBack": false}})
	if err != nil || rejection != nil {
		slog.Warn("coordinator loop: proposal refused", "task", id, "type", kind, "rejection", rejection, "err", err)
	}
}

// brief names the ideas already accepted or dismissed in the task's repo,
// so the coordinator doesn't suggest them again.
func brief(state features.Reader, task *tasks.Task) []features.Section {
	chosen := List(state, task.Repo)
	var sections []features.Section
	if accepted := InState(chosen, Accepted); len(accepted) > 0 {
		sections = append(sections, features.Section{Heading: "Ideas already accepted", Text: acceptedText(accepted)})
	}
	if dismissed := InState(chosen, Dismissed); len(dismissed) > 0 {
		sections = append(sections, features.Section{Heading: "Ideas the user dismissed", Text: dismissedText(dismissed)})
	}
	return sections
}

// acceptedText lists the accepted ideas, with the task each became.
func acceptedText(accepted []*Idea) string {
	var b strings.Builder
	for _, idea := range accepted {
		fmt.Fprintf(&b, "- %s (%s): became %s\n", idea.Title, idea.ID, idea.Became)
	}
	return b.String() + "\nThese ideas for this repo were accepted already. Don't suggest them again, under any title. " +
		"If you really mean to raise one again, say how it differs and set \"repeats\" to its id."
}

// dismissedText lists the dismissed ideas, with what you said of each.
func dismissedText(dismissed []*Idea) string {
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
