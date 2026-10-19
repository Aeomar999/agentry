package current

import (
	"path/filepath"
	"slices"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// seededLog writes the proposals to a new log, closes it, and returns its
// path and its events as read back.
func seededLog(t *testing.T, proposals ...events.Proposal) (string, []events.Event) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, err := store.Open(path, withIdeas, nil, nil)
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
			s, err := store.Open(path, withIdeas, state, nil)
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
		s := New(withIdeas)
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

// The ideas are in the order of their idea.suggested positions too, however
// the state took them, and a broken idea is left out of the list.
func TestTheIdeasAreInLogOrderWithoutBrokenOnes(t *testing.T) {
	suggested := func(stream, title string) events.Proposal {
		return proposal(stream, system, ideas.IdeaSuggested, map[string]any{"title": title, "problem": "p", "proposal": "p", "size": "S", "repo": "/r", "task": "task-1"})
	}
	log := everyStream()
	log = append(log, logOf(suggested("idea-3", "Third"), suggested("idea-2", "Second"), suggested("idea-4", "Fourth"))...)
	for i := range log {
		log[i].Position = int64(i + 1)
	}
	for cut := range len(log) + 1 {
		s := New(withIdeas)
		streams := streamsOf(log[:cut])
		slices.Reverse(streams)
		for _, stream := range streams {
			s.Load(stream, accepted(log[:cut], stream))
		}
		s.Loaded(int64(cut))
		for _, e := range log[cut:] {
			s.Apply(e)
		}
		if got := ideaIDsOf(s.Ideas()); !slices.Equal(got, []string{"idea-1", "idea-3", "idea-2", "idea-4"}) {
			t.Errorf("loaded the first %d events backwards: ideas %v", cut, got)
		}
		s.guard("idea-2", func() string { return "applying idea.exploded to idea-2" }, func() { panic("boom") })
		if got := ideaIDsOf(s.Ideas()); !slices.Equal(got, []string{"idea-1", "idea-3", "idea-4"}) {
			t.Errorf("loaded the first %d events backwards: with idea-2 broken, ideas %v", cut, got)
		}
		if order := s.ideaIDs(); !slices.Equal(order, []string{"idea-1", "idea-3", "idea-2", "idea-4"}) {
			t.Errorf("loaded the first %d events backwards: the idea order should still list idea-2, as the log does: %v", cut, order)
		}
	}
}

func ideaIDsOf(list []*ideas.Idea) []string {
	var ids []string
	for _, idea := range list {
		ids = append(ids, idea.ID)
	}
	return ids
}

// A state that started part way goes on like one that loaded the whole log:
// tasks created after Open come last in the order, and the state still
// equals a replay.
func TestAStateThatStartsPartWayTakesWhatComesAfterOpen(t *testing.T) {
	path, log := seededLog(t, someProposals()...)
	state := applied(log[:len(log)/2])
	s, err := store.Open(path, withIdeas, state, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	before := state.TaskIDs()
	propose(t, s, proposal("task-9", you, events.TaskCreated, map[string]any{"title": "Later", "repo": "/r", "coordinator": "tech-lead"}))
	propose(t, s, proposal("task-9", you, events.TaskHeld, map[string]any{"place": "backlog"}))
	if order := state.TaskIDs(); !slices.Equal(order, append(before, "task-9")) {
		t.Errorf("task order %v, want %v then task-9", order, before)
	}
	if position := state.StreamPosition("task-9"); position != s.LastPosition() {
		t.Errorf("task-9 at %d, want the log's last, %d", position, s.LastPosition())
	}
	if err := Equal(s, state); err != nil {
		t.Error(err)
	}
}
