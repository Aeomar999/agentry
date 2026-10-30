package coordinator

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"runtime/debug"
	"slices"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/turns"
	"gitlab.com/dev.chempa/agentry/internal/workers"
	"gitlab.com/dev.chempa/agentry/internal/workspace"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

// Loop reads the log and, for every task an event touched, does the one move
// Next says. Its own proposals are events too, so each one wakes it again:
// the task moves step by step, and a restart picks up where it was.
type Loop struct {
	store  *store.Store        // the log: what the loop reads and proposes
	events extensions.EventLog // the store, read-only: what an extension's hooks read of the log
	state  *current.State      // every task and roster, as of the log's last event
	runner *turns.Runner
	fleet  *machines.Fleet
	files  fileWork // shared files being moved (files.go)
	held   held     // tasks waiting for a slot (slots.go)

	startedAt int64 // the log's last position when the loop was made: what came before is history (followups.go)

	Clones     string                         // where repos the user names by a git address are cloned, for a "move"
	Defaults   tasks.Limits                   // the server's limits, for a task Agentry makes that comes from no other task (tasks.NewLimits)
	Extensions []extensions.Extension         // every extension in this binary: their coordinator hooks add to briefs and act on dones (extensions.go)
	Logs       map[string]extensions.Proposer // each extension's proposer from extensions.Checked, by its Name: all its hooks may write through

	Settings func(extension string) extensions.Settings // each extension's own table in the settings file, for its hooks; nil for none
}

var agentry = events.Actor{Kind: events.System, ID: tasks.Loop}

// NewLoop makes the loop before the server takes requests, so a close
// written once it does is never mistaken for history.
func NewLoop(s *store.Store, state *current.State, runner *turns.Runner, fleet *machines.Fleet) *Loop {
	return &Loop{store: s, events: extensions.ReadOnly(s), state: state, runner: runner, fleet: fleet, startedAt: s.LastPosition()}
}

// gitTimeout is how long a machine has to make, move or remove a worktree:
// a worker's first copy of a repo can take a while.
const gitTimeout = 10 * time.Minute

func (l *Loop) Run(ctx context.Context) error {
	reader := l.store.NewReader(0)
	defer reader.Close()
	for {
		batch, err := reader.Next(ctx)
		if err != nil {
			return err
		}
		l.take(batch)
	}
}

// take acts on each task a batch of the log touched, then on those waiting
// for a slot, as the batch may have freed one, then follows up the closes
// in it.
func (l *Loop) take(batch []events.Event) {
	for _, id := range touchedTasks(l.store.Catalogue(), batch) {
		l.act(id)
	}
	l.wakeHeld()
	l.followUps(batch)
}

// touchedTasks are the tasks a batch changed, each once, in order. A
// foreign stream is no task's, so the loop never reads one as a task. When
// tasks can be archived, an archived task is skipped here too.
func touchedTasks(catalogue *events.Catalogue, batch []events.Event) []string {
	seen := map[string]bool{}
	var ids []string
	for _, e := range batch {
		if !catalogue.IsTaskStream(e.Stream) || e.Type == events.ProposalRejected || seen[e.Stream] {
			continue
		}
		seen[e.Stream] = true
		ids = append(ids, e.Stream)
	}
	return ids
}

