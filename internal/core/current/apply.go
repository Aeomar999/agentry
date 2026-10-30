package current

import (
	"cmp"
	"fmt"
	"log/slog"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/connections"
	"gitlab.com/dev.chempa/agentry/internal/domain/settings"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// ownerOf is the Streams that keeps a stream, or nil for a stream no
// extension here owns.
func (s *State) ownerOf(stream string, kind events.StreamKind) *extensions.Streams {
	if kind != events.OwnedStream {
		return nil
	}
	for prefix, streams := range s.owners {
		if strings.HasPrefix(stream, prefix) {
			return streams
		}
	}
	return nil
}

// Load builds a stream's state from its accepted events, in place: no copy
// per event. The store calls it at Open, once per stream.
func (s *State) Load(stream string, accepted []events.Event) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(accepted) > 0 {
		s.streams[stream] = accepted[len(accepted)-1].Position
	}
	kind := s.catalogue.KindOf(stream)
	for i, e := range accepted {
		s.skipped.count(s.catalogue, kind, e, i == 0)
		s.countMade(e, i == 0)
	}
	if kind == events.ForeignStream {
		return
	}
	for i, e := range accepted {
		s.made(e, kind, i == 0)
	}
	s.fold(stream, accepted)
}

// fold builds a stream's state from its accepted events, as a replay does,
// and keeps it, or marks the stream broken if it can't be built. A foreign
// stream never gets here.
func (s *State) fold(stream string, accepted []events.Event) {
	kind := s.catalogue.KindOf(stream)
	owner := s.ownerOf(stream, kind)
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
		case events.TemplatesStream:
			s.templates = agents.UserTemplatesFromEvents(accepted)
			s.removed = agents.RemovedTemplatesFromEvents(accepted)
		case events.PolicyStream:
		default:
			if owner != nil {
				if held := owner.FromEvents(accepted); held != nil {
					s.owned[stream] = held
				}
			} else if kind != events.TaskStream {
				return
			} else if task, err := tasks.Replay(accepted); err != nil {
				s.breaks(stream, err)
			} else if task != nil {
				s.tasks[stream] = task
			} else if erasure, erased := tasks.Erased(accepted); erased {
				s.erased[stream] = erasure
			} else if summary, archived := tasks.Archived(accepted); archived {
				s.archived[stream] = summary
			}
		}
	})
}

// Refold builds a stream's state again from its accepted events before e
// and e, as Load does, for an event that changes what came before it
// (events.Catalogue.Refolds). What the state held for the stream goes, even
// a broken or archived task, and the position moves as Apply moves it.
// The fold keeps the summary of a task still archived, so task.unarchived
// brings the task back from its events, and a future event.redacted on an
// archived stream leaves it archived.
func (s *State) Refold(stream string, before []events.Event, e events.Event) {
	defer s.wake()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.position = e.Position
	kind := s.catalogue.KindOf(stream)
	first := s.streams[stream] == 0
	s.skipped.count(s.catalogue, kind, e, first)
	s.streams[stream] = e.Position
	s.countMade(e, first)
	if kind == events.ForeignStream {
		return
	}
	s.made(e, kind, first)
	delete(s.tasks, stream)
	delete(s.owned, stream)
	delete(s.broken, stream)
	delete(s.archived, stream)
	delete(s.erased, stream)
	s.fold(stream, append(slices.Clip(before), e))
}

// Loaded ends the load: position is the log's last, rejections included.
func (s *State) Loaded(position int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	slices.SortFunc(s.created, byPosition)
	for _, order := range s.ownedOrder {
		slices.SortFunc(order, byPosition)
	}
	s.position = position
}

func byPosition(a, b created) int { return cmp.Compare(a.position, b.position) }

// wake tells a SnapshotWriter the position moved. It never waits: a wake
// not yet taken already says so.
func (s *State) wake() {
	select {
	case s.moved <- struct{}{}:
	default:
	}
}

// Since is the last position the state holds, for the store to apply the
// rest of the log from: 0 for a new state.
func (s *State) Since() int64 { return s.Position() }

