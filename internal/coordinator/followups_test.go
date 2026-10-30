package coordinator

import (
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/settings"
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
// that is done already. It returns the epic's id and the story's.
func newEpic(t *testing.T, s *store.Store, repo string) (id, story string) {
	t.Helper()
	id = mustCreateTask(t, s, events.Proposal{Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Ideas list",
		"description": "Keep the Visionaries' ideas in a list.", "repo": repo, "coordinator": "tech-lead", "kind": "epic", "parked": "backlog"}})
	story = mustCreateTask(t, s, events.Proposal{Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Ideas page",
		"repo": repo, "coordinator": "tech-lead", "parked": "backlog"}})
	mustPropose(t, s, events.Proposal{Stream: story, Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "Built the Ideas page.", "moved": true}})
	mustPropose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.StoryAdded, Data: map[string]any{"story": story, "title": "Ideas page",
		"done": true, "summary": "Built the Ideas page."}})
	mustPropose(t, s, events.Proposal{Stream: story, Actor: you, Type: events.EpicJoined, Data: map[string]any{"epic": id}})
	return id, story
}

// mustCreateTask makes a task as web and the loop do, in the next task
// stream, and returns its id.
func mustCreateTask(t *testing.T, s *store.Store, p events.Proposal) string {
	t.Helper()
	created, rejection, err := s.Create(events.TaskPrefix, p)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	return created.Stream
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
	epic, story := newEpic(t, s, repo)
	closed := closeEpic(t, s, epic)
	loop := newLoop(s, state, t.TempDir())
	loop.Defaults = tasks.Limits{Budget: 5_000_000, MaxTurns: 24}
	followUpsOf(t, s, loop)
	followUpsOf(t, s, loop) // a second pass over the same close, as a re-run would

	found := sessionsAbout(state, epic)
	if len(found) != 1 {
		t.Fatalf("one close, one session: %d", len(found))
	}
	session := found[0]
	if session.AboutClose != closed.Position || session.Coordinator != "editor" || session.Limits() != loop.Defaults ||
		session.Parked != "" || session.Repo != repo || session.Kind != tasks.Story || !session.KindChosen {
		t.Errorf("the session: %+v", session)
	}
	if session.Title != "Follow-ups after Ideas list" {
		t.Errorf("title %q", session.Title)
	}
	for _, want := range []string{epic, "Ideas list", "Keep the Visionaries' ideas in a list.", "Ideas are kept and shown.",
		story + ": Ideas page", "Its summary: Built the Ideas page.", `"epic" is "` + epic + `"`} {
		if !strings.Contains(session.Description, want) {
			t.Errorf("the brief should name %q:\n%s", want, session.Description)
		}
	}
	// The close itself stands as task-16 made it: done, with its summary.
	if task := load(t, s, epic); !task.IsDone || task.Summary != "Ideas are kept and shown." || task.Closing != "" {
		t.Errorf("the closed epic: done %v, summary %q, closing %q", task.IsDone, task.Summary, task.Closing)
	}
}

func TestAReopenedEpicClosedAgainGetsASecondSession(t *testing.T) {
	s, state, repo := withTeams(t, true)
	loop := newLoop(s, state, t.TempDir())
	epic, _ := newEpic(t, s, repo)
	first := closeEpic(t, s, epic)
	followUpsOf(t, s, loop)
	mustPropose(t, s, events.Proposal{Stream: epic, Actor: you, Type: events.TaskReopened, Data: map[string]any{"text": "One more story."}})
	second := closeEpic(t, s, epic)
	followUpsOf(t, s, loop)

	found := sessionsAbout(state, epic)
	if len(found) != 2 || found[0].AboutClose != first.Position || found[1].AboutClose != second.Position {
		t.Fatalf("two closes, two sessions: %+v", found)
	}
	if found[1].Limits() != (tasks.Limits{}) {
		t.Errorf("the server's limits are none, so the session has none: %+v", found[1].Limits())
	}
}

func TestNoSessionWhenTheSettingIsOff(t *testing.T) {
	s, state, repo := withTeams(t, true)
	mustPropose(t, s, events.Proposal{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged,
		Data: map[string]any{settings.VisionariesOnEpicClose: false}})
	epic, _ := newEpic(t, s, repo)
	closeEpic(t, s, epic)
	followUpsOf(t, s, newLoop(s, state, t.TempDir()))

	if found := sessionsAbout(state, epic); len(found) != 0 {
		t.Errorf("the setting is off: %+v", found)
	}
	if task := load(t, s, epic); task.Skipped != 0 {
		t.Errorf("turned off, nothing is said either: skipped %d", task.Skipped)
	}
}

func TestAnEpicMovedToDoneGetsNoSession(t *testing.T) {
	s, state, repo := withTeams(t, true)
	epic, _ := newEpic(t, s, repo)
	mustPropose(t, s, events.Proposal{Stream: epic, Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "", "moved": true}})
	followUpsOf(t, s, newLoop(s, state, t.TempDir()))

	if found := sessionsAbout(state, epic); len(found) != 0 {
		t.Errorf("a move to Done isn't a close: %+v", found)
	}
}

