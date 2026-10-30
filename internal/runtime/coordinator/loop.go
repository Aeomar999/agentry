package coordinator

import (
	"context"
	"errors"
	"log/slog"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
	"gitlab.com/dev.chempa/agentry/internal/runtime/turns"
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
