package current

import (
	"path/filepath"
	"slices"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// seededLog writes the proposals to a new log, closes it, and returns its
// path and its events as read back.
func seededLog(t *testing.T, proposals ...events.Proposal) (string, []events.Event) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, err := store.Open(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, p := range proposals {
		propose(t, s, p)
	}
	log, err := s.After(0, len(proposals))
	if err != nil {
		t.Fatal(err)
	}
	return path, log
}

// A state filled from the first part of the log says it holds that much,
// and Open brings it the rest: at every cut, by loading the part or by
// applying it, it ends identical to a replay of the whole log. Cuts fall
// between one task.created and the next, before the broken task and after
// rejections.
func TestAStateThatStartsPartWayTakesTheRestAtOpen(t *testing.T) {
	path, log := seededLog(t, someProposals()...)
	for cut := range len(log) + 1 {
		part := log[:cut]
		var since int64
		if cut > 0 {
			since = part[cut-1].Position
		}
		for name, state := range map[string]*State{"loaded": loaded(part), "applied": applied(part)} {
			if state.Since() != since {
				t.Fatalf("%s from %d events: Since is %d, want %d", name, cut, state.Since(), since)
			}
			s, err := store.Open(path, state, nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := Equal(s, state); err != nil {
				t.Errorf("%s from %d events, then opened: %v", name, cut, err)
			}
			s.Close()
		}
	}
}

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
