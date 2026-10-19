package coordinator

import (
	"cmp"
	"fmt"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// An epic is a goal too big for one story. Its coordinator splits it into
// stories, each a task of its own with its own branch and checks. Stories
// wait in your Backlog until you start them, in your order; a story
// that builds on others starts once they are done, from a dev that has
// their work. Each finished story's summary comes back to the epic, whose
// coordinator adds stories or, once they are all done, finishes it.

// family is what a coordinator's brief needs from the tasks around one: a
// story's epic, and the epic's stories.
// CloseEpic is the answer that approves closing an epic.
const CloseEpic = "Close the epic"

type family struct {
	epic    *tasks.Task
	stories []*tasks.Task
}

func (l *Loop) familyOf(task *tasks.Task) family {
	var f family
	epic := task
	if task.Epic != "" {
		f.epic, _, _ = l.load(task.Epic)
		epic = f.epic
	}
	if epic == nil {
		return f
	}
	for _, id := range epic.Stories {
		if story, _, err := l.load(id); err == nil && story != nil {
			f.stories = append(f.stories, story)
		}
	}
	// In the user's priority order: what they want first is first.
	slices.SortStableFunc(f.stories, func(a, b *tasks.Task) int { return cmp.Compare(a.Rank, b.Rank) })
	return f
}

// epicSection tells the coordinator what its task is: an epic and how its
// stories stand, a story and its place in its epic, or a story it may split.
func epicSection(b *strings.Builder, task *tasks.Task, f family) {
	switch {
	case task.Kind == tasks.Epic:
		section(b, "This task is an epic", epicText(task, f))
	case task.Epic != "" && f.epic != nil:
		section(b, "This task is a story of an epic", storyText(task, f))
	case !task.KindChosen:
		section(b, "If this task is too big", "Some tasks are too big for one story: one branch, one round of checks, one "+
			"summary. If this one is (several features, work for more than one team, or more than a reviewer can read "+
			"in one sitting), split it into stories with a \"split\" decision instead of starting the work, and it becomes "+
			"an epic. "+splitGuide)
	}
}

const splitGuide = "Each story is a task of its own: its team's coordinator runs it, its work is checked, and its summary " +
	"comes back to you. New stories go to the user's Backlog, and each starts when they move it on. Make each story one reviewable change that leaves the work whole. Give each a brief its " +
	"coordinator can start from with nothing else: the goal, what done looks like, and what it must not break. Stories " +
	"run side by side unless one builds on another: list those in \"after\", and it starts once they are done, from dev " +
	"with their work in it. Give a story to another team with \"team\"."

func epicText(task *tasks.Task, f family) string {
	var text strings.Builder
	if len(f.stories) == 0 {
		text.WriteString("Its work is done in stories, and it has none yet. Before anything else, split it: plan the " +
			"stories with a \"split\" decision. " + splitGuide)
		return text.String()
	}
	text.WriteString("Its work is done in its stories, not by agents here, so you don't assign it. Its stories, in the " +
		"user's priority order (they start them in that order, and new ones go last until they move them):\n\n")
	for _, story := range f.stories {
		fmt.Fprintf(&text, "- %s (%s): %s\n", story.ID, storyState(story, f), story.Title)
		if story.IsDone && story.Summary != "" {
			fmt.Fprintf(&text, "  Its summary: %s\n", tasks.Shorten(story.Summary, 1500))
		}
	}
	text.WriteString("\nYou hear from each story as it finishes. When one does, check what it delivered against the " +
		"epic: if more is needed, add stories with another \"split\" (\"after\" may name the stories above by id); if " +
		"something must be decided, ask the user. Stories in the user's Backlog or To do wait for them to start; don't press " +
		"them to, unless the epic can't go on without it. Otherwise, while stories are still going, \"wait\". Once every story is done, finish the epic with a summary of what each one delivered: the user approves closing " +
		"it, or tells you what else it needs. Agentry refuses \"done\" while any story isn't. If the user says to leave it " +
		"open, \"wait\" and don't ask again: it stays open, with every story done, until they close it or add a story.")
	return text.String()
}

func storyText(task *tasks.Task, f family) string {
	var text strings.Builder
	fmt.Fprintf(&text, "It is %s's story. The epic, %s: %s\n\n%s\n\n", f.epic.Coordinator, f.epic.ID, f.epic.Title,
		tasks.Shorten(f.epic.Description, 2000))
	var later []string
	for _, story := range f.stories {
		if story.ID != task.ID && slices.Contains(story.After, task.ID) {
			later = append(later, story.ID+" ("+story.Title+")")
		}
	}
	text.WriteString("Do this story's part only. A story can't be split: if it needs more than its part, finish what " +
		"is its own and say in your summary what else the epic needs, and the epic's coordinator adds it. Your summary " +
		"goes back to them.")
	if len(later) > 0 {
		text.WriteString("\n\nLater stories build on this one: " + strings.Join(later, ", ") + ". They start from dev once " +
			"this story is done, so its work must be in dev before you finish (merged, by whoever does that in your team).")
	}
	return text.String()
}

// storyState is how a story stands, in a few words.
func storyState(story *tasks.Task, f family) string {
	switch {
	case story.IsDone:
		return "done"
	case story.Question != "":
		return "waiting for the user"
	case story.Parked != "" && story.OpenTurn() == nil:
		return "in the user's " + map[string]string{"todo": "To do", "backlog": "Backlog"}[story.Parked] + ", until they start it"
	case story.Worktree == nil && len(story.After) > 0:
		var waiting []string
		for _, before := range f.stories {
			if slices.Contains(story.After, before.ID) && !before.IsDone {
				waiting = append(waiting, before.ID)
			}
		}
		if len(waiting) > 0 {
			return "waiting for " + strings.Join(waiting, ", ")
		}
	case story.CurrentAgent() != "":
		return story.Coordinator + "'s, " + story.CurrentAgent() + " working"
	}
	return story.Coordinator + "'s, under way"
}
