package ideas_test

import (
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/settings"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

func team(name string, members ...string) agents.Roster {
	roster := agents.Roster{{Name: "lead", Coordinates: true, Team: name}}
	for _, member := range members {
		roster = append(roster, agents.Agent{Name: member, Team: name})
	}
	return roster
}

func TestAnIdeaIsAgreedWhenEveryViewpointBacksIt(t *testing.T) {
	four := append(team("Visionaries", "product-strategist", "user-advocate", "technologist", "skeptic"),
		agents.Agent{Name: "engineer"}, agents.Agent{Name: "tech-lead", Coordinates: true})
	all := []string{"product-strategist", "user-advocate", "technologist", "skeptic"}
	cases := []struct {
		name      string
		roster    agents.Roster
		team      string
		backedBy  []string
		doubtedBy []string
		want      bool
	}{
		{"every viewpoint backs it", four, "Visionaries", all, nil, true},
		{"in another order and case, past the team", four, "Visionaries", []string{"Skeptic", "technologist ", "user-advocate", "product-strategist", "engineer"}, nil, true},
		{"one doubts it", four, "Visionaries", all, []string{"skeptic"}, false},
		{"one doubts it as well as backing it", four, "Visionaries", all, []string{"technologist"}, false},
		{"one didn't weigh in", four, "Visionaries", all[:3], nil, false},
		{"nobody weighed in", four, "Visionaries", nil, nil, false},
		{"no such team", four, "Dreamers", all, nil, false},
		{"a team of only its coordinator", team("Visionaries"), "Visionaries", all, nil, false},
		{"a team with other viewpoints", team("Visionaries", "dreamer", "builder"), "Visionaries", []string{"builder", "dreamer"}, nil, true},
		{"a team with other viewpoints, one missing", team("Visionaries", "dreamer", "builder"), "Visionaries", all, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			idea := &ideas.Idea{BackedBy: c.backedBy, DoubtedBy: c.doubtedBy}
			if got := ideas.Agreed(idea, c.roster, c.team); got != c.want {
				t.Errorf("agreed: got %v, want %v", got, c.want)
			}
		})
	}
}

