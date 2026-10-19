package current

import (
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

var (
	you    = events.Actor{Kind: events.Human, ID: "you"}
	lead   = events.Actor{Kind: events.Agent, ID: "tech-lead"}
	worker = events.Actor{Kind: events.Worker, ID: "local"}
	system = events.Actor{Kind: events.System, ID: "loop"}
)

// logOf numbers proposals into a log, one second apart.
func logOf(proposals ...events.Proposal) []events.Event {
	log := make([]events.Event, len(proposals))
	for i, p := range proposals {
		log[i] = events.Event{Position: int64(i + 1), At: time.Date(2026, 10, 18, 9, 0, i, 0, time.UTC), Proposal: p}
	}
	return log
}

func proposal(stream string, actor events.Actor, kind events.Type, data map[string]any) events.Proposal {
	return events.Proposal{Stream: stream, Actor: actor, Type: kind, Data: data}
}

func rejected(stream string, kind events.Type) events.Proposal {
	return proposal(stream, events.Actor{Kind: events.System, ID: "validator"}, events.ProposalRejected,
		map[string]any{"type": string(kind), "by": "human:you", "check": "order", "reason": "no"})
}

// everyStream is a log touching every kind of stream, interleaved, with
// rejections in a task's stream and in one never created.
func everyStream() []events.Event {
	return logOf(
		proposal(events.AgentsStream, system, events.AgentCreated, map[string]any{"name": "tech-lead", "prompt": "Lead.", "engine": "claude", "coordinates": true}),
		proposal("task-1", you, events.TaskCreated, map[string]any{"title": "Speed up the list", "repo": "/r", "coordinator": "tech-lead"}),
		proposal(events.WorkersStream, you, events.WorkerInvited, map[string]any{"code": "c1", "expires": "2026-10-18T10:00:00Z"}),
		proposal(events.AgentsStream, you, events.AgentCreated, map[string]any{"name": "engineer", "prompt": "Build.", "engine": "claude"}),
		rejected("task-1", events.TaskRenamed),
		proposal(events.PolicyStream, system, events.PolicyLoaded, map[string]any{"hash": "h", "path": "/p"}),
		proposal("task-1", you, events.TaskLabeled, map[string]any{"labels": []any{"web"}}),
		proposal(events.WorkersStream, system, events.WorkerJoined, map[string]any{"name": "laptop", "code": "c1", "credential": "k"}),
		proposal("task-2", you, events.TaskCreated, map[string]any{"title": "Second", "repo": "/r", "coordinator": "tech-lead"}),
		rejected("task-9", events.TaskCreated),
		proposal(events.ConnectionsStream, you, events.ConnectionInvited, map[string]any{"code": "c2", "expires": "2026-10-18T10:00:00Z"}),
		proposal("task-1", lead, events.TaskAssigned, map[string]any{"agent": "engineer", "instructions": "Profile it."}),
		proposal(events.AgentsStream, you, events.AgentLearned, map[string]any{"name": "engineer", "knowledge": "Measure first."}),
		proposal(events.ConnectionsStream, system, events.ConnectionMade, map[string]any{"id": "m1", "code": "c2", "credential": "k2"}),
		proposal(events.WorkersStream, system, events.WorkerConnected, map[string]any{"name": "laptop", "engines": []any{"claude 2"}}),
		proposal("task-2", lead, events.TaskRenamed, map[string]any{"title": "Second, renamed"}),
		proposal(events.AgentsStream, you, events.AgentRemoved, map[string]any{"name": "tech-lead"}),
		proposal("task-1", worker, events.TurnStarted, map[string]any{"agent": "engineer", "hash": "h"}),
		proposal("idea-1", system, events.IdeaSuggested, map[string]any{"title": "Faster", "problem": "Slow", "proposal": "Cache", "size": "S", "repo": "/r", "task": "task-1", "backedBy": []any{"engineer"}}),
		proposal("idea-1", you, events.IdeaAccepted, map[string]any{"task": "task-2", "note": "Yes"}),
	)
}

// accepted is a stream's events as Store.Stream gives them.
func accepted(log []events.Event, stream string) []events.Event {
	var found []events.Event
	for _, e := range log {
		if e.Stream == stream && e.Type != events.ProposalRejected {
			found = append(found, e)
		}
	}
	return found
}

// streamsOf names every stream in the log, in the order first seen.
func streamsOf(log []events.Event) []string {
	var streams []string
	for _, e := range log {
		if !slices.Contains(streams, e.Stream) {
			streams = append(streams, e.Stream)
		}
	}
	return streams
}

// loaded is a state loaded from the log, as the store does at Open.
func loaded(log []events.Event) *State {
	s := New()
	for _, stream := range streamsOf(log) {
		s.Load(stream, accepted(log, stream))
	}
	var last int64
	if len(log) > 0 {
		last = log[len(log)-1].Position
	}
	s.Loaded(last)
	return s
}

// sameAsReplay fails the test where the state differs from FromEvents over the log.
func sameAsReplay(t *testing.T, s *State, log []events.Event) {
	t.Helper()
	for _, stream := range streamsOf(log) {
		if events.IsIdeaStream(stream) {
			idea, err := s.Idea(stream)
			if want := ideas.FromEvents(accepted(log, stream)); err != nil || !reflect.DeepEqual(idea, want) {
				t.Errorf("%s: %+v, %v\nreplay: %+v", stream, idea, err, want)
			}
			continue
		}
		if !events.IsTaskStream(stream) {
			continue
		}
		task, err := s.Task(stream)
		if want := tasks.FromEvents(accepted(log, stream)); err != nil || !reflect.DeepEqual(task, want) {
			t.Errorf("%s: %+v, %v\nreplay: %+v", stream, task, err, want)
		}
	}
	if want := agents.FromEvents(accepted(log, events.AgentsStream)); !reflect.DeepEqual(s.Roster(), want) {
		t.Errorf("agents: %+v\nreplay: %+v", s.Roster(), want)
	}
	if want := workers.FromEvents(accepted(log, events.WorkersStream)); !reflect.DeepEqual(s.Workers(), want) {
		t.Errorf("workers: %+v\nreplay: %+v", s.Workers(), want)
	}
	if want := connections.FromEvents(accepted(log, events.ConnectionsStream)); !reflect.DeepEqual(s.Connections(), want) {
		t.Errorf("connections: %+v\nreplay: %+v", s.Connections(), want)
	}
	if want := log[len(log)-1].Position; s.Position() != want {
		t.Errorf("position %d, want %d", s.Position(), want)
	}
}

// Whatever part of the log is loaded at the start and whatever part is
// applied after, the state is what a replay of the whole log gives.
func TestLoadThenApplyEqualsAReplay(t *testing.T) {
	log := everyStream()
	for split := range len(log) + 1 {
		s := loaded(log[:split])
		for _, e := range log[split:] {
			s.Apply(e)
		}
		sameAsReplay(t, s, log)
		if t.Failed() {
			t.Fatalf("loading the first %d events", split)
		}
	}
}

func TestARejectionMovesOnlyThePosition(t *testing.T) {
	log := everyStream()
	s := loaded(log)
	before, _ := s.Task("task-1")
	for i, p := range []events.Proposal{rejected("task-1", events.TaskRenamed), rejected("task-7", events.TaskCreated), rejected(events.AgentsStream, events.AgentRemoved)} {
		e := events.Event{Position: int64(50 + i), Proposal: p}
		s.Apply(e)
		log = append(log, e)
	}
	if after, _ := s.Task("task-1"); after != before {
		t.Errorf("a rejection changed task-1: %+v", after)
	}
	if task, err := s.Task("task-7"); task != nil || err != nil || len(s.Tasks()) != 2 {
		t.Errorf("a rejected task.created made a task: %+v, %v, %d tasks", task, err, len(s.Tasks()))
	}
	if s.Position() != 52 {
		t.Errorf("position %d, want 52", s.Position())
	}
	sameAsReplay(t, s, log)
}

// A worktree.moved before any worktree.ready panics in tasks' apply (the
// rules never let it through). Only that task's reads fail.
func TestAPanicBreaksOnlyItsStream(t *testing.T) {
	log := everyStream()
	broken := logOf(
		proposal("task-3", you, events.TaskCreated, map[string]any{"title": "Third", "repo": "/r", "coordinator": "tech-lead"}),
		proposal("task-3", worker, events.WorktreeMoved, map[string]any{"worker": "laptop", "path": "/w", "port": float64(7901)}),
	)
	for _, e := range broken {
		e.Position += 100
		log = append(log, e)
	}
	more := proposal("task-1", lead, events.TaskRenamed, map[string]any{"title": "Still applied"})
	after := proposal("task-3", lead, events.TaskRenamed, map[string]any{"title": "Never applied"})

	for name, s := range map[string]*State{"applied": applied(log), "loaded": loaded(log)} {
		s.Apply(events.Event{Position: 200, Proposal: more})
		s.Apply(events.Event{Position: 201, Proposal: after})
		if task, err := s.Task("task-3"); task != nil || err == nil || !strings.Contains(err.Error(), "task-3") {
			t.Errorf("%s: task-3 should fail with its stream's error, got %+v, %v", name, task, err)
		}
		if task, err := s.Task("task-1"); err != nil || task.Title != "Still applied" {
			t.Errorf("%s: task-1 should carry on, got %+v, %v", name, task, err)
		}
		if ids := idsOf(s.Tasks()); !slices.Equal(ids, []string{"task-1", "task-2"}) {
			t.Errorf("%s: Tasks should leave the broken task out, got %v", name, ids)
		}
		if order := s.TaskIDs(); !slices.Equal(order, []string{"task-1", "task-2", "task-3"}) {
			t.Errorf("%s: the task order should still list task-3, as the log does: %v", name, order)
		}
		if s.Position() != 201 {
			t.Errorf("%s: position %d", name, s.Position())
		}
	}
}

// The log lists every task.created, whatever its stream and even on a
// broken one, so loading and applying count them all too.
func TestTheTaskOrderCountsEveryTaskCreated(t *testing.T) {
	log := everyStream()
	for i, p := range []events.Proposal{
		proposal("task-3", you, events.TaskCreated, map[string]any{"title": "Third", "repo": "/r", "coordinator": "tech-lead"}),
		proposal("task-3", worker, events.WorktreeMoved, map[string]any{"worker": "laptop", "path": "/w", "port": float64(7901)}),
		proposal(events.PolicyStream, you, events.TaskCreated, map[string]any{"title": "Odd", "repo": "/r", "coordinator": "tech-lead"}),
		proposal("task-3", you, events.TaskCreated, map[string]any{"title": "Again", "repo": "/r", "coordinator": "tech-lead"}),
	} {
		log = append(log, events.Event{Position: int64(100 + i), Proposal: p})
	}
	want := []string{"task-1", "task-2", "task-3", events.PolicyStream, "task-3"}
	for name, s := range map[string]*State{"applied": applied(log), "loaded": loaded(log)} {
		if order := s.TaskIDs(); !slices.Equal(order, want) {
			t.Errorf("%s: task order %v, want %v", name, order, want)
		}
	}
}

// An idea's stream is an idea, never a task: it stays out of the tasks, the
// list and the task order, so the next task's number doesn't move.
func TestAnIdeaIsNoTask(t *testing.T) {
	log := everyStream()
	for name, s := range map[string]*State{"applied": applied(log), "loaded": loaded(log)} {
		if idea, err := s.Idea("idea-1"); err != nil || idea == nil || idea.State != ideas.Accepted {
			t.Errorf("%s: idea-1 is %+v, %v; want it accepted", name, idea, err)
		}
		if task, err := s.Task("idea-1"); task != nil || err != nil {
			t.Errorf("%s: idea-1 is a task: %+v, %v", name, task, err)
		}
		if order := s.TaskIDs(); !slices.Equal(order, []string{"task-1", "task-2"}) {
			t.Errorf("%s: task order %v, want task-1 and task-2 only", name, order)
		}
		if list := idsOf(s.Tasks()); !slices.Equal(list, []string{"task-1", "task-2"}) {
			t.Errorf("%s: tasks %v, want task-1 and task-2 only", name, list)
		}
	}
}

// No roster event can make its apply panic, so a broken roster stream is
// made by running a panic through the same guard Apply uses. The roster
// stays as it was before, takes no more events, and says why; every other
// stream carries on.
func TestABrokenRosterKeepsItsLastState(t *testing.T) {
	log := everyStream()
	s := loaded(log)
	workersBefore := s.Workers()
	s.guard(events.WorkersStream, "applying worker.exploded to", func() { panic("boom") })
	s.Apply(events.Event{Position: 60, Proposal: proposal(events.WorkersStream, system, events.WorkerDisconnected, map[string]any{"name": "laptop"})})
	s.Apply(events.Event{Position: 61, Proposal: proposal(events.AgentsStream, you, events.AgentRemoved, map[string]any{"name": "engineer"})})

	if err := s.Broken(events.WorkersStream); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Errorf("the workers stream should say why it broke, got %v", err)
	}
	if !reflect.DeepEqual(s.Workers(), workersBefore) {
		t.Errorf("a broken roster took an event: %+v", s.Workers())
	}
	if len(s.Roster()) != 0 || s.Broken(events.AgentsStream) != nil {
		t.Errorf("the agents stream should carry on: %+v", s.Roster())
	}
}

