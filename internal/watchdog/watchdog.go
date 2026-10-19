// Package watchdog catches agents that go quiet. Hooks say what an agent
// does, but a crash fires no hook, and a hung network call or a stuck tool
// fires nothing at all; the screen keeps animating, so "screen unchanged"
// detects nothing either (proof 03). So it checks each open turn on a clock,
// and every problem it finds is an issue, which goes back to the coordinator.
package watchdog

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// Limits are how long each kind of quiet may last.
type Limits struct {
	StartGrace  time.Duration // a new session's process may not be up yet
	Acknowledge time.Duration // until the engine echoes the prompt
	Silence     time.Duration // a turn with no events at all
	Tool        time.Duration // one approved tool call without a result
	Repeats     int           // the same command requested this many times in a row
}

var DefaultLimits = Limits{
	StartGrace:  3 * time.Second,
	Acknowledge: 60 * time.Second,
	Silence:     10 * time.Minute,
	Tool:        20 * time.Minute,
	Repeats:     5,
}

// UnknownReset is how long a usage limit is taken to last when the engine
// didn't say when it resets.
const UnknownReset = time.Hour

// MaxLimitWait is the longest a usage limit is waited out, however far off
// the engine says it resets, so a misread reset can't hide a stall for days.
const MaxLimitWait = 24 * time.Hour

// ResetSlack is how long after the engine's reset time a limited turn is
// retried, so a reset read to the minute has surely passed.
const ResetSlack = time.Minute

// CarryOn is typed into a limited turn's session when its limit resets.
const CarryOn = "Your Claude usage limit has reset. Carry on from where you stopped."

type Watchdog struct {
	store     *store.Store   // where it raises issues
	state     *current.State // the tasks it watches
	limits    Limits
	isRunning func(task string) bool        // is the task's agent process alive
	stop      func(task string)             // end the task's session
	typeLine  func(task, line string) error // type a line into the task's session and press Enter
	typing    sync.WaitGroup                // carry-on prompts still being typed
}

var dog = events.Actor{Kind: events.System, ID: "watchdog"}

func New(s *store.Store, state *current.State, limits Limits, isRunning func(task string) bool, stop func(task string),
	typeLine func(task, line string) error) *Watchdog {
	return &Watchdog{store: s, state: state, limits: limits, isRunning: isRunning, stop: stop, typeLine: typeLine}
}

// Run checks every second until ctx ends: a crash is flagged within about
// StartGrace plus a second. It returns once no prompt is being typed.
func (w *Watchdog) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	defer w.Wait()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			w.Check(now)
		}
	}
}

// Check looks at every task with an open turn, once: a finished task's too
// while its coordinator wraps up, so a wrap-up that crashes still ends.
func (w *Watchdog) Check(now time.Time) {
	for _, task := range w.state.Tasks() {
		if task.IsDone && task.WrapUp == nil || task.OpenTurn() == nil {
			continue
		}
		w.checkTurn(now, task, task.OpenTurn())
	}
}

func (w *Watchdog) checkTurn(now time.Time, task *tasks.Task, turn *tasks.Turn) {
	if problem := w.findProblem(now, task, turn); problem != "" {
		w.raise(task.ID, problem)
		return
	}
	if turn.Limited != "" {
		w.checkLimited(now, task, turn)
		return
	}
	if !turn.Acknowledged && turn.Waiting == "" && now.Sub(turn.StartedAt) > w.limits.Acknowledge {
		w.propose(task.ID, events.AgentWaiting, turn.ID, map[string]any{"question": fmt.Sprintf(
			"%s hasn't received its prompt after %s. It may be showing a question on screen, such as whether to trust "+
				"this repository: open its terminal and answer it.", turn.Agent, w.limits.Acknowledge)})
	}
}

// findProblem returns what is wrong with an open turn, or "".
func (w *Watchdog) findProblem(now time.Time, task *tasks.Task, turn *tasks.Turn) string {
	if now.Sub(turn.StartedAt) > w.limits.StartGrace && !w.isRunning(task.ID) {
		return fmt.Sprintf("%s's process exited in %s (crashed or was killed)", turn.Agent, turn.ID)
	}
	if turn.Limited != "" {
		return "" // checkLimited watches it
	}
	if command, times := repeatedCommand(task, turn); times >= w.limits.Repeats {
		return fmt.Sprintf("%s requested the same command %d times in a row, so it may be stuck in a loop: %s", turn.Agent, times, command)
	}
	if action := runningTool(task, turn); action != nil {
		if now.Sub(action.RequestedAt) > w.limits.Tool {
			return fmt.Sprintf("%s's tool call has run for over %s with no result: %s", turn.Agent, w.limits.Tool, action.Command)
		}
		return ""
	}
	isWaitingForYou := len(task.HooksWaiting()) > 0 || turn.Waiting != "" // a held action waits for you, the agent doesn't
	if !isWaitingForYou && now.Sub(turn.LastEventAt) > w.limits.Silence {
		return fmt.Sprintf("%s has been silent for over %s (stalled: a hung network call or a stuck tool)", turn.Agent, w.limits.Silence)
	}
	return ""
}