func (l *Loop) act(id string) {
	delete(l.held, id) // held again below if its move is still a turn with no slot
	task, roster, err := l.load(id)
	if errors.Is(err, current.ErrArchived) || errors.Is(err, current.ErrErased) {
		return // done or gone, and it takes nothing more
	}
	if err != nil || task == nil {
		slog.Error("coordinator loop: reading the task", "task", id, "err", err)
		return
	}
	if task.OpenTurn() == nil {
		l.runner.Stop(id) // the turn is over: end its session
	}
	// A request between teams never stalls: a finished task answers the one
	// that asked for it, and a waiting task checks on its own, so an answer
	// missed (as across a restart) is given on the next pass. A task you
	// finished answers once its coordinator has wrapped up, with its summary.
	finished := task.IsDone && task.WrapUp == nil
	if finished && task.Parent != "" {
		l.answer(task)
	}
	if task.WaitingOn != "" {
		if child, _, err := l.load(task.WaitingOn); err == nil && child != nil && child.IsDone {
			l.answer(child)
		}
	}
	// Stories the same way: a finished one reports to its epic, and the
	// epic checks on the rest, starting any whose stories before it are done.
	if finished && task.Epic != "" {
		l.storyDone(task)
	}
	if task.Kind == tasks.Epic && !task.IsDone {
		l.wakeStories(task)
	}
	if !task.IsDone && task.Worktree == nil && l.blocked(task) {
		return // it starts from dev once the stories it builds on are in
	}
	if task.Fetching != nil && !task.IsDone {
		fetch := *task.Fetching
		l.beside(task, func(ctx context.Context) events.Proposal { return l.fetch(ctx, task, fetch) })
		return
	}

	switch move := Next(task, roster); move.Kind {
	case PrepareWorktree:
		l.prepareWorktree(task)
	case StartTurn:
		if l.holdBack(task) {
			return
		}
		l.startTurn(task, roster, move)
	case ApplyDecision:
		if why := l.unreadable(task, roster, move.Decision); why != "" {
			l.propose(task.ID, agentry, events.IssueRaised, map[string]any{"reason": why})
			return
		}
		l.apply(task, roster, move.Decision)
	case RaiseIssue:
		l.propose(task.ID, agentry, events.IssueRaised, map[string]any{"reason": move.Reason})
	case AskYou:
		l.askAt(task, move.Reason, move.Limit)
	case EndWrapUp:
		l.endWrapUp(task, move)
	}
}

// endWrapUp ends the coordinator's last turn after you moved the task, with
// its summary, or why it has none. A task you finished is cleaned up then,
// as one its coordinator finished is.
func (l *Loop) endWrapUp(task *tasks.Task, move Move) {
	by, data := agentry, map[string]any{"reason": move.Reason}
	if move.Reason == "" {
		by = events.Actor{Kind: events.Agent, ID: task.Coordinator}
		data = map[string]any{"summary": move.Decision.Summary, "toYou": move.Decision.ToYou}
	}
	if l.propose(task.ID, by, events.TaskWrappedUp, data) && task.IsDone && task.Worktree != nil {
		l.removeWorktree(task)
	}
}

// prepareWorktree makes the task's worktree here, where its repo is. If an
// agent that runs on a worker takes the task, the work moves there.
func (l *Loop) prepareWorktree(task *tasks.Task) {
	here, _ := l.fleet.Get(workers.Local)
	w, err := l.prepare(here, task, "")
	if err != nil {
		l.askYou(task, "Agentry couldn't set up the task's worktree: "+sentence(err)+" Fix it and answer, and it tries again.")
		return
	}
	l.propose(task.ID, workerActor(here), events.WorktreeReady, map[string]any{
		"repo": w.Repo, "worker": here.Name(), "path": w.Path, "branch": w.Branch, "base": w.Base, "port": w.Port})
}

func (l *Loop) prepare(m machines.Machine, task *tasks.Task, base string) (worktrees.Worktree, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	origin, _ := worktrees.Origin(task.Repo)
	return m.Prepare(ctx, machines.WorktreeRequest{Task: task.ID, Repo: task.Repo, Base: base,
		Ports: l.portsInUse(m.Name()), Origin: origin, RepoKey: worktrees.RepoKey(task.Repo)})
}

