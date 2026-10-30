package coordinator

import (
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// Slots are the turns the board may run at once, settings.MaxRunningTasks,
// across every repo. A task on the board in progress takes a slot while it
// has a turn open. One between turns that wants its next turn waits its
// turn, in your priority order, until a slot is free: then it is admitted,
// and holds that slot until its turn is recorded, so a turn being started
// counts before it is in the log. A wrap-up turn, after you move a task off
// the board, is never held back and never counts.
//
// The loop holds back a queued task, and the page shows it as queued, both
// from SlotsOf, so they always agree.
type Slots struct {
	Limit   int // 0 is no limit
	Running int // tasks with a turn open
	queued  map[string]bool
}

// SlotsOf is how the slots are taken, given every task.
func SlotsOf(all []*tasks.Task, roster agents.Roster, limit int) Slots {
	slots := Slots{Limit: limit}
	var waiting []*tasks.Task
	for _, task := range all {
		switch {
		case !onTheBoard(task):
		case task.OpenTurn() != nil:
			slots.Running++
		case wantsATurn(task, roster):
			waiting = append(waiting, task)
		}
	}
	if limit == 0 {
		return slots
	}
	slices.SortStableFunc(waiting, tasks.InPriorityOrder)
	free := max(limit-slots.Running, 0)
	slots.queued = map[string]bool{}
	for _, task := range waiting[min(free, len(waiting)):] {
		slots.queued[task.ID] = true
	}
	return slots
}

// Queued says whether the task wants a turn and waits for a slot.
func (s Slots) Queued(id string) bool { return s.queued[id] }

// onTheBoard is a task the limit is for: in progress, not one you moved to
// Done, To do or the Backlog.
func onTheBoard(task *tasks.Task) bool {
	return !task.IsDone && task.Parked == "" && task.WrapUp == nil
}

// wantsATurn is a task between turns whose next move is a turn, as the
// loop would make it, or its coordinator's decision, which most often leads
// to one: it keeps its place however far behind the log the loop is. One
// fetching a file starts nothing until it has it.
func wantsATurn(task *tasks.Task, roster agents.Roster) bool {
	switch Next(task, roster).Kind {
	case StartTurn, ApplyDecision:
		return task.Fetching == nil
	}
	return false
}

// held is the tasks the loop held back for a slot, to act on again when one
// may be free. Only the loop's own goroutine reads or changes it.
type held map[string]bool

// holdBack says whether the task waits for a slot, and if so keeps it to
// wake. Holding writes nothing: the task just waits for its next pass. act
// lets go of it first, so a task whose move is no longer a turn is never
// woken for one.
func (l *Loop) holdBack(task *tasks.Task) bool {
	if l.state.Settings().MaxRunningTasks == 0 || !l.slots().Queued(task.ID) {
		return false
	}
	if l.held == nil {
		l.held = held{}
	}
	l.held[task.ID] = true
	return true
}

// wakeHeld acts on the held tasks a slot is free for now, in your priority
// order. Each is held again, or starts, as act gates it once more.
func (l *Loop) wakeHeld() {
	if len(l.held) == 0 {
		return
	}
	slots := l.slots()
	listed := map[string]bool{}
	var ready []*tasks.Task
	for _, task := range l.state.Tasks() {
		listed[task.ID] = true
		if l.held[task.ID] && !slots.Queued(task.ID) {
			ready = append(ready, task)
		}
	}
	for id := range l.held {
		if !listed[id] { // archived or broken: it takes no turn
			delete(l.held, id)
		}
	}
	slices.SortStableFunc(ready, tasks.InPriorityOrder)
	for _, task := range ready {
		l.act(task.ID)
	}
}

// slots are the board's slots as of the log's last event.
func (l *Loop) slots() Slots {
	return SlotsOf(l.state.Tasks(), l.state.Roster(), l.state.Settings().MaxRunningTasks)
}
