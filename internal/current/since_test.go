package current

import (
	"slices"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// The task order is by each task.created's position, not by when the state
// took it: a state that loaded its streams in any order, then took the rest
// of the log, lists the tasks as the log does.
func TestTheTaskOrderIsByPositionWhateverTheLoadOrder(t *testing.T) {
	log := everyStream()
	log = append(log, logOf(
		proposal("task-3", you, events.TaskCreated, map[string]any{"title": "Third", "repo": "/r", "coordinator": "tech-lead"}),
		proposal("task-4", you, events.TaskCreated, map[string]any{"title": "Fourth", "repo": "/r", "coordinator": "tech-lead"}),
	)...)
	for i := range log {
		log[i].Position = int64(i + 1)
	}
	want := []string{"task-1", "task-2", "task-3", "task-4"}
	for cut := range len(log) + 1 {
		s := New()
		streams := streamsOf(log[:cut])
		slices.Reverse(streams)
		for _, stream := range streams {
			s.Load(stream, accepted(log[:cut], stream))
		}
		s.Loaded(int64(cut))
		for _, e := range log[cut:] {
			s.Apply(e)
		}
		if order := s.TaskIDs(); !slices.Equal(order, want) {
			t.Errorf("loaded the first %d events backwards: task order %v, want %v", cut, order, want)
		}
	}
}
