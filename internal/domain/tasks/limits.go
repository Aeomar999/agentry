package tasks

// Limits are what a task may use before it asks you: Budget tokens, and
// MaxTurns turns without your answer. 0 is no limit for either.
type Limits struct {
	Budget   int
	MaxTurns int
}

// Limits are the task's own limits.
func (t *Task) Limits() Limits { return Limits{Budget: t.Budget, MaxTurns: t.MaxTurns} }

// NewLimits is the one rule for a new task's limits: a task that comes from
// another (a request it asks of another team, a story of its epic, an idea
// that follows up an epic) takes that task's limits; any other takes the
// server's defaults. Every task.created writes both, so the old-log default
// of turns (DefaultMaxTurns) never applies to a new task.
func NewLimits(defaults Limits, from *Task) Limits {
	if from != nil {
		return from.Limits()
	}
	return defaults
}