// startTurn starts the agent's turn on a machine it may run on, moving the
// work there first if it is somewhere else.
func (l *Loop) startTurn(task *tasks.Task, roster agents.Roster, move Move) {
	ctx, cancel := context.WithTimeout(context.Background(), machines.OfflineGrace)
	defer cancel()
	if move.Machine != "" {
		// The coordinator chose it: if it can't be had, the coordinator
		// hears why and chooses again.
		target, err := l.fleet.WaitOn(ctx, move.Agent, move.Machine)
		if err != nil {
			l.propose(task.ID, agentry, events.IssueRaised, map[string]any{"reason": "the turn couldn't start where you chose: " + sentence(err)})
			return
		}
		l.startOn(task, roster, move, target)
		return
	}
	target, err := l.fleet.WaitFor(ctx, move.Agent, task.Worktree.Worker)
	if err != nil {
		l.askYou(task, sentence(err)+" Bring one of its workers online, or change where it runs, and answer to carry on.")
		return
	}
	l.startOn(task, roster, move, target)
}

// startOn starts the turn on target, moving the work there first if it is
// somewhere else.
func (l *Loop) startOn(task *tasks.Task, roster agents.Roster, move Move, target machines.Machine) {
	if last := task.LastEndedTurn(); last != nil && !task.FilesCollected(last.ID) {
		l.readyForTurn(task, task.Worktree.Worker) // read the last turn's files back where it ran, before the work moves
		return
	}
	if target.Name() != task.Worktree.Worker {
		l.moveWork(task, target, move.Agent) // when it has moved, the loop comes back here
		return
	}
	if !l.readyForTurn(task, target.Name()) {
		return // the task's files are on their way: the loop comes back here
	}
	session := l.runner.SessionToResume(task, move.Agent)
	if err := l.runner.Start(task, move.Agent, l.briefFor(task, roster, move, session), session); err != nil {
		slog.Error("coordinator loop: starting a turn", "task", task.ID, "agent", move.Agent.Name, "err", err)
	}
}

// briefFor is the brief for the agent's turn in session: what has changed
// since for one that goes on with its last session; for a fresh one, the
// whole of it, or the handoff for an agent that checks the work.
func (l *Loop) briefFor(task *tasks.Task, roster agents.Roster, move Move, session turns.Session) string {
	shared, _ := l.filesFor(task)
	agent := move.Agent
	if session.Resumes() {
		if agent.Coordinates {
			return resumedCoordinatorBrief(task, agent, roster, session.Seen, session.From, l.familyOf(task), l.machines(), shared, l.state, l.Extensions)
		}
		return resumedWorkerBrief(task, agent, roster, move.Handoff, session.Seen, session.From, shared)
	}
	if agent.Coordinates {
		return coordinatorBrief(task, agent, roster, l.familyOf(task), l.machines(), shared, l.state, l.Extensions)
	}
	if agent.Column == agents.Reviews {
		return handoffBrief(task, agent, roster, move.Handoff, shared, l.commitsToReview(task))
	}
	return workerBrief(task, agent, roster, move.Handoff, shared)
}

// commitsTimeout is how long the machine the work is on has to list the
// task branch's commits for a handoff: a brief shouldn't wait on it.
const commitsTimeout = 10 * time.Second

// commitsToReview is the task branch's commits, newest first, for a
// checking agent's handoff; none, with a warning, when the machine the
// work is on can't list them in time. The brief says so and gives the
// commands instead.
func (l *Loop) commitsToReview(task *tasks.Task) []workspace.Commit {
	ctx, cancel := context.WithTimeout(context.Background(), commitsTimeout)
	defer cancel()
	commits, err := l.fleet.Of(task.Worktree.Worker).Commits(ctx, workspace.Workspace{Dir: task.Worktree.Path, Base: task.Worktree.Base})
	if err != nil {
		slog.Warn("coordinator loop: listing the commits to review", "task", task.ID, "err", err)
		return nil
	}
	return commits
}

