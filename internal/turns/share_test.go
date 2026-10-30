package turns

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/platform/sessions"
	"gitlab.com/dev.chempa/agentry/internal/platform/sessions/sessionstest"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// fakeClaudes is fakeClaude with a record per task, in dir, so turns of
// several tasks can launch at once.
func fakeClaudes(t *testing.T) (dir string) {
	t.Helper()
	sessionstest.Isolate(t)
	bin, dir := t.TempDir(), t.TempDir()
	record := dir + "/$AGENTRY_TASK"
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + record + ".part && mv " + record + ".part " + record + "\n" + sessionstest.Wait() + "\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	return dir
}

// anEpic makes task-1, an epic, and its stories task-2 to task-<1+n>,
// each with a worktree. On task-2, the tech-lead has had a turn that ended
// in session s-1.
func anEpic(t *testing.T, s *store.Store, n int) {
	t.Helper()
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	you := events.Actor{Kind: events.Human, ID: "you"}
	worker := events.Actor{Kind: events.Worker, ID: workers.Local}
	propose := func(p events.Proposal) {
		t.Helper()
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, p.Stream, rejection, err)
		}
	}
	propose(events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Share sessions", "repo": "/r", "coordinator": "tech-lead", "kind": "epic", "parked": "backlog"}})
	for i := 2; i <= 1+n; i++ {
		id := "task-" + string(rune('0'+i))
		propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Story " + id, "repo": "/r", "coordinator": "tech-lead"}})
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": t.TempDir(), "branch": "agentry/" + id, "port": 7900 + i}})
		propose(events.Proposal{Stream: "task-1", Actor: you, Type: events.StoryAdded, Data: map[string]any{"story": id, "title": "Story " + id}})
		propose(events.Proposal{Stream: id, Actor: you, Type: events.EpicJoined, Data: map[string]any{"epic": "task-1"}})
	}
	propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1",
		Data: map[string]any{"agent": "tech-lead", "engine": "claude", "hash": "h", "worker": workers.Local, "freshBecause": "first turn"}})
	propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.SessionStarted, Correlation: "turn-1", Data: map[string]any{"engineSession": "s-1"}})
	propose(events.Proposal{Stream: "task-2", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "report": "done"}})
}

func storyOf(t *testing.T, s *store.Store, id string) *tasks.Task {
	t.Helper()
	return tasks.FromEvents(mustStream(t, s, id))
}

// launchOf waits for the task's engine launch and gives its arguments.
func launchOf(t *testing.T, dir, task string) string {
	t.Helper()
	return launched(t, filepath.Join(dir, task))
}

func TestAFirstTurnOnAStoryGoesOnWithASiblingsSessionAndForksItWhenBusy(t *testing.T) {
	dir := fakeClaudes(t)
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	anEpic(t, s, 3)
	defer sessions.Stop(sessions.SocketFor("task-3"))
	defer sessions.Stop(sessions.SocketFor("task-4"))
	lead, _ := agents.FromEvents(mustStream(t, s, events.AgentsStream)).Get("tech-lead")
	runner := NewRunner(s, state, here(), Config{})

	third := storyOf(t, s, "task-3")
	session := runner.SessionToResume(third, lead)
	if session != (Session{Resume: "s-1", From: "task-2"}) {
		t.Fatalf("task-3's first turn: %+v, want to resume s-1 from task-2, having seen none of task-3", session)
	}
	if err := runner.Start(third, lead, "brief", session); err != nil {
		t.Fatal(err)
	}
	if launch := launchOf(t, dir, "task-3"); !strings.Contains(launch, "--resume\ns-1\n") || strings.Contains(launch, "--fork-session") {
		t.Errorf("task-3 launched with\n%s\nwant --resume s-1 in place", launch)
	}
	if turn := storyOf(t, s, "task-3").Turn("turn-1"); turn.FromSession != "s-1" || turn.SessionFrom != "task-2" || turn.Forked {
		t.Errorf("task-3's turn: from %q of %q, forked %v; want s-1 of task-2, in place", turn.FromSession, turn.SessionFrom, turn.Forked)
	}

	// task-3's open turn started from s-1, so it is busy: task-4 forks it.
	fourth := storyOf(t, s, "task-4")
	if session := runner.SessionToResume(fourth, lead); session != (Session{Resume: "s-1", From: "task-2", Fork: true}) {
		t.Fatalf("task-4's first turn: %+v, want to fork s-1 from task-2", session)
	}
	if err := runner.Start(fourth, lead, "brief", runner.SessionToResume(fourth, lead)); err != nil {
		t.Fatal(err)
	}
	if launch := launchOf(t, dir, "task-4"); !strings.Contains(launch, "--resume\ns-1\n--fork-session\n") {
		t.Errorf("task-4 launched with\n%s\nwant --resume s-1 --fork-session", launch)
	}
	if turn := storyOf(t, s, "task-4").Turn("turn-1"); turn.FromSession != "s-1" || turn.SessionFrom != "task-2" || !turn.Forked {
		t.Errorf("task-4's turn: from %q of %q, forked %v; want a fork of s-1 of task-2", turn.FromSession, turn.SessionFrom, turn.Forked)
	}
}