// The session goes to a team made from the Visionaries template, or from a
// template of yours based on it, live or removed, whatever it is called,
// never to one you made and called Visionaries; an old log's
// Visionaries, from before agents recorded their template, still get it.
func TestTheVisionariesSessionGoesByTemplate(t *testing.T) {
	visionaries, _ := agents.TemplateNamed("Visionaries")
	handMade := []agents.Agent{{Name: "dreamer-lead", Team: "Visionaries", Coordinates: true, Engine: "claude", Prompt: "Dream."},
		{Name: "dreamer", Team: "Visionaries", Engine: "claude", Prompt: "Dream."}}
	dreams := agents.UserTemplate{ID: "dreams", Team: "Dreams", BasedOn: "visionaries"}
	for _, agent := range visionaries.Prompted() {
		agent.Team, agent.Template = "", "" // adding the template sets them
		dreams.Agents = append(dreams.Agents, agent)
	}
	cases := []struct {
		name        string
		add         func(roster agents.Roster) []events.Proposal
		coordinator string // "" for no session
		team        string // the coordinator's
	}{
		{"a second team from the template", func(roster agents.Roster) []events.Proposal {
			var ps []events.Proposal
			for _, agent := range handMade {
				ps = append(ps, agent.Proposal(you, false))
				roster = append(roster, agent)
			}
			return append(ps, agents.AddTeam(visionaries, roster, you)...)
		}, "visionary-lead", "Visionaries 2"},
		{"two teams from the template: the first", func(roster agents.Roster) []events.Proposal {
			ps := agents.AddTeam(visionaries, roster, you)
			for _, p := range ps {
				roster = append(roster, agents.FromEvents([]events.Event{{Proposal: p}})...)
			}
			return append(ps, agents.AddTeam(visionaries, roster, you)...)
		}, "visionary-lead", "Visionaries"},
		{"a team you called Visionaries", func(agents.Roster) []events.Proposal {
			var ps []events.Proposal
			for _, agent := range handMade {
				ps = append(ps, agent.Proposal(you, false))
			}
			return ps
		}, "", ""},
		{"a team from your template based on the Visionaries", func(roster agents.Roster) []events.Proposal {
			return append([]events.Proposal{dreams.Save(you)}, agents.AddTeam(dreams.Template(), roster, you)...)
		}, "visionary-lead", "Dreams"},
		{"a team from your removed template based on the Visionaries", func(roster agents.Roster) []events.Proposal {
			ps := append([]events.Proposal{dreams.Save(you)}, agents.AddTeam(dreams.Template(), roster, you)...)
			return append(ps, agents.RemoveTemplate(dreams.ID, you))
		}, "visionary-lead", "Dreams"},
		{"a team from your template based on nothing", func(roster agents.Roster) []events.Proposal {
			plain := dreams
			plain.BasedOn = ""
			return append([]events.Proposal{plain.Save(you)}, agents.AddTeam(plain.Template(), roster, you)...)
		}, "", ""},
		{"an old log", func(roster agents.Roster) []events.Proposal {
			ps := agents.AddTeam(visionaries, roster, you)
			for _, p := range ps {
				delete(p.Data, "template") // as Agentry wrote it before the field
			}
			return ps
		}, "visionary-lead", "Visionaries"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, state, repo := withTeams(t, false)
			for _, p := range c.add(state.Roster()) {
				mustPropose(t, s, p)
			}
			epic, _ := newEpic(t, s, repo)
			closeEpic(t, s, epic)
			followUpsOf(t, s, newLoop(s, state, t.TempDir()))

			found := sessionsAbout(state, epic)
			switch {
			case c.coordinator == "" && len(found) != 0:
				t.Errorf("no team from the template, no session: %+v", found[0])
			case c.coordinator != "" && (len(found) != 1 || found[0].Coordinator != c.coordinator):
				t.Errorf("want one session for %s: %+v", c.coordinator, found)
			}
			if lead, _ := state.Roster().Get(c.coordinator); lead.Team != c.team {
				t.Errorf("%s is of team %q, want %q", c.coordinator, lead.Team, c.team)
			}
		})
	}
}

// Without a Visionaries team the epic says how to add one, once per close.
func TestWithoutVisionariesTheEpicSaysHowToAddThem(t *testing.T) {
	s, state, repo := withTeams(t, false)
	epic, _ := newEpic(t, s, repo)
	closed := closeEpic(t, s, epic)
	loop := newLoop(s, state, t.TempDir())
	followUpsOf(t, s, loop)
	followUpsOf(t, s, loop)

	if found := sessionsAbout(state, epic); len(found) != 0 {
		t.Errorf("no Visionaries team, no session: %+v", found)
	}
	task := load(t, s, epic)
	if last := task.Trail[len(task.Trail)-1]; task.Skipped != closed.Position || last.From != tasks.Agentry || last.To != tasks.You ||
		!strings.Contains(last.Text, "on the Agents page, Add a team, Visionaries") || !strings.Contains(last.Text, "turn these sessions off") {
		t.Errorf("the epic should say how to add the team: skipped %d, last step %+v", task.Skipped, last)
	}
	notes := 0
	for _, e := range mustStream(t, s, epic) {
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
	older, _ := newEpic(t, s, repo)
	closeEpic(t, s, older)
	loop := newLoop(s, state, t.TempDir())
	newer, _ := newEpic(t, s, repo)
	closeEpic(t, s, newer)
	run(t, loop, state)

	var found []*tasks.Task
	for deadline := time.Now().Add(10 * time.Second); len(found) == 0 && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		found = sessionsAbout(state, newer)
	}
	if len(found) == 0 {
		t.Fatal("timed out waiting for the session after the newer epic closed")
	}
	session := found[0]
	waitForTurn(t, s, session.ID, "turn-1", "editor") // started, not parked
	if found := sessionsAbout(state, older); len(found) != 0 {
		t.Errorf("closed before the loop started: %+v", found)
	}
	if found := sessionsAbout(state, newer); len(found) != 1 {
		t.Errorf("one close, one session: %+v", found)
	}
}
