package coordinator

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/platform/worktrees"
)

// apply turns the coordinator's decision into its event, written as the coordinator.
func (l *Loop) apply(task *tasks.Task, roster agents.Roster, decision Decision) {
	defer l.recoverDecision(task, decision)
	coordinator := events.Actor{Kind: events.Agent, ID: task.Coordinator}
	if title := strings.TrimSpace(decision.Title); title != "" && title != task.Title {
		l.propose(task.ID, coordinator, events.TaskRenamed, map[string]any{"title": title})
	}
	switch decision.Action {
	case Assign:
		agent, _ := roster.Get(decision.Agent)
		machine, refused := l.machineFor(agent, decision.Machine)
		if refused != "" {
			l.propose(task.ID, agentry, events.IssueRaised, map[string]any{"reason": refused})
			return
		}
		l.propose(task.ID, coordinator, events.TaskAssigned, map[string]any{"agent": decision.Agent, "instructions": decision.Instructions,
			"machine": machine, "toYou": decision.ToYou, "checks": agent.Column == agents.Reviews, "ships": agent.Column == agents.Ships})
	case Ask:
		l.propose(task.ID, coordinator, events.TaskNeedsYou, map[string]any{"question": decision.Question,
			"options": decision.Options, "page": decision.Page, "toYou": decision.ToYou})
	case Transfer:
		lead, _ := roster.Coordinator(roster.TeamNamed(decision.Team))
		l.propose(task.ID, coordinator, events.TaskTransferred, map[string]any{"coordinator": lead.Name,
			"team": lead.TeamName(), "reason": decision.Reason})
	case Request:
		l.request(task, roster, coordinator, decision)
	case Split:
		l.split(task, roster, coordinator, decision)
	case FetchFile:
		machine, ok := l.machineNamed(decision.Machine)
		if !ok {
			l.propose(task.ID, agentry, events.IssueRaised, map[string]any{"reason": "there is no machine called " + decision.Machine})
			return
		}
		l.propose(task.ID, coordinator, events.FileFetching, map[string]any{"machine": machine, "path": decision.Path, "as": decision.As})
	case MoveRepo:
		l.moveRepo(task, coordinator, strings.TrimSpace(decision.Repo))
	case AwaitStories:
		l.propose(task.ID, coordinator, events.EpicWaits, map[string]any{"toYou": decision.ToYou})
	case Finish:
		if task.Kind == tasks.Epic { // you close an epic: there may be more you want in it
			l.propose(task.ID, coordinator, events.TaskNeedsYou, map[string]any{"question": "Every story of this epic is done. " +
				"Close it? Its summary:\n\n" + decision.Summary + "\n\nOr say what else it needs, and " + task.Coordinator + " plans it.",
				"options": []string{CloseEpic}, "closing": decision.Summary, "toYou": decision.ToYou})
			return
		}
		summary := decision.Summary
		if note := l.onDone(task, roster, decision); note != "" {
			summary += "\n\n" + note
		}
		if l.propose(task.ID, coordinator, events.TaskDone, map[string]any{"summary": summary, "toYou": decision.ToYou}) {
			l.removeWorktree(task)
		}
	}
}

// recoverDecision turns a panic while acting on a decision into an issue on
// its task: the coordinator hears why and decides again, and the loop, and
// the server with it, goes on serving the other tasks. Deferred by apply.
func (l *Loop) recoverDecision(task *tasks.Task, decision Decision) {
	r := recover()
	if r == nil {
		return
	}
	slog.Error("coordinator loop: acting on a decision panicked", "task", task.ID, "action", decision.Action, "panic", r,
		"stack", string(debug.Stack()))
	l.propose(task.ID, agentry, events.IssueRaised, map[string]any{
		"reason": fmt.Sprintf("Agentry failed acting on the %q decision: %v", decision.Action, r)})
}

// moveRepo moves the task to the repo the user named for it: a folder here,
// or a git address or archive, cloned here first beside the loop, as that
// can take minutes. The decision waits until the move is told, so a restart
// clones again. If it can't be worked in, the coordinator hears why, and
// can ask the user.
func (l *Loop) moveRepo(task *tasks.Task, coordinator events.Actor, repo string) {
	if !worktrees.IsAddress(repo) {
		l.tell(movedTo(task, coordinator, repo, repo, worktrees.IsRepo(repo)))
		return
	}
	l.beside(task, func(context.Context) events.Proposal {
		folder, err := worktrees.Clone(l.Clones, repo)
		reason := ""
		if err != nil {
			reason = sentence(err) + " Ask the user to give Agentry access to it, or to clone it here themselves."
		}
		return movedTo(task, coordinator, repo, folder, reason)
	})
}