// moveWork takes the task's work from the machine it is on to target: the
// first commits what was left and hands the branch to the hub, then target
// makes its worktree from it.
func (l *Loop) moveWork(task *tasks.Task, target machines.Machine, agent agents.Agent) {
	from, online := l.fleet.Get(task.Worktree.Worker)
	if !online {
		l.askYou(task, "The task's work is on "+task.Worktree.Worker+", which is offline, and "+agent.Name+
			" works on "+target.Name()+". Bring "+task.Worktree.Worker+" online and answer to carry on.")
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	message := fmt.Sprintf("Agentry: work left uncommitted on %s, moving to %s for %s", from.Name(), target.Name(), agent.Name)
	if err := from.HandOff(ctx, l.worktreeOf(task), message); err != nil {
		l.askYou(task, "Agentry couldn't move the work off "+from.Name()+": "+sentence(err)+" Fix it and answer, and it tries again.")
		return
	}
	w, err := l.prepare(target, task, task.Worktree.Base)
	if err != nil {
		l.askYou(task, "Agentry couldn't set the work up on "+target.Name()+": "+sentence(err)+
			" Its branch is safe in "+task.Repo+". Fix it and answer, and it tries again.")
		return
	}
	l.propose(task.ID, workerActor(target), events.WorktreeMoved, map[string]any{
		"worker": target.Name(), "from": from.Name(), "path": w.Path, "port": w.Port})
}

func (l *Loop) worktreeOf(task *tasks.Task) worktrees.Worktree {
	return worktrees.Worktree{Repo: task.Worktree.Repo, Path: task.Worktree.Path, Branch: task.Worktree.Branch,
		Base: task.Worktree.Base, Port: task.Worktree.Port}
}

func (l *Loop) askYou(task *tasks.Task, question string) { l.askAt(task, question, "") }

// askAt asks you, saying which limit stopped the task, if one did.
func (l *Loop) askAt(task *tasks.Task, question, limit string) {
	data := map[string]any{"question": question}
	if limit != "" {
		data["limit"] = limit
	}
	l.propose(task.ID, agentry, events.TaskNeedsYou, data)
}

func workerActor(m machines.Machine) events.Actor {
	return events.Actor{Kind: events.Worker, ID: m.Name()}
}

// sentence is an error as the end of a sentence.
func sentence(err error) string { return strings.TrimRight(err.Error(), ". ") + "." }

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

// storyDone gives a finished story's summary to its epic, if the epic
// hasn't heard it yet. A closed epic hears nothing more; reopened, it
// hears its finished stories then.
func (l *Loop) storyDone(story *tasks.Task) {
	epic, _, err := l.load(story.Epic)
	if err != nil || epic == nil || epic.IsDone || !slices.Contains(epic.Stories, story.ID) || slices.Contains(epic.StoriesDone, story.ID) {
		return
	}
	l.propose(epic.ID, agentry, events.StoryDone, map[string]any{"story": story.ID, "summary": story.Summary,
		"coordinator": story.Coordinator})
}

// wakeStories hears from the epic's finished stories and starts any that
// can start now.
func (l *Loop) wakeStories(epic *tasks.Task) {
	for _, id := range openStories(epic) {
		story, _, err := l.load(id)
		switch {
		case err != nil || story == nil:
		case story.IsDone:
			l.storyDone(story)
		case story.Worktree == nil:
			l.act(id)
		}
	}
}

// blocked is a story whose stories before it aren't done: its worktree
// would start from a dev without their work. An archived story is done,
// and an erased one blocks nothing: nothing of it is left to wait for.
func (l *Loop) blocked(story *tasks.Task) bool {
	for _, id := range story.After {
		before, _, err := l.load(id)
		if errors.Is(err, current.ErrArchived) || errors.Is(err, current.ErrErased) {
			continue
		}
		if err != nil || before == nil || !before.IsDone {
			return true
		}
	}
	return false
}

// answer gives a finished task's summary to the task that asked for it, if
// that one still waits for it. An archived asker never does, so it is not
// unarchived for the answer: only a done task is archived, and a done task
// waits on none. Moved to Done, it stops waiting; its coordinator finishes
// it only from a turn, which a waiting task doesn't get (Next); and the
// rules refuse task.request_answered to a done task.
func (l *Loop) answer(child *tasks.Task) {
	parent, _, err := l.load(child.Parent)
	if err != nil || parent == nil || parent.WaitingOn != child.ID {
		return
	}
	l.propose(parent.ID, agentry, events.RequestAnswered, map[string]any{"child": child.ID, "summary": child.Summary,
		"coordinator": child.Coordinator})
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

// removeWorktree cleans up a finished task, unless its branch isn't in dev
// yet: then the worktree is kept, so no work is lost (proof 06). On a
// worker, the hub's copy of the branch goes too once the worker's is merged.
func (l *Loop) removeWorktree(task *tasks.Task) {
	m := l.fleet.Of(task.Worktree.Worker)
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	err := m.Remove(ctx, l.worktreeOf(task))
	if err == nil && !machines.IsLocal(m.Name()) {
		err = worktrees.DeleteBranch(task.Worktree.Repo, task.Worktree.Branch)
	}
	if err != nil && !errors.Is(err, worktrees.ErrNotMerged) {
		slog.Warn("coordinator loop: removing the worktree", "task", task.ID, "err", err)
	}
}

// portsInUse are the ports of tasks whose work is on the machine and not
// over: unfinished, or done by you and still wrapping up, since a reopen
// then works in the same worktree. A broken task's port counts too, read
// from its stream, since nothing says its worktree has gone. An archived
// task is done, so it is skipped like one, without reading its stream, and
// an erased one has no stream left to read.
func (l *Loop) portsInUse(machine string) []int {
	var ports []int
	for _, id := range l.state.TaskIDs() {
		task, err := l.state.Task(id)
		if errors.Is(err, current.ErrArchived) || errors.Is(err, current.ErrErased) {
			continue
		}
		if err != nil {
			if port, ok := l.lastPort(id, machine); ok {
				ports = append(ports, port)
			}
			continue
		}
		if task != nil && (!task.IsDone || task.WrapUp != nil) && task.Worktree != nil && task.Worktree.Worker == machine {
			ports = append(ports, task.Worktree.Port)
		}
	}
	return ports
}

// lastPort is the port a task's stream last gave its worktree, if that
// worktree is on the machine: the worker and port of its last
// worktree.ready or worktree.moved, read as tasks' apply reads them. It
// needs none of the task's other state, so it works on a broken task. A
// reopen or a repo move that clears the worktree is ignored, so a broken
// task may hold a stale port: one port at most, which is fine.
func (l *Loop) lastPort(id, machine string) (int, bool) {
	stream, err := l.store.Stream(id)
	if err != nil {
		slog.Warn("coordinator loop: a broken task's port can't be read", "task", id, "err", err)
		return 0, false
	}
	var worker string
	var port int
	for _, e := range stream {
		switch e.Type {
		case events.WorktreeReady:
			worker, port = cmp.Or(e.Text("worker"), workers.Local), e.Number("port")
		case events.WorktreeMoved:
			worker, port = e.Text("worker"), e.Number("port")
		}
	}
	return port, worker != "" && worker == machine
}

func (l *Loop) load(id string) (*tasks.Task, agents.Roster, error) {
	task, err := l.state.Task(id)
	if err != nil {
		return nil, nil, err
	}
	if err := l.state.Broken(events.AgentsStream); err != nil {
		return nil, nil, err
	}
	return task, l.state.Roster(), nil
}

// propose reports whether the event was accepted. A refusal is already in
// the log as proposal.rejected; it is logged here too, for whoever runs the server.
func (l *Loop) propose(task string, actor events.Actor, kind events.Type, data map[string]any) bool {
	_, rejection, err := l.store.Propose(events.Proposal{Stream: task, Actor: actor, Type: kind, Data: data})
	if err != nil || rejection != nil {
		slog.Warn("coordinator loop: proposal refused", "task", task, "type", kind, "rejection", rejection, "err", err)
		return false
	}
	return true
}

// tell proposes an event made earlier, as work beside the loop ends with.
func (l *Loop) tell(p events.Proposal) bool { return l.propose(p.Stream, p.Actor, p.Type, p.Data) }