// checkLimited retries a turn stopped at a usage limit once it has reset:
// the turn goes on, then its session is told to carry on. A resume that is
// refused leaves the turn waiting, and silence after the reset is a stall.
// The prompt is typed off the loop, since typing into a remote worker's
// session can take half a minute; the accepted resume keeps it to one try.
func (w *Watchdog) checkLimited(now time.Time, task *tasks.Task, turn *tasks.Turn) {
	at := retryAt(turn)
	if now.Before(at) {
		return
	}
	if !w.propose(task.ID, events.TurnResumed, turn.ID, map[string]any{"agent": turn.Agent, "reason": "reset"}) {
		if now.Sub(at) > w.limits.Silence {
			w.raise(task.ID, fmt.Sprintf("%s has been silent for over %s since its usage limit was due to reset", turn.Agent, w.limits.Silence))
		}
		return
	}
	w.typing.Go(func() { w.carryOn(task.ID, turn.ID) })
}

// carryOn types the carry-on prompt into a resumed turn's session. By the
// time it runs, or once typing has failed, the turn may have ended and a
// newer one started, which the prompt and its issue must leave alone.
func (w *Watchdog) carryOn(task, turn string) {
	if !w.isOpen(task, turn) {
		slog.Info("watchdog: not typing the carry-on prompt, the turn has ended", "task", task, "turn", turn)
		return
	}
	err := w.typeLine(task, CarryOn)
	if err == nil {
		return
	}
	if !w.isOpen(task, turn) {
		slog.Info("watchdog: could not type the carry-on prompt, but the turn has ended", "task", task, "turn", turn, "err", err)
		return
	}
	w.raise(task, "could not type the carry-on prompt after the usage limit: "+err.Error())
}

// isOpen says whether turn is still the task's open turn.
func (w *Watchdog) isOpen(task, turn string) bool {
	t, err := w.state.Task(task)
	return err == nil && t != nil && t.OpenTurn() != nil && t.OpenTurn().ID == turn
}

// Wait returns once every carry-on prompt being typed is typed.
func (w *Watchdog) Wait() { w.typing.Wait() }

// retryAt is when a limited turn is retried: just after the engine said its
// limit resets, or UnknownReset after it stopped when it didn't say, and
// never later than MaxLimitWait after it stopped, so a misread reset can't
// hold the turn for days.
func retryAt(turn *tasks.Turn) time.Time {
	at := turn.ResetsAt.Add(ResetSlack)
	if turn.ResetsAt.IsZero() {
		at = turn.LastEventAt.Add(UnknownReset)
	}
	if latest := turn.LastEventAt.Add(MaxLimitWait); at.After(latest) {
		return latest
	}
	return at
}

// runningTool is the turn's approved action that has no result yet, if any.
func runningTool(task *tasks.Task, turn *tasks.Turn) *tasks.Action {
	for _, action := range task.Actions {
		if action.Turn == turn.ID && action.Decision == tasks.Approved && !action.HasResult {
			return action
		}
	}
	return nil
}

// repeatedCommand is the turn's last command and how many times in a row it
// was requested. It compares the tool and the command itself, never its
// description (proof 03). A call whose command Agentry can't read says
// nothing about a loop, so it is never counted as a repeat.
func repeatedCommand(task *tasks.Task, turn *tasks.Turn) (string, int) {
	var last string
	times := 0
	for _, action := range task.Actions {
		if action.Turn != turn.ID {
			continue
		}
		call := action.Tool + ": " + action.Command
		switch {
		case action.Command == "":
			last, times = "", 0
		case call == last:
			times++
		default:
			last, times = call, 1
		}
	}
	return last, times
}

// raise ends the task's session and its turn as an issue.
func (w *Watchdog) raise(task, reason string) {
	w.stop(task)
	w.propose(task, events.IssueRaised, "", map[string]any{"reason": reason})
}

// propose says whether the event was recorded.
func (w *Watchdog) propose(task string, kind events.Type, correlation string, data map[string]any) bool {
	if _, rejection, err := w.store.Propose(events.Proposal{Stream: task, Actor: dog, Type: kind, Correlation: correlation, Data: data}); err != nil || rejection != nil {
		slog.Warn("watchdog: proposal refused", "task", task, "type", kind, "rejection", rejection, "err", err)
		return false
	}
	return true
}
