package watchdog

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

var (
	you      = events.Actor{Kind: events.Human, ID: "you"}
	worker   = events.Actor{Kind: events.Worker, ID: "local"}
	policy   = events.Actor{Kind: events.Policy, ID: "rules"}
	engineer = events.Actor{Kind: events.Agent, ID: "engineer"}
)

// world is a store with task-1 in an open coordinator turn, and a fake
// process table: alive says whether the agent's process is running.
type world struct {
	t       *testing.T
	store   *store.Store
	alive   bool
	stopped bool
	dog     *Watchdog
}

func newWorld(t *testing.T) *world {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	w := &world{t: t, store: s, alive: true}
	w.dog = New(s, state, DefaultLimits, func(string) bool { return w.alive }, func(string) { w.stopped = true })
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	w.propose(you, events.TaskCreated, "", map[string]any{"title": "t", "repo": "/r", "coordinator": "tech-lead"})
	w.propose(worker, events.WorktreeReady, "", map[string]any{"path": "/w", "branch": "b", "port": 7901})
	w.propose(worker, events.TurnStarted, "turn-1", map[string]any{"agent": "tech-lead", "hash": "h"})
	w.propose(worker, events.PromptAcknowledged, "turn-1", map[string]any{"hash": "h"})
	return w
}

func (w *world) propose(actor events.Actor, kind events.Type, correlation string, data map[string]any) {
	w.t.Helper()
	if _, rejection, err := w.store.Propose(events.Proposal{Stream: "task-1", Actor: actor, Type: kind, Correlation: correlation, Data: data}); rejection != nil || err != nil {
		w.t.Fatal(kind, rejection, err)
	}
}

func (w *world) task() *tasks.Task {
	stream, _ := w.store.Stream("task-1")
	return tasks.FromEvents(stream)
}

// after runs the watchdog as if d had passed, and returns the issue it raised, or "".
func (w *world) after(d time.Duration) string {
	w.dog.Check(time.Now().Add(d))
	if turn := w.task().LastTurn(); turn.Issue != "" {
		return turn.Issue
	}
	return ""
}

func TestAHealthyTurnIsLeftAlone(t *testing.T) {
	w := newWorld(t)
	if issue := w.after(5 * time.Minute); issue != "" || w.stopped {
		t.Errorf("issue %q, stopped %v", issue, w.stopped)
	}
}

// Proof 03: a crash fires no hook; only the dead pane shows it.
func TestACrashIsFlaggedAfterTheGrace(t *testing.T) {
	w := newWorld(t)
	w.alive = false
	if issue := w.after(time.Second); issue != "" {
		t.Errorf("within the start grace the process may not be up yet: %q", issue)
	}
	if issue := w.after(4 * time.Second); !strings.Contains(issue, "process exited") || !w.stopped {
		t.Errorf("issue %q, stopped %v", issue, w.stopped)
	}
	if w.task().OpenTurn() != nil {
		t.Error("the issue should end the turn, so the coordinator gets it")
	}
}

func TestASilentTurnIsAStall(t *testing.T) {
	w := newWorld(t)
	if issue := w.after(11 * time.Minute); !strings.Contains(issue, "silent") {
		t.Errorf("issue %q", issue)
	}
}