// What a getter handed out is never changed by a later Apply.
func TestWhatWasHandedOutStaysAsItWas(t *testing.T) {
	log := everyStream()
	split := len(log) - 6
	s := loaded(log[:split])
	task1, _ := s.Task("task-1")
	task2, _ := s.Task("task-2")
	roster, workerRoster, connectionRoster := s.Roster(), s.Workers(), s.Connections()
	all := s.Tasks()
	for _, e := range log[split:] {
		s.Apply(e)
	}

	before := log[:split]
	if !reflect.DeepEqual(task1, tasks.FromEvents(accepted(before, "task-1"))) || !reflect.DeepEqual(task2, tasks.FromEvents(accepted(before, "task-2"))) {
		t.Errorf("a task handed out changed:\n%+v\n%+v", task1, task2)
	}
	if !reflect.DeepEqual(all, []*tasks.Task{task1, task2}) {
		t.Errorf("the list handed out changed: %+v", all)
	}
	if !reflect.DeepEqual(roster, agents.FromEvents(accepted(before, events.AgentsStream))) ||
		!reflect.DeepEqual(workerRoster, workers.FromEvents(accepted(before, events.WorkersStream))) ||
		!reflect.DeepEqual(connectionRoster, connections.FromEvents(accepted(before, events.ConnectionsStream))) {
		t.Errorf("a roster handed out changed:\n%+v\n%+v\n%+v", roster, workerRoster, connectionRoster)
	}
	sameAsReplay(t, s, log)
}

