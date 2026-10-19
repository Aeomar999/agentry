package coordinator

import (
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/settings"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

func mustPropose(t *testing.T, s *store.Store, p events.Proposal) events.Event {
	t.Helper()
	e, rejection, err := s.Propose(p)
	if rejection != nil || err != nil {
		t.Fatalf("%s on %s: %v %v", p.Type, p.Stream, rejection, err)
	}
	return e
}

// newEpic makes an epic, parked so no loop takes it up, with one story
// that is done already.
func newEpic(t *testing.T, s *store.Store, id, repo string) {
	t.Helper()
	story := id + "-story"
	mustPropose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Ideas list",
		"description": "Keep the Visionaries' ideas in a list.", "repo": repo, "coordinator": "tech-lead", "kind": "epic", "parked": "backlog"}})
	mustPropose(t, s, events.Proposal{Stream: story, Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Ideas page",
		"repo": repo, "coordinator": "tech-lead", "parked": "backlog"}})
	mustPropose(t, s, events.Proposal{Stream: story, Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "Built the Ideas page.", "moved": true}})
	mustPropose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.StoryAdded, Data: map[string]any{"story": story, "title": "Ideas page",
		"done": true, "summary": "Built the Ideas page."}})
	mustPropose(t, s, events.Proposal{Stream: story, Actor: you, Type: events.EpicJoined, Data: map[string]any{"epic": id}})
}

// closeEpic asks to close the epic, as its coordinator's finish does (here
// Agentry asks, as the epic is parked), then closes it as your "Close the
// epic" answer does in web's answer and MCP's close_epic: task.done with
// its closing summary. It returns the close.
func closeEpic(t *testing.T, s *store.Store, id string) events.Event {
	t.Helper()
	mustPropose(t, s, events.Proposal{Stream: id, Actor: agentry, Type: events.TaskNeedsYou, Data: map[string]any{"question": "Close it?",
		"options": []string{CloseEpic}, "closing": "Ideas are kept and shown."}})
	epic := load(t, s, id)
	return mustPropose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": epic.Closing}})
}

// sessionsAbout are the tasks Agentry made after the epic closed.
func sessionsAbout(state *current.State, epic string) []*tasks.Task {
	var about []*tasks.Task
	for _, task := range state.Tasks() {
		if task.About == epic {
			about = append(about, task)
		}
	}
	return about
}

// followUpsOf runs the loop's trigger on the whole log, as if every event
// in it came after the loop started.
func followUpsOf(t *testing.T, s *store.Store, loop *Loop) {
	t.Helper()
	log, err := s.After(0, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	loop.startedAt = 0
	loop.followUps(log)
}

func TestClosingAnEpicStartsOneVisionariesSessionAboutIt(t *testing.T) {
	s, state, repo := withTeams(t, true)
	newEpic(t, s, "task-epic", repo)
	closed := closeEpic(t, s, "task-epic")
	loop := newLoop(s, state, t.TempDir())
	followUpsOf(t, s, loop)
	followUpsOf(t, s, loop) // a second pass over the same close, as a re-run would

	found := sessionsAbout(state, "task-epic")
	if len(found) != 1 {
		t.Fatalf("one close, one session: %d", len(found))
	}
	session := found[0]
	if session.AboutClose != closed.Position || session.Coordinator != "editor" || session.Budget != FollowUpBudget ||
		session.MaxTurns != FollowUpMaxTurns || session.Parked != "" || session.Repo != repo || session.Kind != tasks.Story || !session.KindChosen {
		t.Errorf("the session: %+v", session)
	}
	if session.Title != "Follow-ups after Ideas list" {
		t.Errorf("title %q", session.Title)
	}
	for _, want := range []string{"task-epic", "Ideas list", "Keep the Visionaries' ideas in a list.", "Ideas are kept and shown.",
		"task-epic-story: Ideas page", "Its summary: Built the Ideas page.", `"epic" is "task-epic"`} {
		if !strings.Contains(session.Description, want) {
			t.Errorf("the brief should name %q:\n%s", want, session.Description)
		}
	}
	// The close itself stands as task-16 made it: done, with its summary.
	if epic := load(t, s, "task-epic"); !epic.IsDone || epic.Summary != "Ideas are kept and shown." || epic.Closing != "" {
		t.Errorf("the closed epic: done %v, summary %q, closing %q", epic.IsDone, epic.Summary, epic.Closing)
	}
}

func TestAReopenedEpicClosedAgainGetsASecondSession(t *testing.T) {
	s, state, repo := withTeams(t, true)
	loop := newLoop(s, state, t.TempDir())
	newEpic(t, s, "task-epic", repo)
	first := closeEpic(t, s, "task-epic")
	followUpsOf(t, s, loop)
	mustPropose(t, s, events.Proposal{Stream: "task-epic", Actor: you, Type: events.TaskReopened, Data: map[string]any{"text": "One more story."}})
	second := closeEpic(t, s, "task-epic")
	followUpsOf(t, s, loop)

	found := sessionsAbout(state, "task-epic")
	if len(found) != 2 || found[0].AboutClose != first.Position || found[1].AboutClose != second.Position {
		t.Fatalf("two closes, two sessions: %+v", found)
	}
}

func TestNoSessionWhenTheSettingIsOff(t *testing.T) {
	s, state, repo := withTeams(t, true)
	mustPropose(t, s, events.Proposal{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged,
		Data: map[string]any{settings.VisionariesOnEpicClose: false}})
	newEpic(t, s, "task-epic", repo)
	closeEpic(t, s, "task-epic")
	followUpsOf(t, s, newLoop(s, state, t.TempDir()))

	if found := sessionsAbout(state, "task-epic"); len(found) != 0 {
		t.Errorf("the setting is off: %+v", found)
	}
	if epic := load(t, s, "task-epic"); epic.Skipped != 0 {
		t.Errorf("turned off, nothing is said either: skipped %d", epic.Skipped)
	}
}

func TestAnEpicMovedToDoneGetsNoSession(t *testing.T) {
	s, state, repo := withTeams(t, true)
	newEpic(t, s, "task-epic", repo)
	mustPropose(t, s, events.Proposal{Stream: "task-epic", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "", "moved": true}})
	followUpsOf(t, s, newLoop(s, state, t.TempDir()))

	if found := sessionsAbout(state, "task-epic"); len(found) != 0 {
		t.Errorf("a move to Done isn't a close: %+v", found)
	}
}