func TestWaitingForYouIsNotAStall(t *testing.T) {
	w := newWorld(t)
	w.propose(worker, events.TurnEnded, "turn-1", map[string]any{"agent": "tech-lead", "report": "r"})
	w.propose(events.Actor{Kind: events.Agent, ID: "tech-lead"}, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"})
	w.propose(worker, events.TurnStarted, "turn-2", map[string]any{"agent": "engineer", "hash": "h2"})
	w.propose(worker, events.PromptAcknowledged, "turn-2", map[string]any{"hash": "h2"})
	w.propose(engineer, events.ActionRequested, "toolu_1", map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"})
	w.propose(policy, events.ActionAsked, "toolu_1", map[string]any{"reason": "every push"})
	if issue := w.after(time.Hour); issue != "" {
		t.Errorf("an hour waiting for your approval is not a stall: %q", issue)
	}
}

func TestAToolOpenTooLong(t *testing.T) {
	w := newWorld(t)
	coordinator := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	w.propose(coordinator, events.ActionRequested, "toolu_1", map[string]any{"tool": "Bash", "command": "npm test"})
	w.propose(policy, events.ActionApproved, "toolu_1", map[string]any{"reason": "ok"})
	if issue := w.after(15 * time.Minute); issue != "" {
		t.Errorf("a running tool isn't silence, and 15 minutes is within its limit: %q", issue)
	}
	if issue := w.after(21 * time.Minute); !strings.Contains(issue, "npm test") {
		t.Errorf("issue %q", issue)
	}
}

func TestTheSameCommandRepeated(t *testing.T) {
	w := newWorld(t)
	coordinator := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	for i := range DefaultLimits.Repeats {
		id := fmt.Sprintf("toolu_%d", i)
		w.propose(coordinator, events.ActionRequested, id, map[string]any{"tool": "Bash", "command": "go test ./..."})
		w.propose(policy, events.ActionApproved, id, map[string]any{"reason": "ok"})
		w.propose(worker, events.ActionResult, id, nil)
	}
	if issue := w.after(time.Second); !strings.Contains(issue, "5 times in a row") {
		t.Errorf("issue %q", issue)
	}
}

func TestAPromptThatNeverArrivesWaitsForYou(t *testing.T) {
	w := newWorld(t)
	w.propose(worker, events.TurnEnded, "turn-1", map[string]any{"agent": "tech-lead", "report": "r"})
	w.propose(events.Actor{Kind: events.Agent, ID: "tech-lead"}, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"})
	w.propose(worker, events.TurnStarted, "turn-2", map[string]any{"agent": "engineer", "hash": "h2"})

	w.after(61 * time.Second)
	if task := w.task(); task.Status() != tasks.NeedsYou || !strings.Contains(task.OpenTurn().Waiting, "trust") {
		t.Errorf("status %s, waiting %q", task.Status(), task.OpenTurn().Waiting)
	}
	w.propose(worker, events.PromptAcknowledged, "turn-2", map[string]any{"hash": "h2"})
	if task := w.task(); task.Status() != tasks.Working {
		t.Errorf("once the prompt arrives the task is working again, got %s", task.Status())
	}
}

// limit stops turn-1 at a usage limit that resets at resetsAt ("" when the
// engine didn't say).
func (w *world) limit(resetsAt string) {
	w.t.Helper()
	w.propose(worker, events.TurnLimited, "turn-1", map[string]any{"agent": "tech-lead", "error": "rate_limit", "details": "limit reached", "resetsAt": resetsAt})
}

func TestALimitedTurnIsQuietUntilTheResetPlusSilence(t *testing.T) {
	w := newWorld(t)
	w.limit(time.Now().Add(30 * time.Minute).UTC().Format(time.RFC3339))
	if issue := w.after(39 * time.Minute); issue != "" || w.stopped {
		t.Errorf("quiet before the reset plus the silence limit is waiting, not a stall: %q", issue)
	}
	if issue := w.after(41 * time.Minute); !strings.Contains(issue, "usage limit") || !w.stopped {
		t.Errorf("a turn that never goes on after the reset is a stall: issue %q, stopped %v", issue, w.stopped)
	}
}

func TestALimitWithNoResetTimeWaitsAnHour(t *testing.T) {
	w := newWorld(t)
	w.limit("")
	if issue := w.after(UnknownReset + 9*time.Minute); issue != "" {
		t.Errorf("issue %q", issue)
	}
	if issue := w.after(UnknownReset + 11*time.Minute); !strings.Contains(issue, "usage limit") {
		t.Errorf("issue %q", issue)
	}
}

func TestALimitResetDaysAwayIsWaitedOutForADayAtMost(t *testing.T) {
	w := newWorld(t)
	w.limit(time.Now().Add(3 * 24 * time.Hour).UTC().Format(time.RFC3339))
	if issue := w.after(MaxLimitWait + 9*time.Minute); issue != "" {
		t.Errorf("issue %q", issue)
	}
	if issue := w.after(MaxLimitWait + 11*time.Minute); !strings.Contains(issue, "usage limit") {
		t.Errorf("a reset days away still stalls after a day plus the silence limit: issue %q", issue)
	}
}

func TestALimitedTurnWhoseProcessExitedIsACrash(t *testing.T) {
	w := newWorld(t)
	w.limit(time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	w.alive = false
	if issue := w.after(4 * time.Second); !strings.Contains(issue, "process exited") {
		t.Errorf("issue %q", issue)
	}
}

func TestALimitedTurnIsNeverAskedAboutItsPrompt(t *testing.T) {
	w := newWorld(t)
	w.propose(worker, events.TurnEnded, "turn-1", map[string]any{"agent": "tech-lead", "report": "r"})
	w.propose(events.Actor{Kind: events.Agent, ID: "tech-lead"}, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "go"})
	w.propose(worker, events.TurnStarted, "turn-2", map[string]any{"agent": "engineer", "hash": "h2"})
	w.propose(worker, events.TurnLimited, "turn-2", map[string]any{"agent": "engineer", "error": "rate_limit", "details": "", "resetsAt": ""})

	w.after(2 * time.Minute)
	if waiting := w.task().OpenTurn().Waiting; waiting != "" {
		t.Errorf("a limited engine echoes no prompt until the reset, so nobody should be asked: %q", waiting)
	}
}

func TestALimitedTurnSkipsTheRepeatCheck(t *testing.T) {
	w := newWorld(t)
	coordinator := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	for i := range DefaultLimits.Repeats {
		id := fmt.Sprintf("toolu_%d", i)
		w.propose(coordinator, events.ActionRequested, id, map[string]any{"tool": "Bash", "command": "go test ./..."})
		w.propose(policy, events.ActionApproved, id, map[string]any{"reason": "ok"})
		w.propose(worker, events.ActionResult, id, nil)
	}
	w.limit("")
	if issue := w.after(time.Second); issue != "" {
		t.Errorf("issue %q", issue)
	}
}

// resume says turn-1's engine went on after its limit.
func (w *world) resume() {
	w.t.Helper()
	w.propose(worker, events.TurnResumed, "turn-1", map[string]any{"agent": "tech-lead", "reason": "engine"})
}

func TestAResumedTurnIsWatchedForSilenceAgain(t *testing.T) {
	w := newWorld(t)
	w.limit(time.Now().Add(time.Hour).UTC().Format(time.RFC3339))
	w.resume()
	if issue := w.after(11 * time.Minute); !strings.Contains(issue, "stalled") {
		t.Errorf("a resumed turn quiet for over the silence limit is a stall, not waiting for the reset: %q", issue)
	}
}

func TestAResumedTurnCountsRepeatsAgain(t *testing.T) {
	w := newWorld(t)
	coordinator := events.Actor{Kind: events.Agent, ID: "tech-lead"}
	for i := range DefaultLimits.Repeats {
		id := fmt.Sprintf("toolu_%d", i)
		w.propose(coordinator, events.ActionRequested, id, map[string]any{"tool": "Bash", "command": "go test ./..."})
		w.propose(policy, events.ActionApproved, id, map[string]any{"reason": "ok"})
		w.propose(worker, events.ActionResult, id, nil)
	}
	w.limit("")
	w.resume()
	if issue := w.after(time.Second); !strings.Contains(issue, "5 times in a row") {
		t.Errorf("issue %q", issue)
	}
}
