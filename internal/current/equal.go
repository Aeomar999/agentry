package current

import (
	"fmt"
	"reflect"
	"slices"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// Equal checks the state against a replay of the store's log, which stays
// the definition of state: every task, idea and roster, the task order and
// the position. It returns the first difference, named by stream, or nil. A
// stream the state holds as broken must fail to replay too.
func Equal(s *store.Store, state *State) error {
	ids, err := taskIDsIn(s)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := equalStream(s, id, tasks.Replay, func() (any, error) { return state.Task(id) }); err != nil {
			return err
		}
	}
	ideaIDs, err := s.IdeaIDs()
	if err != nil {
		return err
	}
	for _, id := range ideaIDs {
		if err := equalStream(s, id, infallible(ideas.FromEvents), func() (any, error) { return state.Idea(id) }); err != nil {
			return err
		}
	}
	rosters := []struct {
		stream string
		replay func([]events.Event) (any, error)
		held   any
	}{
		{events.AgentsStream, func(e []events.Event) (any, error) { return agents.FromEvents(e), nil }, state.Roster()},
		{events.WorkersStream, func(e []events.Event) (any, error) { return workers.FromEvents(e), nil }, state.Workers()},
		{events.ConnectionsStream, func(e []events.Event) (any, error) { return connections.FromEvents(e), nil }, state.Connections()},
	}
	for _, r := range rosters {
		held := func() (any, error) { return r.held, state.Broken(r.stream) }
		if err := equalStream(s, r.stream, r.replay, held); err != nil {
			return err
		}
	}
	if order := state.TaskIDs(); !slices.Equal(order, ids) {
		return fmt.Errorf("task order: the state has %v, the log %v", order, ids)
	}
	if position, last := state.Position(), s.LastPosition(); position != last {
		return fmt.Errorf("position: the state is at %d, the log at %d", position, last)
	}
	return nil
}

// equalStream compares what the state holds for a stream with a replay of
// the stream's accepted events in the log.
func equalStream[T any](s *store.Store, stream string, replay func([]events.Event) (T, error), held func() (any, error)) error {
	accepted, err := s.Stream(stream)
	if err != nil {
		return err
	}
	return equalReplay(stream, accepted, replay, held)
}

// equalReplay compares what the state holds for a stream with a replay of
// accepted, the stream's accepted events.
func equalReplay[T any](stream string, accepted []events.Event, replay func([]events.Event) (T, error), held func() (any, error)) error {
	want, replayErr := replayed(replay, accepted)
	got, heldErr := held()
	switch {
	case replayErr != nil && heldErr != nil:
		return nil
	case replayErr != nil:
		return fmt.Errorf("%s: the replay fails (%v), the state doesn't", stream, replayErr)
	case heldErr != nil:
		return fmt.Errorf("%s: the state is broken (%v), the replay isn't", stream, heldErr)
	}
	if !reflect.DeepEqual(got, any(want)) {
		return fmt.Errorf("%s: %s differs from a replay", stream, differingField(got, want))
	}
	return nil
}

// replayed runs a replay, turning a panic into an error as the state does.
// A task's replay gives its own error, as tasks.Replay recovers the panic.
func replayed[T any](replay func([]events.Event) (T, error), accepted []events.Event) (state T, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%v", recovered)
		}
	}()
	return replay(accepted)
}

// infallible is a FromEvents in the shape of tasks.Replay: its failure is
// a panic, which replayed recovers.
func infallible[T any](fromEvents func([]events.Event) T) func([]events.Event) (T, error) {
	return func(accepted []events.Event) (T, error) { return fromEvents(accepted), nil }
}

// differingField names the first exported field that differs, when both
// are structs or pointers to them; otherwise the whole value.
func differingField(got, want any) string {
	a, b := reflect.Indirect(reflect.ValueOf(got)), reflect.Indirect(reflect.ValueOf(want))
	if !a.IsValid() || !b.IsValid() || a.Kind() != reflect.Struct || a.Type() != b.Type() {
		return "the state"
	}
	for i := range a.NumField() {
		field := a.Type().Field(i)
		if field.IsExported() && !reflect.DeepEqual(a.Field(i).Interface(), b.Field(i).Interface()) {
			return "field " + field.Name
		}
	}
	return "an unexported field"
}

// taskIDsIn is the stream of every accepted task.created in the log,
// oldest first, whatever the stream.
func taskIDsIn(s *store.Store) ([]string, error) {
	const page = 1000
	var ids []string
	for position := int64(0); ; {
		batch, err := s.After(position, page)
		if err != nil {
			return nil, err
		}
		for _, e := range batch {
			if e.Type == events.TaskCreated {
				ids = append(ids, e.Stream)
			}
		}
		if len(batch) < page {
			return ids, nil
		}
		position = batch[len(batch)-1].Position
	}
}
