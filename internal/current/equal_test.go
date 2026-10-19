package current

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// openStore is a real store with no order check, so it also takes events
// the rules would refuse, such as one that breaks a task's apply.
func openStore(t *testing.T, proposals ...events.Proposal) *store.Store {
	t.Helper()
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, p := range proposals {
		propose(t, s, p)
	}
	return s
}

func propose(t *testing.T, s *store.Store, p events.Proposal) {
	t.Helper()
	if _, _, err := s.Propose(p); err != nil {
		t.Fatal(err)
	}
}

// fromTheLog applies every event after the state's position, read back
// from the log: in the stored form, as a restart would load it.
func fromTheLog(t *testing.T, s *store.Store, state *State) {
	t.Helper()
	log, err := s.After(state.Position(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range log {
		state.Apply(e)
	}
}

func someProposals() []events.Proposal {
	var proposals []events.Proposal
	for _, e := range everyStream() {
		proposals = append(proposals, e.Proposal)
	}
	return append(proposals,
		proposal("task-1", you, "task.exploded", nil), // refused by the shape check
		proposal("task-3", you, events.TaskCreated, map[string]any{"title": "Third", "repo": "/r", "coordinator": "tech-lead"}),
		proposal("task-3", worker, events.WorktreeMoved, map[string]any{"worker": "laptop", "path": "/w", "port": 7901}),
	)
}

func TestEqualPassesOnAStateThatFollowedTheLog(t *testing.T) {
	s := openStore(t, someProposals()...)
	state := New()
	state.Loaded(0)
	fromTheLog(t, s, state)
	if err := Equal(s, state); err != nil {
		t.Errorf("applied: %v", err)
	}
	if state.Broken("task-3") == nil {
		t.Errorf("task-3 should be broken, and Equal should have accepted that its replay fails too")
	}

	loadedState := New()
	for _, stream := range []string{"task-3", events.AgentsStream, "task-1", events.WorkersStream, events.PolicyStream, "task-2", events.ConnectionsStream, "idea-1"} {
		accepted, err := s.Stream(stream)
		if err != nil {
			t.Fatal(err)
		}
		loadedState.Load(stream, accepted)
	}
	loadedState.Loaded(s.LastPosition())
	if err := Equal(s, loadedState); err != nil {
		t.Errorf("loaded: %v", err)
	}
}

func TestEqualNamesWhatAMissedApplyLeftBehind(t *testing.T) {
	cases := []struct {
		name   string
		missed events.Proposal
		want   string
	}{
		{"task", proposal("task-2", lead, events.TaskRenamed, map[string]any{"title": "Missed"}), "task-2: field Title"},
		{"new task", proposal("task-4", you, events.TaskCreated, map[string]any{"title": "Missed", "repo": "/r", "coordinator": "tech-lead"}), "task-4:"},
		{"roster", proposal(events.AgentsStream, you, events.AgentLearned, map[string]any{"name": "engineer", "knowledge": "Missed."}), "agents:"},
		{"workers", proposal(events.WorkersStream, system, events.WorkerDisconnected, map[string]any{"name": "laptop"}), "workers: field Workers"},
		{"rejection", proposal("task-1", you, "task.exploded", nil), "position: the state is at"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := openStore(t, someProposals()...)
			state := New()
			state.Loaded(0)
			fromTheLog(t, s, state)
			propose(t, s, c.missed)
			if err := Equal(s, state); err == nil || !strings.HasPrefix(err.Error(), c.want) {
				t.Errorf("Equal after a missed %s: %v, want it to start %q", c.missed.Type, err, c.want)
			}
		})
	}
}

func TestEqualCatchesTheWrongTaskOrder(t *testing.T) {
	s := openStore(t, someProposals()...)
	state := New()
	state.Loaded(0)
	fromTheLog(t, s, state)
	state.taskIDs[0], state.taskIDs[1] = state.taskIDs[1], state.taskIDs[0]
	if err := Equal(s, state); err == nil || !strings.HasPrefix(err.Error(), "task order:") {
		t.Errorf("Equal with task-1 and task-2 swapped: %v", err)
	}
}

// Equal compares every field of a task, so it sees a DoneAt the state got
// wrong though every other field is right.
func TestEqualSeesAWrongDoneAt(t *testing.T) {
	s := openStore(t, someProposals()...)
	state := New()
	state.Loaded(0)
	fromTheLog(t, s, state)
	wrong := state.tasks["task-1"].Clone()
	wrong.DoneAt = time.Unix(1, 0).UTC()
	state.tasks["task-1"] = wrong
	if err := Equal(s, state); err == nil || err.Error() != "task-1: field DoneAt differs from a replay" {
		t.Errorf("Equal with task-1's DoneAt wrong: %v", err)
	}
}
