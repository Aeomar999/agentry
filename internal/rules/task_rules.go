package rules

import (
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// A rule returns "" when the proposal may happen now, or the reason it may not.
type rule func(t *tasks.Task, p events.Proposal) string

// taskRules has one entry per task event type, except task.created, which
// Check handles. Each lists the conditions that must all hold.
var taskRules = map[events.Type]rule{
	events.WorktreeReady:      all(noWorktreeYet),
	events.WorktreeMoved:      all(worktreeReady, noOpenTurn, toAnotherMachine),
	events.TaskRenamed:        all(byTheCoordinatorOrYou, aNewTitle),
	events.TaskDirected:       all(noQuestionOpen), // an open question takes an answer instead
	events.TaskLabeled:        all(labelsAreValid),
	events.TaskBudgeted:       all(aPositiveBudget),
	events.TaskRanked:         all(),
	events.TaskHeld:           all(aKnownPlace),
	events.TaskReopened:       all(isDone),
	events.RepoChanged:        all(anotherRepo),
	events.AgentRenamedInTask: all(),

	events.TaskTransferred: all(byTheCoordinator, noOpenTurn, noQuestionOpen, notWaitingOnAnother),
	events.TaskRequested:   all(byTheCoordinator, noOpenTurn, noQuestionOpen, notWaitingOnAnother),
	events.RequestAnswered: all(waitingOnIt),
	events.TaskSplit:       all(byTheCoordinator, noOpenTurn, noQuestionOpen, notWaitingOnAnother, notAStory),
	events.StoryDone:       all(itsStory, notYetHeard),
	events.EpicWaits:       all(byTheCoordinator, noOpenTurn, anEpic),
	events.EpicJoined:      all(notAnEpic, inNoEpic, notItself),
	events.EpicLeft:        all(inThatEpic),
	events.StoryAdded:      all(anEpic, notItsStoryYet),
	events.StoryRemoved:    all(itsStory),

	events.FileAdded:      all(aSharedFileName, aFileHash),
	events.FileRemoved:    all(aSharedFile),
	events.FilesCollected: all(anEndedTurn, notCollectedYet),
	events.FilesDelivered: all(),
	events.FileFetching:   all(byTheCoordinator, noOpenTurn, noQuestionOpen, notFetchingAlready),
	events.FilesFetched:   all(approvedIfAnswer),

	events.TaskAssigned: all(byTheCoordinator, noOpenTurn, noQuestionOpen, notToTheCoordinator),
	events.TurnStarted:  all(noOpenTurn, noQuestionOpen, worktreeReady, isTheNextTurn),

	events.PromptAcknowledged: all(isTheOpenTurn, notYetAcknowledged, hashMatches),
	events.SessionStarted:     all(isTheOpenTurn),

	events.ActionRequested: all(byTheWorkingAgent, isNewAction),
	events.ActionAsked:     all(actionUndecided, notYetAsked),
	events.ActionApproved:  all(actionUndecided),
	events.ActionDenied:    all(actionUndecided),
	events.ActionResult:    all(actionApproved, noResultYet),

	events.PlanUpdated:  all(isTheOpenTurn),
	events.AgentWaiting: all(isTheOpenTurn),
	events.TurnEnded:    all(isTheOpenTurn, sameAgentAsTheTurn),
	events.IssueRaised:  all(),
	events.TurnStopped:  all(isTheOpenTurn),

	events.TaskNeedsYou: all(byTheCoordinatorOrAgentry, noOpenTurn, noQuestionOpen),
	events.TaskAnswered: all(questionOpen),
	events.TaskDone:     all(byTheCoordinatorOrYou, noOpenTurn, closedByYouIfAnEpic),
}

func all(rules ...rule) rule {
	return func(t *tasks.Task, p events.Proposal) string {
		for _, r := range rules {
			if reason := r(t, p); reason != "" {
				return reason
			}
		}
		return ""
	}
}

// ---- who ----------------------------------------------------------------------

func byTheCoordinator(t *tasks.Task, p events.Proposal) string {
	if p.Actor.ID != t.Coordinator {
		return p.Actor.ID + " is not this task's coordinator (" + t.Coordinator + ")"
	}
	return ""
}

func byTheCoordinatorOrAgentry(t *tasks.Task, p events.Proposal) string {
	if p.Actor.Kind == events.System {
		return ""
	}
	return byTheCoordinator(t, p)
}

func byTheCoordinatorOrYou(t *tasks.Task, p events.Proposal) string {
	if p.Actor.Kind == events.Human {
		return ""
	}
	return byTheCoordinator(t, p)
}

func aNewTitle(t *tasks.Task, p events.Proposal) string {
	if title := p.Text("title"); title == "" || title == t.Title {
		return "the task already has that title"
	}
	return ""
}

func aPositiveBudget(_ *tasks.Task, p events.Proposal) string {
	if p.Number("budget") <= 0 {
		return "a budget is a number of tokens above 0"
	}
	return ""
}

// labelsAreValid keeps labels short words: they are filters and chips.
func labelsAreValid(_ *tasks.Task, p events.Proposal) string {
	return CheckLabels(p.List("labels"))
}

// CheckLabels is "" for labels that may be used, or why not. Task creation
// runs it too.
func CheckLabels(labels []string) string {
	if len(labels) > 12 {
		return "at most 12 labels"
	}
	for _, label := range labels {
		if label == "" || len(label) > 32 || strings.TrimSpace(label) != label {
			return "a label is 1 to 32 characters with no space at either end: " + `"` + label + `"`
		}
	}
	return ""
}

// Places a task can be parked in: the board's To do and Backlog.
var Places = []string{"todo", "backlog"}

func aKnownPlace(_ *tasks.Task, p events.Proposal) string {
	if place := p.Text("place"); place != "" && !slices.Contains(Places, place) {
		return "a task is parked in todo or backlog, not " + place
	}
	return ""
}

func isDone(t *tasks.Task, _ events.Proposal) string {
	if !t.IsDone {
		return "the task isn't done, so there is nothing to reopen"
	}
	return ""
}

func byTheWorkingAgent(t *tasks.Task, p events.Proposal) string {
	if t.OpenTurn() == nil {
		return "no agent is working on the task"
	}
	if working := t.OpenTurn().Agent; p.Actor.ID != working {
		return p.Actor.ID + " is not the agent working on the task (" + working + ")"
	}
	return ""
}

func notToTheCoordinator(t *tasks.Task, p events.Proposal) string {
	if p.Text("agent") == t.Coordinator {
		return "the coordinator can't assign the task to itself"
	}
	return ""
}

// ---- turns --------------------------------------------------------------------

func noOpenTurn(t *tasks.Task, _ events.Proposal) string {
	if turn := t.OpenTurn(); turn != nil {
		return turn.Agent + " is still working (" + turn.ID + ")"
	}
	return ""
}

func isTheNextTurn(t *tasks.Task, p events.Proposal) string {
	if p.Correlation != t.NextTurnID() {
		return "the next turn is " + t.NextTurnID() + ", not " + p.Correlation
	}
	return ""
}

func anotherRepo(t *tasks.Task, p events.Proposal) string {
	if repo := p.Text("repo"); repo == t.Repo && t.MovingTo == "" || repo == t.MovingTo {
		return "the task is in " + repo + " already"
	}
	return ""
}

func notWaitingOnAnother(t *tasks.Task, _ events.Proposal) string {
	if t.WaitingOn != "" {
		return "the task is waiting for " + t.WaitingOn + " already"
	}
	return ""
}

func waitingOnIt(t *tasks.Task, p events.Proposal) string {
	if t.WaitingOn != p.Text("child") {
		return "the task isn't waiting for " + p.Text("child")
	}
	return ""
}

// notAStory keeps to two levels: an epic's story has no stories of its own.
func notAStory(t *tasks.Task, _ events.Proposal) string {
	if t.Epic != "" {
		return "the task is a story of " + t.Epic + ", and a story can't be split; the epic's coordinator adds stories"
	}
	return ""
}

func anEpic(t *tasks.Task, _ events.Proposal) string {
	if t.Kind != tasks.Epic {
		return "only an epic waits for its stories"
	}
	return ""
}

// closedByYouIfAnEpic: an epic closes when you approve it, and only then,
// since you may have more for it; a story closes when its coordinator says.
func closedByYouIfAnEpic(t *tasks.Task, p events.Proposal) string {
	switch {
	case t.Kind == tasks.Epic && p.Actor.Kind != events.Human:
		return "an epic closes when the user approves it"
	case t.Kind != tasks.Epic && p.Actor.Kind == events.Human:
		return "a task is finished by its coordinator; move it to Done on the board to have it finished as it is"
	}
	return ""
}

func notAnEpic(t *tasks.Task, _ events.Proposal) string {
	if t.Kind == tasks.Epic {
		return "the task is an epic, and an epic can't be a story of another"
	}
	return ""
}

func inNoEpic(t *tasks.Task, _ events.Proposal) string {
	if t.Epic != "" {
		return "the task is a story of " + t.Epic + " already; take it out of that one first"
	}
	return ""
}

func notItself(t *tasks.Task, p events.Proposal) string {
	if p.Text("epic") == t.ID {
		return "a task can't be a story of itself"
	}
	return ""
}

func inThatEpic(t *tasks.Task, p events.Proposal) string {
	if t.Epic != p.Text("epic") {
		return "the task isn't a story of " + p.Text("epic")
	}
	return ""
}

func notItsStoryYet(t *tasks.Task, p events.Proposal) string {
	if slices.Contains(t.Stories, p.Text("story")) {
		return p.Text("story") + " is one of this epic's stories already"
	}
	return ""
}

func itsStory(t *tasks.Task, p events.Proposal) string {
	if !slices.Contains(t.Stories, p.Text("story")) {
		return p.Text("story") + " is not a story of this task"
	}
	return ""
}

func notYetHeard(t *tasks.Task, p events.Proposal) string {
	if slices.Contains(t.StoriesDone, p.Text("story")) {
		return "the task has heard that " + p.Text("story") + " is done already"
	}
	return ""
}

func isTheOpenTurn(t *tasks.Task, p events.Proposal) string {
	if turn := t.OpenTurn(); turn == nil || turn.ID != p.Correlation {
		return p.Correlation + " is not the turn in progress"
	}
	return ""
}

func sameAgentAsTheTurn(t *tasks.Task, p events.Proposal) string {
	if agent := t.OpenTurn().Agent; p.Text("agent") != agent {
		return p.Correlation + " belongs to " + agent + ", not " + p.Text("agent")
	}
	return ""
}

func notYetAcknowledged(t *tasks.Task, p events.Proposal) string {
	if t.Turn(p.Correlation).Acknowledged {
		return "the prompt of " + p.Correlation + " was already acknowledged"
	}
	return ""
}

func hashMatches(t *tasks.Task, p events.Proposal) string {
	if p.Text("hash") != t.Turn(p.Correlation).Hash {
		return "the agent received different text than was sent (hash mismatch)"
	}
	return ""
}

func worktreeReady(t *tasks.Task, _ events.Proposal) string {
	if t.Worktree == nil {
		return "the task's worktree isn't ready"
	}
	return ""
}

func toAnotherMachine(t *tasks.Task, p events.Proposal) string {
	if p.Text("worker") == t.Worktree.Worker {
		return "the task's work is on " + t.Worktree.Worker + " already"
	}
	return ""
}

func noWorktreeYet(t *tasks.Task, _ events.Proposal) string {
	if t.Worktree != nil {
		return "the task already has a worktree"
	}
	return ""
}

// ---- actions ------------------------------------------------------------------

func isNewAction(t *tasks.Task, p events.Proposal) string {
	if t.Action(p.Correlation) != nil {
		return "action " + p.Correlation + " was already requested"
	}
	return ""
}

func actionUndecided(t *tasks.Task, p events.Proposal) string {
	action := t.Action(p.Correlation)
	switch {
	case action == nil:
		return "action " + p.Correlation + " doesn't exist"
	case action.Decision != tasks.Undecided:
		return "action " + p.Correlation + " was already " + string(action.Decision)
	}
	return ""
}

func notYetAsked(t *tasks.Task, p events.Proposal) string {
	if t.Action(p.Correlation).Asked {
		return "action " + p.Correlation + " was already asked about"
	}
	return ""
}

func actionApproved(t *tasks.Task, p events.Proposal) string {
	action := t.Action(p.Correlation)
	switch {
	case action == nil:
		return "action " + p.Correlation + " doesn't exist"
	case action.Decision != tasks.Approved:
		return "action " + p.Correlation + " was never approved"
	}
	return ""
}

func noResultYet(t *tasks.Task, p events.Proposal) string {
	if t.Action(p.Correlation).HasResult {
		return "action " + p.Correlation + " already has a result"
	}
	return ""
}

// ---- questions ----------------------------------------------------------------

func noQuestionOpen(t *tasks.Task, _ events.Proposal) string {
	if t.Question != "" {
		return "the task is waiting for your answer"
	}
	return ""
}

func questionOpen(t *tasks.Task, _ events.Proposal) string {
	if t.Question == "" {
		return "the task isn't waiting for an answer"
	}
	return ""
}

// ---- shared files ---------------------------------------------------------------

func aSharedFileName(_ *tasks.Task, p events.Proposal) string {
	if name, err := sharedfiles.CleanName(p.Text("name")); err != nil || name != p.Text("name") {
		return "not a shared file's name: " + p.Text("name")
	}
	return ""
}

func aFileHash(_ *tasks.Task, p events.Proposal) string {
	if hash := p.Text("hash"); len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" {
		return "not a file's hash"
	}
	return ""
}

func aSharedFile(t *tasks.Task, p events.Proposal) string {
	if _, ok := t.Files[p.Text("name")]; !ok {
		return "the task has no shared file " + p.Text("name")
	}
	return ""
}

func anEndedTurn(t *tasks.Task, p events.Proposal) string {
	if turn := t.Turn(p.Text("turn")); turn == nil || !turn.Ended {
		return p.Text("turn") + " isn't a turn that has ended"
	}
	return ""
}

func notCollectedYet(t *tasks.Task, p events.Proposal) string {
	if t.FilesCollected(p.Text("turn")) {
		return p.Text("turn") + "'s files were read back already"
	}
	return ""
}

func notFetchingAlready(t *tasks.Task, _ events.Proposal) string {
	if t.Fetching != nil {
		return "a file is on its way already: " + t.Fetching.Machine + ":" + t.Fetching.Path
	}
	return ""
}

// approvedIfAnswer: a fetch you approved answers the question that asked.
func approvedIfAnswer(t *tasks.Task, p events.Proposal) string {
	if p.Flag("answers") && (t.Question == "" || t.Asked.Fetch == nil) {
		return "no fetch waits for your approval"
	}
	return ""
}