// Without a Visionaries team the epic says how to add one, once per close.
func TestWithoutVisionariesTheEpicSaysHowToAddThem(t *testing.T) {
	s, state, repo := withTeams(t, false)
	newEpic(t, s, "task-epic", repo)
	closed := closeEpic(t, s, "task-epic")
	loop := newLoop(s, state, t.TempDir())
	followUpsOf(t, s, loop)
	followUpsOf(t, s, loop)

	if found := sessionsAbout(state, "task-epic"); len(found) != 0 {
		t.Errorf("no Visionaries team, no session: %+v", found)
	}
	epic := load(t, s, "task-epic")
	if last := epic.Trail[len(epic.Trail)-1]; epic.Skipped != closed.Position || last.From != tasks.Agentry || last.To != tasks.You ||
		!strings.Contains(last.Text, "on the Agents page, Add a team, Visionaries") || !strings.Contains(last.Text, "turn these sessions off") {
		t.Errorf("the epic should say how to add the team: skipped %d, last step %+v", epic.Skipped, last)
	}
	notes := 0
	for _, e := range mustStream(t, s, "task-epic") {
		if e.Type == events.FollowUpsSkipped {
			notes++
		}
	}
	if notes != 1 {
		t.Errorf("said %d times, want once", notes)
	}
}

// The running loop starts a session for a close written after it was made,
// even one written before Run reads the log (the server takes requests by
// then), and none for one before: every start reads the whole log again.
func TestTheLoopFollowsUpOnlyCloseAfterItStarted(t *testing.T) {
	s, state, repo := withTeams(t, true)
	newEpic(t, s, "task-old", repo)
	closeEpic(t, s, "task-old")
	loop := newLoop(s, state, t.TempDir())
	newEpic(t, s, "task-new", repo)
	closeEpic(t, s, "task-new")
	run(t, loop, state)

	var found []*tasks.Task
	for deadline := time.Now().Add(10 * time.Second); len(found) == 0 && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		found = sessionsAbout(state, "task-new")
	}
	if len(found) == 0 {
		t.Fatal("timed out waiting for the session after task-new closed")
	}
	session := found[0]
	waitForTurn(t, s, session.ID, "turn-1", "editor") // started, not parked
	if found := sessionsAbout(state, "task-old"); len(found) != 0 {
		t.Errorf("closed before the loop started: %+v", found)
	}
	if found := sessionsAbout(state, "task-new"); len(found) != 1 {
		t.Errorf("one close, one session: %+v", found)
	}
}
