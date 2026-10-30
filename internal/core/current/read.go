package current

import (
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/connections"
	"gitlab.com/dev.chempa/agentry/internal/domain/settings"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
)

// Resumed is the position of the snapshot Open went on from, or 0 when it
// replayed the whole log.
func (s *State) Resumed() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.resumed
}

// Position is the last event applied, rejections included.
func (s *State) Position() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.position
}

// StreamPosition is the position of the last accepted event applied to a
// stream, 0 for a stream with none.
func (s *State) StreamPosition(stream string) int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.streams[stream]
}

// Task is a task's state: nil, nil when there is no such task, ErrArchived
// when it is archived, ErrErased when it is erased, and the error when its
// stream is broken.
func (s *State) Task(id string) (*tasks.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if _, erased := s.erased[id]; erased {
		return nil, ErrErased
	}
	if _, archived := s.archived[id]; archived {
		return nil, ErrArchived
	}
	if err := s.broken[id]; err != nil {
		return nil, err
	}
	return s.tasks[id], nil
}

// Owned is an extension's stream's state, as its Streams left it: nil, nil
// when the stream holds nothing, and the error when it is broken. The
// extension's own typed read (ideas.Get) says what it is.
func (s *State) Owned(stream string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.broken[stream]; err != nil {
		return nil, err
	}
	return s.owned[stream], nil
}

// Erased is an erased task's erasure, and false for a task that isn't
// erased.
func (s *State) Erased(id string) (tasks.Erasure, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	erasure, erased := s.erased[id]
	return erasure, erased
}

// Tasks is every task, oldest first, in a new slice the caller may sort.
// A broken, archived or erased task is left out, as the list skips a task it can't
// read.
func (s *State) Tasks() []*tasks.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make([]*tasks.Task, 0, len(s.created))
	for _, c := range s.created {
		if task := s.tasks[c.id]; task != nil && s.broken[c.id] == nil {
			all = append(all, task)
		}
	}
	return all
}

// TaskIDs is the stream of every task.created applied, oldest first, in a
// new slice: a broken, archived or erased task's too, so its length counts
// every task made.
func (s *State) TaskIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	ids := make([]string, len(s.created))
	for i, c := range s.created {
		ids[i] = c.id
	}
	return ids
}

// NextTask is the stream the next task.created goes into: the one
// Store.Create("task-", ...) names, as both count every task.created, an
// archived or erased task's through its marker (see Store.made).
func (s *State) NextTask() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return events.Numbered(events.TaskPrefix, s.tasksMade)
}

// OwnedIDs is every stream with prefix that an extension here owns, in the
// order of their first events in the log, in a new slice: a broken one's
// too, as TaskIDs lists a broken task.
func (s *State) OwnedIDs(prefix string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	order := s.ownedOrder[prefix]
	ids := make([]string, len(order))
	for i, c := range order {
		ids[i] = c.id
	}
	return ids
}

// Summaries is every task as the list shows it, oldest first, in a new
// slice the caller may sort: a whole task's summary, as tasks.SummaryOf
// makes it with the roster, and an archived one's, as its marker keeps it.
// A broken or erased task is left out, as Tasks leaves it out: an erased
// one has no summary, only its Erased. BlockedBy and Queued
// need the other tasks, so they are the caller's to fill.
func (s *State) Summaries() []tasks.Summary {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make([]tasks.Summary, 0, len(s.created))
	for _, c := range s.created {
		if summary, archived := s.archived[c.id]; archived {
			all = append(all, summary)
		} else if task := s.tasks[c.id]; task != nil && s.broken[c.id] == nil {
			all = append(all, tasks.SummaryOf(task, s.roster))
		}
	}
	return all
}

// Summary is a task's summary as Summaries has it: a whole task's, made
// with the roster, or an archived one's, as its marker keeps it. It is
// false for a broken or erased task and for no such task.
func (s *State) Summary(id string) (tasks.Summary, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if summary, archived := s.archived[id]; archived {
		return summary, true
	}
	if task := s.tasks[id]; task != nil && s.broken[id] == nil {
		return tasks.SummaryOf(task, s.roster), true
	}
	return tasks.Summary{}, false
}

// Roster is every agent. If the agents stream is broken, it is the roster
// as it was before the event that broke it, or empty if it broke while
// loading, and Broken says why.
func (s *State) Roster() agents.Roster {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.roster
}

// Workers is every worker; a broken stream is kept as Roster's is.
func (s *State) Workers() workers.Roster {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.workers
}

// Connections is every connection; a broken stream is kept as Roster's is.
func (s *State) Connections() connections.Roster {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.connections
}

// Settings are your settings; a broken stream is kept as Roster's is.
func (s *State) Settings() settings.Settings {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.settings
}

// Templates are the team templates you saved, in the order first saved;
// a broken stream is kept as Roster's is. They share nothing with the
// state's own.
func (s *State) Templates() agents.UserTemplates {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.templates.Clone()
}

// RemovedTemplates are the ids of the templates you removed, with the base
// each had; a broken stream is kept as Roster's is. They share nothing with
// the state's own.
func (s *State) RemovedTemplates() agents.RemovedTemplates {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.removed.Clone()
}

// Unknown is what the state skipped or didn't understand: each foreign
// stream prefix's events, then each unknown type's in the other streams.
func (s *State) Unknown() []Unknown {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.skipped.list()
}

// Broken is why a stream's state could not be built, or nil.
func (s *State) Broken(stream string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.broken[stream]
}
