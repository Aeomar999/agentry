// Package current keeps the state of every stream in memory, as of the
// last event the log committed. The log stays the only thing stored: the
// state is built from it at every start and changed only by the store,
// as each event commits, so it can't disagree with what happened.
//
// A stream is a task's when events.IsTaskStream says so, as the rules and
// the coordinator tell them apart, and an idea's when events.IsIdeaStream
// does: an idea is never a task. The agents, workers and connections
// streams each make a roster, and the settings stream your settings. The
// policy stream keeps no state here: its events only move the position.
package current

import (
	"cmp"
	"fmt"
	"log/slog"
	"slices"
	"sync"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/settings"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// State is every stream's state. A *tasks.Task or a roster it hands out is
// never changed afterwards: applying an event replaces it with a new one,
// so readers may keep what they got as long as they like, without a lock.
// They must not change it themselves.
type State struct {
	mu          sync.RWMutex
	position    int64                  // of the last event applied, rejections included
	tasks       map[string]*tasks.Task // by stream
	taskIDs     []string               // every task.created applied, oldest first, as the log lists them
	ideas       map[string]*ideas.Idea // by stream
	created     []created              // the task.created events Load found, until Loaded puts them in order
	broken      map[string]error       // streams whose apply panicked: reads fail, as a replay would
	roster      agents.Roster
	workers     workers.Roster
	connections connections.Roster
	settings    settings.Settings
}

// created is a task.created event's place in the log, which orders taskIDs.
type created struct {
	position int64
	id       string
}

func New() *State {
	return &State{tasks: map[string]*tasks.Task{}, ideas: map[string]*ideas.Idea{}, broken: map[string]error{},
		settings: settings.Default()}
}

// Load builds a stream's state from its accepted events, in place: no copy
// per event. The store calls it at Open, once per stream.
func (s *State) Load(stream string, accepted []events.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, e := range accepted {
		if e.Type == events.TaskCreated {
			s.created = append(s.created, created{e.Position, stream})
		}
	}
	s.guard(stream, func() string { return "building " + stream }, func() {
		switch stream {
		case events.AgentsStream:
			s.roster = agents.FromEvents(accepted)
		case events.WorkersStream:
			s.workers = workers.FromEvents(accepted)
		case events.ConnectionsStream:
			s.connections = connections.FromEvents(accepted)
		case events.SettingsStream:
			s.settings = settings.FromEvents(accepted)
		case events.PolicyStream:
		default:
			if events.IsIdeaStream(stream) {
				if idea := ideas.FromEvents(accepted); idea != nil {
					s.ideas[stream] = idea
				}
			} else if task, err := tasks.Replay(accepted); err != nil {
				s.breaks(stream, err)
			} else if task != nil {
				s.tasks[stream] = task
			}
		}
	})
}

// Loaded ends the load: position is the log's last, rejections included.
func (s *State) Loaded(position int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slices.SortFunc(s.created, func(a, b created) int { return cmp.Compare(a.position, b.position) })
	for _, c := range s.created {
		s.taskIDs = append(s.taskIDs, c.id)
	}
	s.created = nil
	s.position = position
}

// Apply changes the state by one committed event. A proposal.rejected only
// moves the position, as Store.Stream leaves it out: it never happened.
// A broken stream takes no more events, as a replay would fail before them.
func (s *State) Apply(e events.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.position = e.Position
	if e.Type == events.ProposalRejected {
		return
	}
	if e.Type == events.TaskCreated {
		s.taskIDs = append(s.taskIDs, e.Stream)
	}
	if s.broken[e.Stream] != nil {
		return
	}
	// Said as tasks.Replay says it, so a task that breaks here reads as one
	// that broke at Load.
	applying := func() string { return fmt.Sprintf("applying %s to %s at %d", e.Type, e.Stream, e.Position) }
	s.guard(e.Stream, applying, func() {
		switch e.Stream {
		case events.AgentsStream:
			s.roster = s.roster.Next(e)
		case events.WorkersStream:
			s.workers = s.workers.Next(e)
		case events.ConnectionsStream:
			s.connections = s.connections.Next(e)
		case events.SettingsStream:
			s.settings = s.settings.Next(e)
		case events.PolicyStream:
		default:
			if events.IsIdeaStream(e.Stream) {
				if idea := ideas.Next(s.ideas[e.Stream], e); idea != nil {
					s.ideas[e.Stream] = idea
				}
			} else if task := tasks.Next(s.tasks[e.Stream], e); task != nil {
				s.tasks[e.Stream] = task
			}
		}
	})
}

// guard runs change, and marks the stream broken if it panics, so one bad
// stream fails its own reads and leaves every other stream working. doing
// says what change was doing, for the error.
func (s *State) guard(stream string, doing func() string, change func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			s.breaks(stream, fmt.Errorf("%s: %v", doing(), recovered))
		}
	}()
	change()
}

// breaks marks a stream broken, and logs it: a broken task leaves the lists,
// and the log line is where you find out why.
func (s *State) breaks(stream string, err error) {
	s.broken[stream] = err
	slog.Error("current: a stream's state can't be built, so its reads fail", "stream", stream, "err", err)
}

// Position is the last event applied, rejections included.
func (s *State) Position() int64 {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.position
}

// Task is a task's state: nil, nil when there is no such task, and the
// error when its stream is broken.
func (s *State) Task(id string) (*tasks.Task, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.broken[id]; err != nil {
		return nil, err
	}
	return s.tasks[id], nil
}

// Idea is an idea's state: nil, nil when there is no such idea, and the
// error when its stream is broken.
func (s *State) Idea(id string) (*ideas.Idea, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if err := s.broken[id]; err != nil {
		return nil, err
	}
	return s.ideas[id], nil
}

// Tasks is every task, oldest first, in a new slice the caller may sort.
// A broken task is left out, as the list skips a task it can't read.
func (s *State) Tasks() []*tasks.Task {
	s.mu.RLock()
	defer s.mu.RUnlock()
	all := make([]*tasks.Task, 0, len(s.taskIDs))
	for _, id := range s.taskIDs {
		if task := s.tasks[id]; task != nil && s.broken[id] == nil {
			all = append(all, task)
		}
	}
	return all
}

// TaskIDs is the stream of every task.created applied, oldest first, in a
// new slice: a broken task's too, so its length counts every task made.
func (s *State) TaskIDs() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return slices.Clone(s.taskIDs)
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

// Broken is why a stream's state could not be built, or nil.
func (s *State) Broken(stream string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.broken[stream]
}
