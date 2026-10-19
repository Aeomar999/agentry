package coordinator

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/turns"
	"gitlab.com/dev.chempa/agentry/internal/workers"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

// Loop reads the log and, for every task an event touched, does the one move
// Next says. Its own proposals are events too, so each one wakes it again:
// the task moves step by step, and a restart picks up where it was.
type Loop struct {
	store  *store.Store
	runner *turns.Runner
	fleet  *machines.Fleet
	files  fileWork // shared files being moved (files.go)

	Clones string // where repos the user names by a git address are cloned, for a "move"
}

var agentry = events.Actor{Kind: events.System, ID: "coordinator-loop"}

// AsksForItself says whether the task's open question is the loop's own, one
// of its stops, rather than one a coordinator asked.
func AsksForItself(task *tasks.Task) bool { return task.Question != "" && task.Asked.By == agentry.ID }

// The limits that stop a task, as its question says.
const (
	LimitBudget = "budget"
	LimitTurns  = "turns"
)

// StoppedAt is the limit the task's open question is the loop's stop for,
// LimitBudget or LimitTurns, or "" for any other question: raising that
// limit answers it, and nothing else does.
func StoppedAt(task *tasks.Task) string {
	switch {
	case !AsksForItself(task):
		return ""
	case task.Asked.Limit != "":
		return task.Asked.Limit
	case task.Asked.Fetch != nil:
		return "" // the copy question
	}
	// Asked before the question said which: the stops' fixed words tell, and
	// the loop's other questions, which carry no limit, never match them.
	switch {
	case strings.HasPrefix(task.Question, oldTurnStop):
		return LimitTurns
	case strings.HasPrefix(task.Question, oldBudgetStop):
		return LimitBudget
	}
	return ""
}

// How the loop's stops at the limits began before they said their limit.
const (
	oldTurnStop   = "This task has had "
	oldBudgetStop = "This task has used "
)

func NewLoop(s *store.Store, runner *turns.Runner, fleet *machines.Fleet) *Loop {
	return &Loop{store: s, runner: runner, fleet: fleet}
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
		for _, id := range touchedTasks(batch) {
			l.act(id)
		}
	}
}

// touchedTasks are the tasks a batch changed, each once, in order.
func touchedTasks(batch []events.Event) []string {
	seen := map[string]bool{}
	var ids []string
	for _, e := range batch {
		if !events.IsTaskStream(e.Stream) || e.Type == events.ProposalRejected || seen[e.Stream] {
			continue
		}
		seen[e.Stream] = true
		ids = append(ids, e.Stream)
	}
	return ids
}

func (l *Loop) act(id string) {
	task, roster, err := l.load(id)
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
		l.beside(task.ID, func(ctx context.Context) { l.fetch(ctx, task, fetch) })
		return
	}

	switch move := Next(task, roster); move.Kind {
	case PrepareWorktree:
		l.prepareWorktree(task)
	case StartTurn:
		l.startTurn(task, roster, move)
	case ApplyDecision:
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
	shared, _ := l.filesFor(task)
	brief := workerBrief(task, move.Agent, move.Handoff, shared)
	if move.Agent.Coordinates {
		brief = coordinatorBrief(task, move.Agent, roster, l.familyOf(task), l.machines(), shared)
	}
	if err := l.runner.Start(task, move.Agent, brief); err != nil {
		slog.Error("coordinator loop: starting a turn", "task", task.ID, "agent", move.Agent.Name, "err", err)
	}
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
		l.proposeOrRaise(task.ID, coordinator, events.TaskAssigned, map[string]any{"agent": decision.Agent, "instructions": decision.Instructions,
			"machine": machine, "toYou": decision.ToYou, "checks": agent.Column == agents.Reviews})
	case Ask:
		l.propose(task.ID, coordinator, events.TaskNeedsYou, map[string]any{"question": decision.Question,
			"options": decision.Options, "page": decision.Page, "toYou": decision.ToYou})
	case Transfer:
		lead, _ := roster.Coordinator(teamNamed(roster, decision.Team))
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
		if l.propose(task.ID, coordinator, events.TaskDone, map[string]any{"summary": decision.Summary, "toYou": decision.ToYou}) {
			l.removeWorktree(task)
		}
	}
}

// moveRepo moves the task to the repo the user named for it: a folder here,
// or a git address or archive, cloned here first beside the loop, as that
// can take minutes. The decision waits until the move is told, so a restart
// clones again. If it can't be worked in, the coordinator hears why, and
// can ask the user.
func (l *Loop) moveRepo(task *tasks.Task, coordinator events.Actor, repo string) {
	if !worktrees.IsAddress(repo) {
		l.movedTo(task, coordinator, repo, repo, worktrees.IsRepo(repo))
		return
	}
	l.beside(task.ID, func(context.Context) {
		folder, err := worktrees.Clone(l.Clones, repo)
		reason := ""
		if err != nil {
			reason = sentence(err) + " Ask the user to give Agentry access to it, or to clone it here themselves."
		}
		l.movedTo(task, coordinator, repo, folder, reason)
	})
}

