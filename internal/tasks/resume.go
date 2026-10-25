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
	worker = cmp.Or(worker, workers.Local)
	last := t.LastTurnOf(agent)
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
