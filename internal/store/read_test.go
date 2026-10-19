package store

import (
	"math"
	"slices"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// A page of a stream's record is the events of that stream alone, refusals
// included, strictly between its bounds, in order: the first ones from
// LogAfter, the last ones from LogBefore.
func TestALogPageIsTheStreamsEventsBetweenItsBounds(t *testing.T) {
	s := openTestStore(t)
	s.Propose(newTask("task-1"))                                          // 1
	s.Propose(newTask("task-2"))                                          // 2
	for _, id := range []string{"task-1", "task-2", "task-1", "task-1"} { // refused: 3, 4, 5, 6
		s.Propose(newTask(id))
	}
	everything := int64(math.MaxInt64)
	pages := []struct {
		name string
		read func() ([]events.Event, error)
		want []int64
	}{
		{"LogAfter from the start", func() ([]events.Event, error) { return s.LogAfter("task-1", 0, everything, 10) }, []int64{1, 3, 5, 6}},
		{"LogAfter, limited", func() ([]events.Event, error) { return s.LogAfter("task-1", 1, everything, 2) }, []int64{3, 5}},
		{"LogAfter, before 5", func() ([]events.Event, error) { return s.LogAfter("task-1", 0, 5, 10) }, []int64{1, 3}},
		{"LogAfter past the end", func() ([]events.Event, error) { return s.LogAfter("task-1", 6, everything, 10) }, nil},
		{"LogBefore from the end", func() ([]events.Event, error) { return s.LogBefore("task-1", 0, everything, 2) }, []int64{5, 6}},
		{"LogBefore, between", func() ([]events.Event, error) { return s.LogBefore("task-1", 1, 6, 10) }, []int64{3, 5}},
		{"LogBefore the start", func() ([]events.Event, error) { return s.LogBefore("task-1", 0, 1, 10) }, nil},
		{"another stream", func() ([]events.Event, error) { return s.LogAfter("task-2", 0, everything, 10) }, []int64{2, 4}},
	}
	for _, page := range pages {
		found, err := page.read()
		if err != nil {
			t.Fatalf("%s: %v", page.name, err)
		}
		var positions []int64
		for _, e := range found {
			positions = append(positions, e.Position)
		}
		if !slices.Equal(positions, page.want) {
			t.Errorf("%s: positions %v, want %v", page.name, positions, page.want)
		}
	}
	if page, _ := s.LogAfter("task-1", 0, everything, 2); page[1].Type != events.ProposalRejected {
		t.Errorf("a refusal should be in the page as it is: %+v", page[1])
	}
}
