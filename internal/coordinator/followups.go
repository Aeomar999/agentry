package coordinator

import (
	"fmt"
	"log/slog"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// When you close an epic, Agentry starts one Visionaries session about it:
// what it delivered, and what could follow. Its ideas land in Ideas like
// any session's, each from a task that is "about" the epic. The session
// comes from no task, so it takes the server's limits, as a task you make
// does. You turn it off on the Ideas page; without a team made from the
// Visionaries template, the epic says how to add one.

// followUps starts a session after each epic the batch closed: your
// task.done on it, as you approved closing it, not a move to Done on the
// board. Only closes after the loop started count: the ones before are
// history, which every start reads again, and none of them, closed before
// there were these sessions, ever had one.
func (l *Loop) followUps(batch []events.Event) {
	for _, e := range batch {
		if e.Type == events.TaskDone && e.Actor.Kind == events.Human && !e.Flag("moved") && e.Position > l.startedAt {
			l.followUp(e.Stream, e.Position)
		}
	}
}

// followUp starts the session about the epic's close at position close,
// unless one was started for it already, or says on the epic why it
// started none.
func (l *Loop) followUp(id string, close int64) {
	if !l.state.Settings().VisionariesOnEpicClose {
		return
	}
	epic, roster, err := l.load(id)
	if err != nil || epic == nil || epic.Kind != tasks.Epic || !epic.IsDone || l.followedUp(id, close) {
		return
	}
	teams := ideas.Visionaries(roster, agents.BasesIn(l.state))
	if len(teams) == 0 {
		l.skipFollowUp(epic, close, "Agentry started no Visionaries session about this epic, as no team comes from the Visionaries template or a template of yours based on it. "+
			"To have one after the next epic you close, add one: on the Agents page, Add a team, Visionaries. "+
			"Or turn these sessions off on the Ideas page.")
		return
	}
	lead, _ := roster.Coordinator(teams[0])
	limits := tasks.NewLimits(l.Defaults, nil)
	_, refused := l.newTask(agentry, map[string]any{
		"title": firstLine("Follow-ups after " + epic.Title), "description": followUpBrief(epic, l.familyOf(epic)),
		"repo": epic.Repo, "coordinator": lead.Name, "teamChosen": true, "kindChosen": true,
		"budget": limits.Budget, "maxTurns": limits.MaxTurns, "labels": listOf(epic.Labels), "about": id, "close": close})
	if refused != "" {
		slog.Warn("coordinator loop: starting the Visionaries after an epic", "epic", id, "refused", refused)
		l.skipFollowUp(epic, close, "Agentry couldn't start the Visionaries session about this epic: "+refused)
	}
}

// followedUp says whether a task about the epic's close at position close
// was made already: by an earlier pass over the same close, or before a
// restart.
func (l *Loop) followedUp(id string, close int64) bool {
	for _, task := range l.state.Tasks() {
		if task.About == id && task.AboutClose == close {
			return true
		}
	}
	return false
}

// skipFollowUp tells you on the epic why its close got no session, once.
func (l *Loop) skipFollowUp(epic *tasks.Task, close int64, reason string) {
	if epic.Skipped != close {
		l.propose(epic.ID, agentry, events.FollowUpsSkipped, map[string]any{"reason": reason, "close": close})
	}
}

// followUpBrief is the session's task: the epic, its closing summary and
// each story's, the long ones cut short.
func followUpBrief(epic *tasks.Task, f family) string {
	var b strings.Builder
	fmt.Fprintf(&b, "The user has just closed the epic %s, %q. Look at what it delivered and suggest what should follow: "+
		"the follow-ups it left, the next steps it makes possible, and any gap between what it set out to do and what it did. "+
		"Its work is in dev: check what the summaries say against the code. Give each follow-up as an idea whose \"epic\" is %q.\n\n",
		epic.ID, epic.Title, epic.ID)
	fmt.Fprintf(&b, "The epic, as it was asked for:\n\n%s\n\n", tasks.Shorten(strings.TrimSpace(epic.Description), 2000))
	fmt.Fprintf(&b, "Its closing summary:\n\n%s\n\n", tasks.Shorten(strings.TrimSpace(epic.Summary), 4000))
	if len(f.stories) == 0 {
		b.WriteString("It had no stories.\n\n")
	} else {
		b.WriteString("Its stories:\n\n")
	}
	for _, story := range f.stories {
		fmt.Fprintf(&b, "- %s: %s\n", story.ID, story.Title)
		if summary := strings.TrimSpace(story.Summary); summary != "" {
			fmt.Fprintf(&b, "  Its summary: %s\n", tasks.Shorten(summary, 1500))
		}
	}
	b.WriteString("\nA summary cut short ends in [...]. The whole of it is on that task's page in Agentry, and a story's " +
		"work is in the repo's history, on its branch agentry/<story id>.")
	return b.String()
}