func TestTasksAreOldestFirstInANewSlice(t *testing.T) {
	s := applied(everyStream())
	first := s.Tasks()
	if ids := idsOf(first); !slices.Equal(ids, []string{"task-1", "task-2"}) {
		t.Fatalf("Tasks: %v, want task-1 then task-2", ids)
	}
	second := s.Tasks()
	slices.Reverse(second)
	second[0] = nil
	if ids := idsOf(first); !slices.Equal(ids, []string{"task-1", "task-2"}) {
		t.Errorf("sorting one caller's slice changed another's: %v", ids)
	}
	if ids := idsOf(s.Tasks()); !slices.Equal(ids, []string{"task-1", "task-2"}) {
		t.Errorf("sorting the slice Tasks gave changed the state: %v", ids)
	}
}

// Readers call every getter while events are applied (run with -race).
func TestReadersRaceApply(t *testing.T) {
	log := everyStream()
	s := New()
	s.Loaded(0)
	var readers sync.WaitGroup
	done := make(chan struct{})
	for range 4 {
		readers.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				s.Position()
				if task, _ := s.Task("task-1"); task != nil {
					_ = task.Title + string(task.Status())
				}
				for _, task := range s.Tasks() {
					_ = len(task.Trail)
				}
				_ = len(s.Roster()) + len(s.Workers().Workers) + len(s.Connections().Connections)
				_ = s.Broken(events.AgentsStream)
			}
		})
	}
	for _, e := range log {
		s.Apply(e)
	}
	close(done)
	readers.Wait()
	sameAsReplay(t, s, log)
}

// applied is a state built by applying every event, from nothing.
func applied(log []events.Event) *State {
	s := New()
	s.Loaded(0)
	for _, e := range log {
		s.Apply(e)
	}
	return s
}

func idsOf(list []*tasks.Task) []string {
	var ids []string
	for _, task := range list {
		if task == nil {
			ids = append(ids, "<nil>")
		} else {
			ids = append(ids, task.ID)
		}
	}
	return ids
}