// movedTo tells the move to folder, the repo named as repo, or why it
// couldn't be made.
func (l *Loop) movedTo(task *tasks.Task, coordinator events.Actor, repo, folder, reason string) {
	if reason != "" {
		l.propose(task.ID, agentry, events.IssueRaised, map[string]any{"reason": "the task couldn't move to " + repo + ": " + reason})
		return
	}
	l.propose(task.ID, coordinator, events.RepoChanged, map[string]any{"repo": folder})
}

// request opens a task for another team, for what this one needs from it,
// in the same repo, and makes this one wait for it.
func (l *Loop) request(task *tasks.Task, roster agents.Roster, coordinator events.Actor, decision Decision) {
	lead, _ := roster.Coordinator(teamNamed(roster, decision.Team))
	asker, _ := roster.Get(task.Coordinator)
	description := decision.Request + "\n\n(Asked by the " + asker.TeamName() + " team's coordinator, " + task.Coordinator +
		", for " + task.ID + ": " + task.Title + ". When this is done, your summary goes back to them.)"
	child, refused := l.newTask(coordinator, map[string]any{
		"title": firstLine(decision.Request), "description": description, "repo": task.Repo, "coordinator": lead.Name,
		"teamChosen": true, "parent": task.ID, "depth": task.Depth + 1, "budget": task.Budget, "maxTurns": task.MaxTurns, "labels": listOf(task.Labels)})
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
	var ids, titles []string
	refused := ""
	for i, story := range decision.Stories {
		team := cmp.Or(teamNamed(roster, story.Team), own.TeamName())
		lead, ok := roster.Coordinator(team)
		if !ok {
			refused = fmt.Sprintf("story %d is for the %s team, which has no coordinator", i+1, team)
			break
		}
		after := []string{}
		for _, ref := range story.After {
			after = append(after, cmp.Or(ref.ID, ids[ref.Number-1]))
		}
		description := strings.TrimSpace(story.Brief) + "\n\n(A story of " + task.ID + ", the epic \"" + task.Title +
			"\". When it is done, your summary goes back to the epic's coordinator, " + task.Coordinator + ".)"
		var id string
		if id, refused = l.newTask(coordinator, map[string]any{
			"title": strings.TrimSpace(story.Title), "description": description, "repo": task.Repo, "coordinator": lead.Name,
			"teamChosen": true, "kindChosen": true, "epic": task.ID, "after": after, "parked": "backlog", "budget": task.Budget, "maxTurns": task.MaxTurns,
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

// newTask makes a task with the next free id, and returns the id, or why it
// wasn't made. Another task made at the same moment can take the id: then
// it tries the one after.
func (l *Loop) newTask(by events.Actor, data map[string]any) (string, string) {
	for attempt := 0; attempt < 5; attempt++ {
		ids, err := l.store.TaskIDs()
		if err != nil {
			return "", "Agentry couldn't list the tasks: " + err.Error()
		}
		id := fmt.Sprintf("task-%d", len(ids)+1)
		_, rejection, err := l.store.Propose(events.Proposal{Stream: id, Actor: by, Type: events.TaskCreated, Data: data})
		switch {
		case err != nil:
			return "", "Agentry couldn't write it: " + err.Error()
		case rejection != nil && strings.Contains(rejection.Reason, "already exists"):
			continue // taken a moment ago: the next id
		case rejection != nil:
			return "", rejection.Reason
		}
		return id, ""
	}
	return "", "no task id was free"
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
// would start from a dev without their work.
func (l *Loop) blocked(story *tasks.Task) bool {
	for _, id := range story.After {
		if before, _, err := l.load(id); err != nil || before == nil || !before.IsDone {
			return true
		}
	}
	return false
}

// answer gives a finished task's summary to the task that asked for it, if
// that one still waits for it.
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
// then works in the same worktree.
func (l *Loop) portsInUse(machine string) []int {
	ids, _ := l.store.TaskIDs()
	var ports []int
	for _, id := range ids {
		task, _, err := l.load(id)
		if err == nil && task != nil && (!task.IsDone || task.WrapUp != nil) && task.Worktree != nil && task.Worktree.Worker == machine {
			ports = append(ports, task.Worktree.Port)
		}
	}
	return ports
}

// proposeOrRaise writes the coordinator's event, or, if the rules refuse
// it, raises their reason as an issue: the coordinator hears it and decides again.
func (l *Loop) proposeOrRaise(task string, actor events.Actor, kind events.Type, data map[string]any) {
	_, rejection, err := l.store.Propose(events.Proposal{Stream: task, Actor: actor, Type: kind, Data: data})
	switch {
	case err != nil:
		slog.Warn("coordinator loop: proposal refused", "task", task, "type", kind, "err", err)
	case rejection != nil:
		l.propose(task, agentry, events.IssueRaised, map[string]any{"reason": rejection.Reason})
	}
}

func (l *Loop) load(id string) (*tasks.Task, agents.Roster, error) {
	stream, err := l.store.Stream(id)
	if err != nil {
		return nil, nil, err
	}
	agentEvents, err := l.store.Stream(events.AgentsStream)
	if err != nil {
		return nil, nil, err
	}
	return tasks.FromEvents(stream), agents.FromEvents(agentEvents), nil
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