func TestAgentryAcceptsNoIdeaYouHadBefore(t *testing.T) {
	idea := &ideas.Idea{ID: "idea-9", Title: "Search the board", Repo: "/r", State: ideas.Open}
	other := func(id, title, repo string, state ideas.State) *ideas.Idea {
		return &ideas.Idea{ID: id, Title: title, Repo: repo, State: state}
	}
	none := func(string) bool { return false }
	cases := []struct {
		name string
		idea *ideas.Idea
		list []*ideas.Idea
		made func(string) bool
		want string // in the reason, "" for none
	}{
		{"new", idea, []*ideas.Idea{idea, other("idea-1", "Dark mode", "/r", ideas.Dismissed)}, none, ""},
		{"not open", other("idea-9", "Search the board", "/r", ideas.Dismissed), nil, none, "dismissed already"},
		{"dismissed before", idea, []*ideas.Idea{other("idea-1", " search the Board", "/r", ideas.Dismissed)}, none, "you dismissed idea-1"},
		{"accepted before", idea, []*ideas.Idea{other("idea-1", "Search the board", "/r", ideas.Accepted)}, none, "idea-1, under the same title, is a task"},
		{"a task made from one still open", idea, []*ideas.Idea{other("idea-1", "Search the board", "/r", ideas.Open)},
			func(id string) bool { return id == "idea-1" }, "idea-1, under the same title, is a task"},
		{"open twice, no task", idea, []*ideas.Idea{other("idea-1", "Search the board", "/r", ideas.Open)}, none, ""},
		{"dismissed in another repo", idea, []*ideas.Idea{other("idea-1", "Search the board", "/elsewhere", ideas.Dismissed)}, none, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := ideas.Repeats(c.idea, c.list, c.made)
			if (c.want == "") != (got == "") || !strings.Contains(got, c.want) {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}

// task-48's shape: the epic's limits are a lead-in line and its bullets,
// between other paragraphs and lists.
const portsideEpic = `An epic: improve the Portside system.

The system: 8 repos, cloned side by side:
- portsidemotors-web: buyer storefront.
- portsidemotors-api: domain API.

House rules: the user's skill "portsidemotors". Read it first.

Hard limits, because these are live services with real users and data:
- Land changes ONLY as merge requests into each repo's dev branch.
  Never push to or merge into beta or prod.
- Never trigger or call deploys.

- Never commit secrets.

The user is asleep and everything is unlimited.`

func TestATaskFromAnIdeaCarriesItsEpicsLimits(t *testing.T) {
	cases := []struct {
		name, description string
		want, not         []string
	}{
		{"a block of limits", portsideEpic,
			[]string{"Limits from task-2 (copied from the epic):\n\nHard limits, because these are live services with real users and data:\n" +
				"- Land changes ONLY as merge requests into each repo's dev branch.\n  Never push to or merge into beta or prod.\n" +
				"- Never trigger or call deploys."},
			[]string{"portsidemotors-web", "House rules", "unlimited", "Never commit secrets"}},
		{"no block: the whole description, cut short", "Keep it small.\n\nNo deploys. " + strings.Repeat("x", 3000),
			[]string{"Limits from task-2 (copied from the epic):\n\nKeep it small.\n\nNo deploys. xxx", "x [...]"}, nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, state, repo := openSeeded(t)
			mustPropose(t, s, events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{
				"title": "Improve Portside", "description": c.description, "repo": repo, "coordinator": "tech-lead", "kind": "epic"}})
			id := suggest(t, s, repo, map[string]any{"epic": "task-2"})
			if got, err := ideas.Accept(s, state, id, ideas.Choice{}); err != nil || !got.Accepted {
				t.Fatalf("accept: %+v %v", got, err)
			}
			description := loadTask(t, s, "task-3").Description
			for _, want := range c.want {
				if !strings.Contains(description, want) {
					t.Errorf("description has no %q:\n%s", want, description)
				}
			}
			for _, not := range c.not {
				if strings.Contains(description, not) {
					t.Errorf("description has %q:\n%s", not, description)
				}
			}
		})
	}
	s, state, repo := openSeeded(t)
	id := suggest(t, s, repo, map[string]any{"epic": ""})
	if got, _ := ideas.Accept(s, state, id, ideas.Choice{}); !got.Accepted || strings.Contains(loadTask(t, s, "task-2").Description, "Limits") {
		t.Errorf("an idea with no epic has no limits: %+v\n%s", got, loadTask(t, s, "task-2").Description)
	}
}

// turnOn adds the default Visionaries team and turns on Agentry accepting,
// for you, the ideas every Visionary agreed on, and starting them.
func turnOn(t *testing.T, s *store.Store, state *current.State) {
	t.Helper()
	visionaries, _ := agents.TemplateNamed(ideas.VisionariesTeam)
	for _, p := range agents.AddTeam(visionaries, state.Roster(), you) {
		mustPropose(t, s, p)
	}
	mustPropose(t, s, events.Proposal{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged,
		Data: map[string]any{settings.AutoAcceptIdeas: true, settings.AutoStartIdeas: true}})
}

// suggestAgreed makes an idea about repo every default Visionary backs and
// none doubts, following up no epic, with data's fields over those.
func suggestAgreed(t *testing.T, s *store.Store, repo string, data map[string]any) string {
	t.Helper()
	full := map[string]any{"backedBy": []string{"product-strategist", "user-advocate", "technologist", "skeptic"},
		"doubtedBy": []string{}, "epic": ""}
	for field, value := range data {
		full[field] = value
	}
	return suggest(t, s, repo, full)
}

func TestAgentryAcceptsAnIdeaAsItself(t *testing.T) {
	s, state, repo := openSeeded(t)
	turnOn(t, s, state)
	id := suggestAgreed(t, s, repo, nil)
	got, err := ideas.Accept(s, state, id, ideas.Choice{Note: "auto-accepted: all Visionaries agreed", By: agentry})
	if err != nil || got != (ideas.Acceptance{Task: "task-2", Accepted: true}) {
		t.Fatalf("accept: %+v %v", got, err)
	}
	idea := load(t, s, id)
	if idea.State != ideas.Accepted || !idea.AutoAccepted || idea.Note != "auto-accepted: all Visionaries agreed" {
		t.Errorf("idea: %s, auto %v, note %q", idea.State, idea.AutoAccepted, idea.Note)
	}
	stream, _ := s.Stream("task-2")
	if stream[0].Actor != agentry || loadTask(t, s, "task-2").Parked != "backlog" {
		t.Errorf("the task should be Agentry's, in the Backlog: %v %q", stream[0].Actor, loadTask(t, s, "task-2").Parked)
	}

	mine := suggest(t, s, repo, map[string]any{"title": "Dark mode"})
	if got, _ := ideas.Accept(s, state, mine, ideas.Choice{}); !got.Accepted || load(t, s, mine).AutoAccepted {
		t.Errorf("an idea you accept isn't Agentry's: %+v", got)
	}
}

// The rules, not only the loop, keep Agentry to the ideas every Visionary
// agreed on, and only while you have that turned on; your accepts are
// yours to make either way.
func TestAgentryAcceptsOnlyWhatTheSettingAndTheVisionariesAllow(t *testing.T) {
	s, state, repo := openSeeded(t)
	off := suggestAgreed(t, s, repo, nil)
	if got, _ := ideas.Accept(s, state, off, ideas.Choice{By: agentry}); got.Task != "" || got.Accepted {
		t.Errorf("with the setting off: %+v", got)
	}
	mustPropose(t, s, events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Search the board", "repo": repo, "coordinator": "tech-lead", "idea": off}})
	accept := events.Proposal{Stream: off, Actor: agentry, Type: events.IdeaAccepted, Data: map[string]any{"task": "task-2"}}
	if _, rejection, err := s.Propose(accept); err != nil || rejection == nil {
		t.Errorf("marking it accepted with the setting off: want refused, got %v %v", rejection, err)
	}

	turnOn(t, s, state)
	for name, data := range map[string]map[string]any{
		"a doubter":         {"title": "Doubted", "doubtedBy": []string{"skeptic"}},
		"a missing backer":  {"title": "Short", "backedBy": []string{"product-strategist", "user-advocate", "technologist"}},
		"the usual dissent": {"title": "Usual", "backedBy": []string{"dreamer", "builder"}, "doubtedBy": []string{"skeptic"}},
	} {
		id := suggestAgreed(t, s, repo, data)
		if got, _ := ideas.Accept(s, state, id, ideas.Choice{By: agentry}); got.Task != "" || got.Accepted {
			t.Errorf("%s: %+v", name, got)
		}
		if got, _ := ideas.Accept(s, state, id, ideas.Choice{}); !got.Accepted {
			t.Errorf("%s: you may accept it: %+v", name, got)
		}
	}
	if _, rejection, err := s.Propose(accept); err != nil || rejection != nil {
		t.Errorf("marking an agreed idea accepted with the setting on: %v %v", rejection, err)
	}
}

// Agentry makes a task from an idea only while it is open, and starts only
// a task it made from an idea it accepted, once, out of the Backlog into
// In progress, before it has run; no agent accepts an idea.
func TestTheRulesKeepAgentryToIdeasItAccepted(t *testing.T) {
	s, state, repo := openSeeded(t)
	turnOn(t, s, state)
	auto, yours := suggestAgreed(t, s, repo, nil), suggestAgreed(t, s, repo, map[string]any{"title": "Dark mode"})
	if got, _ := ideas.Accept(s, state, auto, ideas.Choice{By: agentry}); !got.Accepted {
		t.Fatalf("accept: %+v", got)
	}
	if got, _ := ideas.Accept(s, state, yours, ideas.Choice{}); !got.Accepted {
		t.Fatalf("accept: %+v", got)
	}
	held := func(task, column string, sentBack bool) events.Proposal {
		return events.Proposal{Stream: task, Actor: agentry, Type: events.TaskHeld,
			Data: map[string]any{"place": "", "note": "", "column": column, "sentBack": sentBack}}
	}
	start := func(task string) events.Proposal { return held(task, "in-progress", false) }
	refused := []struct {
		name string
		p    events.Proposal
	}{
		{"a task from no idea", events.Proposal{Stream: "task-9", Actor: agentry, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": repo, "coordinator": "tech-lead"}}},
		{"a task from an idea accepted already", events.Proposal{Stream: "task-9", Actor: agentry, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": repo, "coordinator": "tech-lead", "idea": auto}}},
		{"a task from no such idea", events.Proposal{Stream: "task-9", Actor: agentry, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": repo, "coordinator": "tech-lead", "idea": "idea-99"}}},
		{"starting the task you accepted", start("task-3")},
		{"starting the Visionaries' task", start("task-1")},
		{"parking its own", events.Proposal{Stream: "task-2", Actor: agentry, Type: events.TaskHeld, Data: map[string]any{"place": "todo"}}},
		{"moving its own to Shipping", held("task-2", "shipping", false)},
		{"moving its own to Review", held("task-2", "review", false)},
		{"sending its own back", held("task-2", "in-progress", true)},
		{"an agent accepting", events.Proposal{Stream: suggest(t, s, repo, map[string]any{"title": "Third"}), Actor: coordinator,
			Type: events.IdeaAccepted, Data: map[string]any{"task": "task-2"}}},
	}
	for _, c := range refused {
		if _, rejection, err := s.Propose(c.p); err != nil || rejection == nil {
			t.Errorf("%s: want refused, got %v %v", c.name, rejection, err)
		}
	}
	mustPropose(t, s, start("task-2"))
	if task := loadTask(t, s, "task-2"); task.Parked != "" || task.Column != "in-progress" {
		t.Errorf("started: parked %q column %q", task.Parked, task.Column)
	}

	worker := events.Actor{Kind: events.Worker, ID: "local"}
	for _, p := range []events.Proposal{
		{Stream: "task-2", Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/task-2", "branch": "agentry/task-2", "port": 7901}},
		{Stream: "task-2", Actor: worker, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "hash": "h"}},
		{Stream: "task-2", Actor: worker, Type: events.TurnEnded, Correlation: "turn-1", Data: map[string]any{"agent": "tech-lead", "report": "x"}},
		{Stream: "task-2", Actor: you, Type: events.TaskHeld, Data: map[string]any{"place": "backlog", "note": ""}},
	} {
		mustPropose(t, s, p)
	}
	if _, rejection, err := s.Propose(start("task-2")); err != nil || rejection == nil {
		t.Errorf("starting again a task you parked after it ran: want refused, got %v %v", rejection, err)
	}
}

// Agentry starts the task of an idea it accepted only while you have both
// accepting and starting them turned on.
func TestAgentryStartsOnlyWhileTheSettingsSay(t *testing.T) {
	s, state, repo := openSeeded(t)
	turnOn(t, s, state)
	if got, _ := ideas.Accept(s, state, suggestAgreed(t, s, repo, nil), ideas.Choice{By: agentry}); got != (ideas.Acceptance{Task: "task-2", Accepted: true}) {
		t.Fatalf("accept: %+v", got)
	}
	start := events.Proposal{Stream: "task-2", Actor: agentry, Type: events.TaskHeld,
		Data: map[string]any{"place": "", "note": "", "column": "in-progress", "sentBack": false}}
	for _, off := range []string{settings.AutoStartIdeas, settings.AutoAcceptIdeas} {
		mustPropose(t, s, events.Proposal{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged,
			Data: map[string]any{settings.AutoAcceptIdeas: true, settings.AutoStartIdeas: true, off: false}})
		if _, rejection, err := s.Propose(start); err != nil || rejection == nil {
			t.Errorf("with %s off: want refused, got %v %v", off, rejection, err)
		}
	}
}

// A task you made from an idea is yours: Agentry doesn't start it, even
// when it marked the idea accepted.
func TestAgentryStartsNoTaskYouMade(t *testing.T) {
	s, state, repo := openSeeded(t)
	turnOn(t, s, state)
	id := suggestAgreed(t, s, repo, nil)
	mustPropose(t, s, events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Search the board", "repo": repo, "coordinator": "tech-lead", "idea": id, "parked": "backlog"}})
	if got, _ := ideas.Accept(s, state, id, ideas.Choice{By: agentry}); got != (ideas.Acceptance{Task: "task-2", Accepted: true}) {
		t.Fatalf("accept: %+v", got)
	}
	start := events.Proposal{Stream: "task-2", Actor: agentry, Type: events.TaskHeld,
		Data: map[string]any{"place": "", "note": "", "column": "in-progress", "sentBack": false}}
	if _, rejection, err := s.Propose(start); err != nil || rejection == nil {
		t.Errorf("want refused, got %v %v", rejection, err)
	}
}

// An idea naming an epic that isn't there loses the epic's limits: Agentry
// leaves it for you, and your accept says so in the task.
func TestAnIdeaWhoseEpicIsMissingSaysSo(t *testing.T) {
	s, state, repo := openSeeded(t)
	turnOn(t, s, state)
	id := suggestAgreed(t, s, repo, map[string]any{"epic": "task-9"})
	if got, _ := ideas.Accept(s, state, id, ideas.Choice{By: agentry}); got.Task != "" || got.Accepted || !strings.Contains(got.Refused, "task-9") {
		t.Errorf("Agentry's accept: %+v", got)
	}
	got, err := ideas.Accept(s, state, id, ideas.Choice{})
	if err != nil || !got.Accepted {
		t.Fatalf("your accept: %+v %v", got, err)
	}
	const want = "The epic task-9 this follows up wasn't found, so its limits weren't copied."
	if description := loadTask(t, s, got.Task).Description; !strings.Contains(description, want) {
		t.Errorf("description has no %q:\n%s", want, description)
	}
}
