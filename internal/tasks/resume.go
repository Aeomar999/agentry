package tasks

import (
	"cmp"
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// MaxResumedContext is the most context, in tokens, a session may carry
// into a resumed turn: a 200k window less room for the turn's own work.
// It is measured by the input of the last turn's last call (Turn.Context),
// not its split, which sums every call of the turn.
const MaxResumedContext = 150_000

// SessionToResume is the engine session the agent's next turn resumes: the
// one of its latest turn on this task, when that turn ended on the same
// engine and machine, and reported a session whose context isn't over
// MaxResumedContext; a context the engine didn't report still resumes.
// A turn that resumed and failed before it took its prompt couldn't
// resume, so the next starts fresh, and only once: it resumes nothing.
// Otherwise it is "", with a short plain reason for starting fresh.
// The machine is a worker's name, or "" or workers.Local for this one.
func (t *Task) SessionToResume(agent, engine, worker string) (session, fresh string) {
	return resumableFrom(t.LastTurnOf(agent), engine, worker)
}

// SessionToShare is the engine session the agent's next turn on the task
// goes on with, and the task whose turn it was: its own latest turn on the
// task, by SessionToResume, or on its first turn here, its latest ended
// turn on any other of stories, the epic's stories, by the same rules. A
// sibling's open turn is passed over: it has no session to go on with yet,
// and the one it started from is busy, so it is forked. A story's own
// session wins, even when a sibling's turn is newer. With no session, from
// is nil and fresh says why.
func (t *Task) SessionToShare(stories []*Task, agent, engine, worker string) (session string, from *Task, fresh string) {
	from, last := t, t.LastTurnOf(agent)
	if last == nil {
		from, last = latestEndedTurnOf(agent, stories, t.ID)
	}
	if session, fresh = resumableFrom(last, engine, worker); session == "" {
		return "", nil, fresh
	}
	return session, from, ""
}

// latestEndedTurnOf is the agent's latest ended turn on any of the tasks
// but the one with id except, and the task it is on; nil, nil when it has
// none.
func latestEndedTurnOf(agent string, among []*Task, except string) (on *Task, latest *Turn) {
	for _, task := range among {
		if task == nil || task.ID == except {
			continue
		}
		for _, turn := range task.Turns {
			if turn.Agent == agent && turn.Ended && (latest == nil || turn.StartedAt.After(latest.StartedAt)) {
				on, latest = task, turn
			}
		}
	}
	return on, latest
}

// SessionBusy says whether a turn is open in the engine session, or one
// that started from it, on any of the tasks. Two turns going on with one
// session at once would both write it, and one would be lost, so a busy
// session is forked.
func SessionBusy(session string, among []*Task) bool {
	for _, task := range among {
		if open := task.OpenTurn(); open != nil && (open.EngineSession == session || open.FromSession == session) {
			return true
		}
	}
	return false
}

// resumableFrom is the session to go on with from the agent's last turn, or
// "" and why not.
func resumableFrom(last *Turn, engine, worker string) (session, fresh string) {
	worker = cmp.Or(worker, workers.Local)
	switch {
	case last == nil:
		return "", "first turn"
	case !last.Ended:
		return "", "its last turn hasn't ended"
	case last.Issue != "" && last.FromSession != "" && !last.Acknowledged:
		return "", "its resume failed"
	case last.Issue != "":
		return "", "its last turn failed"
	case last.Stopped:
		return "", "its last turn was stopped"
	case last.Worker != worker:
		return "", "the work moved to " + worker
	case last.Engine == "":
		return "", "engine unknown"
	case last.Engine != engine:
		return "", "another engine"
	case last.EngineSession == "":
		return "", "no session id"
	}
	// A zero context is unknown, not small; it still resumes.
	if last.Context > MaxResumedContext {
		return "", fmt.Sprintf("context too big (%dk tokens)", (last.Context+999)/1000)
	}
	return last.EngineSession, ""
}

// LastTurnOf is the agent's latest turn on the task, nil before its first.
func (t *Task) LastTurnOf(agent string) *Turn {
	for i := len(t.Turns) - 1; i >= 0; i-- {
		if t.Turns[i].Agent == agent {
			return t.Turns[i]
		}
	}
	return nil
}
