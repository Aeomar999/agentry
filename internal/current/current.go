// Package current keeps the state of every stream in memory, as of the
// last event the log committed. The log stays the only thing stored: the
// state is built from it at every start and changed only by the store,
// as each event commits, so it can't disagree with what happened.
//
// A stream is a task's when events.IsTaskStream says so, as the rules and
// the coordinator tell them apart. The agents, workers and connections
// streams each make a roster. The policy stream keeps no state here: its
// events only move the position.
package current

import (
	"cmp"
	"fmt"
	"slices"
	"sync"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/events"
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
	taskIDs     []string               // oldest first, as Store.TaskIDs
	created     []created              // the task.created events Load found, until Loaded puts them in order
	broken      map[string]error       // streams whose apply panicked: reads fail, as a replay would
	roster      agents.Roster
	workers     workers.Roster
	connections connections.Roster
}

// created is a task.created event's place in the log, which orders taskIDs.
type created struct {
	position int64
	id       string
}

func New() *State {
	return &State{tasks: map[string]*tasks.Task{}, broken: map[string]error{}}
}

// Load builds a stream's state from its accepted events, in place: no copy
// per event. The store calls it at Open, once per stream.
func (s *State) Load(stream string, accepted []events.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if events.IsTaskStream(stream) {
		for _, e := range accepted {
			if e.Type == events.TaskCreated {
				s.created = append(s.created, created{e.Position, stream})
			}
		}
	}
	s.guard(stream, "building", func() {
		switch stream {
		case events.AgentsStream:
			s.roster = agents.FromEvents(accepted)
		case events.WorkersStream:
			s.workers = workers.FromEvents(accepted)
		case events.ConnectionsStream:
			s.connections = connections.FromEvents(accepted)
		case events.PolicyStream:
		default:
			if task := tasks.FromEvents(accepted); task != nil {
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
	if e.Type == events.ProposalRejected || s.broken[e.Stream] != nil {
		return
	}
	if e.Type == events.TaskCreated {
		s.taskIDs = append(s.taskIDs, e.Stream)
	}
	s.guard(e.Stream, "applying "+string(e.Type)+" to", func() {
		switch e.Stream {
		case events.AgentsStream:
			s.roster = s.roster.Next(e)
		case events.WorkersStream:
			s.workers = s.workers.Next(e)
		case events.ConnectionsStream:
			s.connections = s.connections.Next(e)
		case events.PolicyStream:
		default:
			if task := tasks.Next(s.tasks[e.Stream], e); task != nil {
				s.tasks[e.Stream] = task
			}
		}
	})
}

// guard runs change, and marks the stream broken if it panics, so one bad
// stream fails its own reads and leaves every other stream working.
func (s *State) guard(stream, doing string, change func()) {
	defer func() {
		if recovered := recover(); recovered != nil {
			s.broken[stream] = fmt.Errorf("%s %s: %v", doing, stream, recovered)
		}
	}()
	change()
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

// Roster is every agent. If the agents stream is broken, it is the roster
// as it was before the event that broke it, and Broken says why.
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

// Broken is why a stream's state could not be built, or nil.
func (s *State) Broken(stream string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.broken[stream]
}