// Apply changes the state by one committed event. A proposal.rejected only
// moves the position, as Store.Stream leaves it out: it never happened.
// A foreign stream's event is only counted. A broken stream takes no more
// events, as a replay would fail before them.
func (s *State) Apply(e events.Event) {
	defer s.wake()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.position = e.Position
	if e.Type == events.ProposalRejected {
		return
	}
	kind := s.catalogue.KindOf(e.Stream)
	first := s.streams[e.Stream] == 0
	s.skipped.count(s.catalogue, kind, e, first)
	s.streams[e.Stream] = e.Position
	s.countMade(e, first)
	if kind == events.ForeignStream {
		return
	}
	s.made(e, kind, first)
	if e.Type == events.TaskErased {
		s.erase(e) // a broken task too: nothing of its stream is left to break
		return
	}
	if s.broken[e.Stream] != nil {
		return
	}
	// Said as tasks.Replay says it, so a task that breaks here reads as one
	// that broke at Load.
	applying := func() string { return fmt.Sprintf("applying %s to %s at %d", e.Type, e.Stream, e.Position) }
	owner := s.ownerOf(e.Stream, kind)
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
		case events.TemplatesStream:
			s.removed = s.removed.Next(e, s.templates)
			s.templates = s.templates.Next(e)
		case events.PolicyStream:
		default:
			if owner != nil {
				if held := owner.Next(s.owned[e.Stream], e); held != nil {
					s.owned[e.Stream] = held
				}
			} else if kind != events.TaskStream {
				return
			} else if e.Type == events.TaskArchived {
				s.archive(e)
			} else if task := s.nextTask(s.tasks[e.Stream], e); task != nil {
				s.tasks[e.Stream] = task
			}
		}
	})
}

// archive keeps a task as its marker's summary alone, as a replay of its
// stream ends: tasks.Replay folds it to nil, and tasks.Archived gives the
// summary.
func (s *State) archive(marker events.Event) {
	summary, _ := tasks.Archived([]events.Event{marker})
	delete(s.tasks, marker.Stream)
	s.archived[marker.Stream] = summary
}

// erase keeps a task as its erasure alone, as a replay of its stream
// ends: tasks.Replay folds it to nil, and tasks.Erased gives the erasure.
func (s *State) erase(marker events.Event) {
	erasure, _ := tasks.Erased([]events.Event{marker})
	delete(s.tasks, marker.Stream)
	delete(s.archived, marker.Stream)
	delete(s.broken, marker.Stream)
	s.erased[marker.Stream] = erasure
}

// nextTask is tasks.Next, or, while Resume applies the events after a
// snapshot, the same change made in place: nobody has read the restored
// tasks yet, and copying a large task for every event would cost more than
// the rest of the load. A panic part way through leaves the task half
// changed, but Resume then throws the whole state away.
func (s *State) nextTask(task *tasks.Task, e events.Event) *tasks.Task {
	if s.tail == nil || task == nil || e.Type == events.TaskCreated {
		return tasks.Next(task, e)
	}
	task.Apply(e)
	return task
}

// made notes where a task.created, or the first event of a stream a
// extension here owns, is in the log, for the task and owned stream orders.
// An archived or erased task's task.created row is gone with the rest, so
// a marker that starts its stream stands for it, at the position it names
// (standsFor).
//
// The store applies in the log's order, after everything Load or an
// earlier Apply gave, so appending a task.created keeps the positions in
// order. A marker's task.created comes before it, so it goes in its place;
// during Load the order is anyone's until Loaded sorts it.
func (s *State) made(e events.Event, kind events.StreamKind, first bool) {
	switch {
	case e.Type == events.TaskCreated:
		s.created = append(s.created, created{e.Position, e.Stream})
	case standsFor(e, first):
		c := created{int64(e.Number("created")), e.Stream}
		at, _ := slices.BinarySearchFunc(s.created, c, byPosition)
		s.created = slices.Insert(s.created, at, c)
	}
	if owner := s.ownerOf(e.Stream, kind); owner != nil && first {
		s.ownedOrder[owner.Prefix] = append(s.ownedOrder[owner.Prefix], created{e.Position, e.Stream})
	}
}

// countMade counts a task.created in any stream, and a marker that stands
// for one (see made), so NextTask names the stream Store.Create would.
func (s *State) countMade(e events.Event, first bool) {
	if e.Type == events.TaskCreated || standsFor(e, first) {
		s.tasksMade++
	}
}

// standsFor says whether e stands for its stream's task.created: a
// task.archived or task.erased that starts the stream, whose rows the
// marker took away, the task.created among them.
func standsFor(e events.Event, first bool) bool {
	return first && (e.Type == events.TaskArchived || e.Type == events.TaskErased)
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
	if s.tail != nil && s.tail.broke == nil {
		s.tail.broke = err
	}
	slog.Error("current: a stream's state can't be built, so its reads fail", "stream", stream, "err", err)
}