// movedTo tells the move to folder, the repo named as repo, or why it
// couldn't be made.
func movedTo(task *tasks.Task, coordinator events.Actor, repo, folder, reason string) events.Proposal {
	if reason != "" {
		return told(task, agentry, events.IssueRaised, map[string]any{"reason": "the task couldn't move to " + repo + ": " + reason})
	}
	return told(task, coordinator, events.RepoChanged, map[string]any{"repo": folder})
}

// request opens a task for another team, for what this one needs from it,
// in the same repo, and makes this one wait for it.
func (l *Loop) request(task *tasks.Task, roster agents.Roster, coordinator events.Actor, decision Decision) {
	lead, _ := roster.Coordinator(roster.TeamNamed(decision.Team))
	asker, _ := roster.Get(task.Coordinator)
	description := decision.Request + "\n\n(Asked by the " + asker.TeamName() + " team's coordinator, " + task.Coordinator +
		", for " + task.ID + ": " + task.Title + ". When this is done, your summary goes back to them.)"
	limits := tasks.NewLimits(l.Defaults, task)
	child, refused := l.newTask(coordinator, map[string]any{
		"title": firstLine(decision.Request), "description": description, "repo": task.Repo, "coordinator": lead.Name,
		"teamChosen": true, "parent": task.ID, "depth": task.Depth + 1, "budget": limits.Budget, "maxTurns": limits.MaxTurns, "labels": listOf(task.Labels)})
	if refused != "" {
		l.propose(task.ID, agentry, events.IssueRaised, map[string]any{"reason": "the request to the " + lead.TeamName() + " team failed: " + refused})
		return
	}
	l.propose(task.ID, coordinator, events.TaskRequested, map[string]any{"child": child, "team": lead.TeamName(),
		"request": decision.Request, "coordinator": lead.Name})
}

// split makes the decision's stories, each a task of its team in the same
// repo, in your Backlog until you start it, and adds them to the task,
// which is an epic from then on. If one
// can't be made, the ones before it are kept and the coordinator hears why.
func (l *Loop) split(task *tasks.Task, roster agents.Roster, coordinator events.Actor, decision Decision) {
	own, _ := roster.Get(task.Coordinator)
	limits := tasks.NewLimits(l.Defaults, task)
	var ids, titles []string
	refused := ""
	for i, story := range decision.Stories {
		team := cmp.Or(roster.TeamNamed(story.Team), own.TeamName())
		lead, ok := roster.Coordinator(team)
		if !ok {
			refused = fmt.Sprintf("story %d is for the %s team, which has no coordinator", i+1, team)
			break
		}
		after := []string{}
		for _, ref := range story.After {
			after = append(after, storyID(ref, ids))
		}
		description := strings.TrimSpace(story.Brief) + "\n\n(A story of " + task.ID + ", the epic \"" + task.Title +
			"\". When it is done, your summary goes back to the epic's coordinator, " + task.Coordinator + ".)"
		var id string
		if id, refused = l.newTask(coordinator, map[string]any{
			"title": strings.TrimSpace(story.Title), "description": description, "repo": task.Repo, "coordinator": lead.Name,
			"teamChosen": true, "kindChosen": true, "epic": task.ID, "after": after, "parked": "backlog", "budget": limits.Budget, "maxTurns": limits.MaxTurns,
			"labels": listOf(task.Labels)}); refused != "" {
			refused = fmt.Sprintf("story %d wasn't made: %s", i+1, refused)
			break
		}
		ids, titles = append(ids, id), append(titles, id+" "+strings.TrimSpace(story.Title))
	}
	if len(ids) > 0 {
		l.propose(task.ID, coordinator, events.TaskSplit, map[string]any{"stories": ids, "titles": strings.Join(titles, "; "),
			"toYou": decision.ToYou})
	}
	if refused != "" {
		l.propose(task.ID, agentry, events.IssueRaised, map[string]any{"reason": refused})
	}
}

// storyID is the task a story builds on: the existing story ref names, or
// the one made for its number in this split. ParseDecision has already
// refused a number that isn't an earlier story's, and ids holds every story
// made before this one, so the index is in range.
func storyID(ref StoryRef, ids []string) string {
	if ref.ID != "" {
		return ref.ID
	}
	return ids[ref.Number-1]
}

// newTask makes a task with the next free id, and returns the id, or why it
// wasn't made.
func (l *Loop) newTask(by events.Actor, data map[string]any) (string, string) {
	created, rejection, err := l.store.Create("task-", events.Proposal{Actor: by, Type: events.TaskCreated, Data: data})
	switch {
	case err != nil:
		return "", "Agentry couldn't write it: " + err.Error()
	case rejection != nil:
		return "", rejection.Reason
	}
	return created.Stream, ""
}

func firstLine(text string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(text), "\n")
	if len([]rune(line)) > 80 {
		line = string([]rune(line)[:77]) + "..."
	}
	return line
}

func listOf(items []string) []string {
	if items == nil {
		return []string{}
	}
	return items
}
