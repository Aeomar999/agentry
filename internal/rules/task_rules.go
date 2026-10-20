package rules

import (
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
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
	events.TaskBudgeted:       all(aBudget),
	events.TaskTurnsLimited:   all(aTurnLimit),
	events.TaskRanked:         all(),
	events.TaskHeld:           all(aKnownPlace),
	events.TaskWrappedUp:      all(byTheCoordinatorOrAgentry, wrappingUp, noOpenTurn),
	events.TaskReopened:       all(isDone),
	events.RepoChanged:        all(byTheCoordinatorOrYou, notUndoingYourMove, anotherRepo),
	events.AgentRenamedInTask: all(),
	events.CoordinatorMoved:   all(fromItsCoordinator),

	events.TaskTransferred: all(byTheCoordinator, notUndoingYourMove, noOpenTurn, noQuestionOpen, notWaitingOnAnother),
	events.TaskRequested:   all(byTheCoordinator, notUndoingYourMove, noOpenTurn, noQuestionOpen, notWaitingOnAnother),
	events.RequestAnswered: all(waitingOnIt),
	events.TaskSplit:       all(byTheCoordinator, notUndoingYourMove, noOpenTurn, noQuestionOpen, notWaitingOnAnother, notAStory),
	events.StoryDone:       all(itsStory, notYetHeard),
	events.EpicWaits:       all(byTheCoordinator, notUndoingYourMove, noOpenTurn, anEpic),
	events.EpicJoined:      all(notAnEpic, inNoEpic, notItself),
	events.EpicLeft:        all(inThatEpic),
	events.StoryAdded:      all(anEpic, notItsStoryYet),
	events.StoryRemoved:    all(itsStory),

	events.FollowUpsSkipped: all(aClosedEpic, notSkippedYet),

	events.FileAdded:      all(aSharedFileName, aFileHash),
	events.FileRemoved:    all(aSharedFile),
	events.FilesCollected: all(anEndedTurn, notCollectedYet),
	events.FilesDelivered: all(),
	events.FileFetching:   all(byTheCoordinator, notUndoingYourMove, noOpenTurn, noQuestionOpen, notFetchingAlready),
	events.FilesFetched:   all(approvedIfAnswer),

	events.TaskAssigned: all(byTheCoordinator, notUndoingYourMove, noOpenTurn, noQuestionOpen, notToTheCoordinator),
	events.TurnStarted:  all(noOpenTurn, noQuestionOpen, worktreeReady, isTheNextTurn),

	events.PromptAcknowledged: all(isTheOpenTurn, notYetAcknowledged, hashMatches),
	events.SessionStarted:     all(isTheOpenTurn),

	events.ActionRequested: all(byTheWorkingAgent, isNewAction),
	events.ActionAsked:     all(actionUndecided, notYetAsked, inTheOpenTurn),
	events.ActionApproved:  all(actionUndecided, heldForYouAtWork, aFittingPass),
	events.ActionDenied:    all(actionUndecided, heldForYouAtWork),
	events.ActionHeld:      all(actionUndecided, wasAsked, notYetHeld),
	events.ActionResult:    all(actionApproved, noResultYet),

	events.PlanUpdated:  all(isTheOpenTurn),
	events.AgentWaiting: all(isTheOpenTurn),
	events.TurnEnded:    all(isTheOpenTurn, sameAgentAsTheTurn),
	events.TurnLimited:  all(isTheOpenTurn, sameAgentAsTheTurn),
	events.TurnResumed:  all(isTheOpenTurn, sameAgentAsTheTurn, turnIsLimited),
	events.IssueRaised:  all(),
	events.TurnStopped:  all(isTheOpenTurn),

	events.TaskNeedsYou: all(byTheCoordinatorOrAgentry, notUndoingYourMove, noOpenTurn, noQuestionOpen),
	events.TaskAnswered: all(questionOpen),
	events.TaskDone:     all(byTheCoordinatorOrYou, notUndoingYourMove, noOpenTurnUnlessMoved, closedByYouIfAnEpic),

	events.TaskArchived:   all(doneToArchive, aMarker),
	events.TaskUnarchived: all(anArchivedTask), // Check lets it onto an archived task; a task here is whole
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

// fromItsCoordinator: the coordinator a task moves from is the one it has.
func fromItsCoordinator(t *tasks.Task, p events.Proposal) string {
	if p.Text("from") != t.Coordinator {
		return "this task's coordinator is " + t.Coordinator + ", not " + p.Text("from")
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

// aBudget is a number of tokens, or 0 for no limit.
func aBudget(_ *tasks.Task, p events.Proposal) string {
	if p.Number("budget") < 0 {
		return "a budget is a number of tokens, or 0 for no limit"
	}
	return ""
}

// aTurnLimit is a number of turns, or 0 for no limit.
func aTurnLimit(_ *tasks.Task, p events.Proposal) string {
	if p.Number("maxTurns") < 0 {
		return "a limit of turns is a number of turns, or 0 for no limit"
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

// notUndoingYourMove: when you move a task to Done, To do or the Backlog
// yourself, it stays there. Its coordinator only wraps up: it starts no new
// work, asks nothing, and doesn't finish, move or pass the task on.
func notUndoingYourMove(t *tasks.Task, p events.Proposal) string {
	if p.Actor.Kind != events.Agent {
		return ""
	}
	switch {
	case t.DoneByYou:
		return "the user moved this task to Done; it stays done"
	case t.Parked != "":
		return "the user moved this task to " + tasks.PlaceName(t.Parked) + "; it waits there until they move it on"
	}
	return ""
}

// taskRosterRules check a task's event against its team's agents too.
var taskRosterRules = map[events.Type]func(t *tasks.Task, roster agents.Roster, p events.Proposal) string{
	events.TaskAssigned: toTheColumnYouMovedItTo,
}

// toTheColumnYouMovedItTo: a handoff doesn't undo your move of the card to
// In progress, Review or Shipping.
func toTheColumnYouMovedItTo(t *tasks.Task, roster agents.Roster, p events.Proposal) string {
	return t.UndoesYourColumn(p.Text("agent"), roster)
}

func wrappingUp(t *tasks.Task, _ events.Proposal) string {
	if t.WrapUp == nil {
		return "the task has no wrap-up going: only a task the user moved to Done, To do or the Backlog has one"
	}
	return ""
}

// ---- turns --------------------------------------------------------------------

// noOpenTurnUnlessMoved: your move to Done on the board stops the turn in
// progress; anyone else waits for it to end.
func noOpenTurnUnlessMoved(t *tasks.Task, p events.Proposal) string {
	if p.Actor.Kind == events.Human && p.Flag("moved") {
		return ""
	}
	return noOpenTurn(t, p)
}

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

func aClosedEpic(t *tasks.Task, _ events.Proposal) string {
	if t.Kind != tasks.Epic || !t.IsDone {
		return "only an epic you closed gets a Visionaries session after it"
	}
	return ""
}

func notSkippedYet(t *tasks.Task, p events.Proposal) string {
	if t.Skipped == int64(p.Number("close")) {
		return "Agentry has said already why this close got no Visionaries session"
	}
	return ""
}

// closedByYouIfAnEpic: an epic closes when you approve it or move it to
// Done, and only then, since you may have more for it; a story closes when
// its coordinator says, or when you move it to Done.
func closedByYouIfAnEpic(t *tasks.Task, p events.Proposal) string {
	switch {
	case t.Kind == tasks.Epic && p.Actor.Kind != events.Human:
		return "an epic closes when the user approves it"
	case t.Kind != tasks.Epic && p.Actor.Kind == events.Human && !p.Flag("moved"):
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

func turnIsLimited(t *tasks.Task, p events.Proposal) string {
	if t.OpenTurn().Limited == "" {
		return p.Correlation + " is not waiting for a usage limit"
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

// inTheOpenTurn: an action asked for in a turn that has stopped since, as
// your move to To do or Done stops it, waits for no one. Refused, the hook
// denies it. Its approval and denial stay open, so the hook still records
// what it told the agent.
func inTheOpenTurn(t *tasks.Task, p events.Proposal) string {
	if turn := t.OpenTurn(); turn == nil || turn.ID != t.Action(p.Correlation).Turn {
		return "action " + p.Correlation + "'s turn is over"
	}
	return ""
}

func wasAsked(t *tasks.Task, p events.Proposal) string {
	if !t.Action(p.Correlation).Asked {
		return "action " + p.Correlation + " was never asked about"
	}
	return ""
}

func notYetHeld(t *tasks.Task, p events.Proposal) string {
	if t.Action(p.Correlation).Held {
		return "action " + p.Correlation + " is held already"
	}
	return ""
}

// heldForYouAtWork: no hook waits on a held action, so only you decide it,
// and only while the task is at work, as only then does it show in Needs you.
func heldForYouAtWork(t *tasks.Task, p events.Proposal) string {
	switch {
	case !t.Action(p.Correlation).Held:
		return ""
	case p.Actor.Kind != events.Human:
		return "action " + p.Correlation + " is held for the user to decide"
	case !t.AtWork():
		return "action " + p.Correlation + " is held, and the task isn't at work: move it back to decide it"
	}
	return ""
}

// aFittingPass: an approval that uses a held action's pass names one you
// approved, not used yet, for the same agent, tool and command, asked for
// in that agent's first turn after its coordinator heard your approval.
func aFittingPass(t *tasks.Task, p events.Proposal) string {
	id := p.Text("pass")
	if id == "" {
		return ""
	}
	pass, action := t.Action(id), t.Action(p.Correlation)
	switch {
	case pass == nil || !pass.Held || pass.Decision != tasks.Approved:
		return "action " + id + " is not a held action the user approved"
	case pass.PassUsed:
		return "action " + id + "'s pass was used already"
	case pass.Agent != action.Agent:
		return "action " + id + " was asked for by another agent"
	case pass.PassTurn != action.Turn:
		return "action " + id + "'s pass works only in " + pass.Agent + "'s first turn after its coordinator heard the approval"
	case pass.Tool != action.Tool || pass.Command != action.Command:
		return "action " + id + " was for another command"
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
	if !isSHA256(p.Text("hash")) {
		return "not a file's hash"
	}
	return ""
}

// isSHA256 says whether text is a sha256 in lower-case hex, as the shared
// files' store names its files and blobs.
func isSHA256(text string) bool {
	return len(text) == 64 && strings.Trim(text, "0123456789abcdef") == ""
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
