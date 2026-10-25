package tasks

import "fmt"

// MaxResumedContext is the most context, in tokens, a session may carry
// into a resumed turn: a 200k window less room for the turn's own work.
const MaxResumedContext = 150_000

// SessionToResume is the engine session the agent's next turn resumes: the
// one of its latest turn on this task, when that turn ended on the same
// engine and machine and reported a session small enough to carry on.
// Otherwise it is "", with a short plain reason for starting fresh.
func (t *Task) SessionToResume(agent, engine, worker string) (session, fresh string) {
	last := t.lastTurnOf(agent)
	switch {
	case last == nil:
		return "", "first turn"
	case !last.Ended:
		return "", "its last turn hasn't ended"
	case last.Worker != worker:
		return "", "the work moved to " + worker
	case last.Engine == "":
		return "", "engine unknown"
	case last.Engine != engine:
		return "", "another engine"
	case last.EngineSession == "":
		return "", "no session id"
	}
	// A zero split is unknown, not small; it still resumes.
	if size := last.Split.CacheRead + last.Split.Input; size > MaxResumedContext {
		return "", fmt.Sprintf("context too big (%dk tokens)", size/1000)
	}
	return last.EngineSession, ""
}

func (t *Task) lastTurnOf(agent string) *Turn {
	for i := len(t.Turns) - 1; i >= 0; i-- {
		if t.Turns[i].Agent == agent {
			return t.Turns[i]
		}
	}
	return nil
}