// A story's own session is busy too while a sibling's turn goes on with it.
func TestAStorysOwnSessionForksWhileASiblingIsInIt(t *testing.T) {
	fakeClaudes(t)
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	anEpic(t, s, 2)
	defer sessions.Stop(sessions.SocketFor("task-3"))
	lead, _ := agents.FromEvents(mustStream(t, s, events.AgentsStream)).Get("tech-lead")
	runner := NewRunner(s, state, here(), Config{})
	third := storyOf(t, s, "task-3")
	if err := runner.Start(third, lead, "brief", runner.SessionToResume(third, lead)); err != nil {
		t.Fatal(err)
	}
	own := storyOf(t, s, "task-2")
	if session := runner.SessionToResume(own, lead); session != (Session{Resume: "s-1", From: "task-2", Fork: true, Seen: own.Turn("turn-1").Seen}) {
		t.Errorf("task-2's next turn: %+v, want to fork its own s-1", session)
	}
}

func TestResumeOffKeepsEveryStoryFresh(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	anEpic(t, s, 2)
	lead, _ := agents.FromEvents(mustStream(t, s, events.AgentsStream)).Get("tech-lead")
	if session := NewRunner(s, state, here(), Config{NoResume: true}).SessionToResume(storyOf(t, s, "task-3"), lead); session != (Session{Fresh: ResumeOff}) {
		t.Errorf("with resume off: %+v, want fresh", session)
	}
}

// Two stories that start at once both decided to resume s-1 before either
// turn was in the log; the runner forks the later, so only one goes on
// with s-1 in place.
func TestTwoStoriesStartingAtOnceFromOneSessionNeverBothResumeIt(t *testing.T) {
	dir := fakeClaudes(t)
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	anEpic(t, s, 3)
	defer sessions.Stop(sessions.SocketFor("task-3"))
	defer sessions.Stop(sessions.SocketFor("task-4"))
	lead, _ := agents.FromEvents(mustStream(t, s, events.AgentsStream)).Get("tech-lead")
	runner := NewRunner(s, state, here(), Config{})

	stories := []*tasks.Task{storyOf(t, s, "task-3"), storyOf(t, s, "task-4")}
	decided := make([]Session, len(stories))
	for i, story := range stories {
		if decided[i] = runner.SessionToResume(story, lead); decided[i] != (Session{Resume: "s-1", From: "task-2"}) {
			t.Fatalf("%s decided %+v, want to resume s-1 in place", story.ID, decided[i])
		}
	}
	var wg sync.WaitGroup
	for i, story := range stories {
		wg.Go(func() {
			if err := runner.Start(story, lead, "brief", decided[i]); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()

	inPlace, forked := 0, 0
	for _, story := range stories {
		turn := storyOf(t, s, story.ID).Turn("turn-1")
		forks := strings.Contains(launchOf(t, dir, story.ID), "--fork-session")
		if turn.FromSession != "s-1" || turn.Forked != forks {
			t.Errorf("%s: from %q, forked %v, launched forking %v", story.ID, turn.FromSession, turn.Forked, forks)
		}
		if turn.Forked {
			forked++
		} else {
			inPlace++
		}
	}
	if inPlace != 1 || forked != 1 {
		t.Errorf("%d resumed s-1 in place and %d forked it, want 1 and 1", inPlace, forked)
	}
}
