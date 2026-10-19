package ideas_test

import (
	"encoding/json"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/settings"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
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
		want      string // "" for agreed
	}{
		{"every viewpoint backs it", four, "Visionaries", all, nil, ""},
		{"in another order and case, past the team", four, "Visionaries", []string{"Skeptic", "technologist ", "user-advocate", "product-strategist", "engineer"}, nil, ""},
		{"one doubts it", four, "Visionaries", all, []string{"skeptic"}, "doubted by skeptic"},
		{"one doubts it as well as backing it", four, "Visionaries", all, []string{"technologist"}, "doubted by technologist"},
		{"one didn't weigh in", four, "Visionaries", all[:3], nil, "not backed by skeptic"},
		{"nobody weighed in", four, "Visionaries", nil, nil, "not backed by product-strategist, user-advocate, technologist, skeptic"},
		{"no such team", four, "Dreamers", all, nil, "the Dreamers team has no viewpoints"},
		{"a team of only its coordinator", team("Visionaries"), "Visionaries", all, nil, "the Visionaries team has no viewpoints"},
		{"a team with other viewpoints", team("Visionaries", "dreamer", "builder"), "Visionaries", []string{"builder", "dreamer"}, nil, ""},
		{"a team with other viewpoints, one missing", team("Visionaries", "dreamer", "builder"), "Visionaries", all, nil, "not backed by dreamer, builder"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			idea := &ideas.Idea{BackedBy: c.backedBy, DoubtedBy: c.doubtedBy}
			for _, rule := range []string{settings.EveryoneBacked, "a rule from a later version"} {
				if got := ideas.Disagreed(idea, c.roster, c.team, rule); got != c.want {
					t.Errorf("%s: got %q, want %q", rule, got, c.want)
				}
			}
		})
	}
}

func TestAnIdeaNobodyDoubtedIsAgreedWithTwoBackers(t *testing.T) {
	four := append(team("Visionaries", "product-strategist", "user-advocate", "technologist", "skeptic"),
		agents.Agent{Name: "engineer"}, agents.Agent{Name: "tech-lead", Coordinates: true})
	cases := []struct {
		name      string
		backedBy  []string
		doubtedBy []string
		want      string // "" for agreed
	}{
		{"two backers", []string{"skeptic", "technologist"}, nil, ""},
		{"every viewpoint", []string{"product-strategist", "user-advocate", "technologist", "skeptic"}, nil, ""},
		{"two, in another case and spaced", []string{" Skeptic", "TECHNOLOGIST "}, []string{}, ""},
		{"one backer", []string{"skeptic"}, nil, "only one backer"},
		{"no backers", nil, nil, "no backers"},
		{"one backer named twice", []string{"skeptic", "Skeptic "}, nil, "only one backer"},
		{"one backer and the lead", []string{"skeptic", "lead"}, nil, "only one backer"},
		{"one backer and agents of other teams", []string{"skeptic", "engineer", "tech-lead", "stranger"}, nil, "only one backer"},
		{"two backers and a doubter", []string{"skeptic", "technologist"}, []string{"user-advocate"}, "doubted by user-advocate"},
		{"every viewpoint and a doubter", []string{"product-strategist", "user-advocate", "technologist", "skeptic"}, []string{"skeptic"}, "doubted by skeptic"},
		{"doubted by two, one an outsider", []string{"skeptic", "technologist"}, []string{"skeptic", " stranger "}, "doubted by skeptic, stranger"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			idea := &ideas.Idea{BackedBy: c.backedBy, DoubtedBy: c.doubtedBy}
			if got := ideas.Disagreed(idea, four, "Visionaries", settings.NobodyDoubted); got != c.want {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
	if got := ideas.Disagreed(&ideas.Idea{BackedBy: []string{"skeptic", "technologist"}}, team("Visionaries"), "Visionaries", settings.NobodyDoubted); got == "" {
		t.Error("a team with no viewpoints agrees on nothing")
	}
}

func TestAgentryAcceptsNoIdeaYouHadBefore(t *testing.T) {
	at := func(minute int) time.Time { return time.Date(2026, 10, 19, 1, minute, 0, 0, time.UTC) }
	idea := &ideas.Idea{ID: "idea-9", Title: "Search the board", Repo: "/r", State: ideas.Open, CreatedAt: at(9)}
	other := func(id, title, repo string, state ideas.State, minute int) *ideas.Idea {
		return &ideas.Idea{ID: id, Title: title, Repo: repo, State: state, CreatedAt: at(minute)}
	}
	accepted := other("idea-1", "Search the board", "/r", ideas.Accepted, 1)
	accepted.Became = "task-64"
	// You dismissed a later idea as it repeats this one: this one stays the original.
	repeatsIt := other("idea-12", "Search the board", "/r", ideas.Dismissed, 12)
	repeatsIt.Repeats = "idea-9"
	cases := []struct {
		name string
		idea *ideas.Idea
		list []*ideas.Idea
		want string // "" for none
	}{
		{"new", idea, []*ideas.Idea{idea, other("idea-1", "Dark mode", "/r", ideas.Dismissed, 1)}, ""},
		{"dismissed before", idea, []*ideas.Idea{other("idea-1", " search the Board", "/r", ideas.Dismissed, 1)}, "duplicate of idea-1"},
		{"dismissed after", idea, []*ideas.Idea{other("idea-12", "Search the board", "/r", ideas.Dismissed, 12)}, ""},
		{"dismissed after as it repeats this one", idea, []*ideas.Idea{repeatsIt}, ""},
		{"accepted after", idea, []*ideas.Idea{other("idea-12", "Search the board", "/r", ideas.Accepted, 12)}, "duplicate of idea-12"},
		{"accepted before", idea, []*ideas.Idea{accepted}, "duplicate of idea-1 (task-64)"},
		{"accepted, the title reworded", idea, []*ideas.Idea{other("idea-1", "Search on the board!", "/r", ideas.Accepted, 1)}, "duplicate of idea-1"},
		{"open before", idea, []*ideas.Idea{other("idea-1", "Search the board", "/r", ideas.Open, 1)}, "duplicate of idea-1"},
		{"open after: this one is the original", idea, []*ideas.Idea{other("idea-12", "Search the board", "/r", ideas.Open, 12)}, ""},
		{"open in the same instant, a lower id", idea, []*ideas.Idea{other("idea-8", "Search the board", "/r", ideas.Open, 9)}, "duplicate of idea-8"},
		{"open in the same instant, a higher id", idea, []*ideas.Idea{other("idea-10", "Search the board", "/r", ideas.Open, 9)}, ""},
		{"the earliest of several", idea, []*ideas.Idea{other("idea-5", "Search the board", "/r", ideas.Dismissed, 5),
			other("idea-3", "search the board", "/r", ideas.Open, 3), other("idea-12", "Search the board", "/r", ideas.Accepted, 12)}, "duplicate of idea-3"},
		{"dismissed in another repo", idea, []*ideas.Idea{other("idea-1", "Search the board", "/elsewhere", ideas.Dismissed, 1)}, ""},
		{"another feature", idea, []*ideas.Idea{other("idea-1", "Search the ideas", "/r", ideas.Accepted, 1)}, ""},
		{"said to repeat one", &ideas.Idea{ID: "idea-9", Title: "Find tasks fast", Repo: "/r", State: ideas.Open, Repeats: "idea-1", CreatedAt: at(9)},
			[]*ideas.Idea{accepted}, "duplicate of idea-1 (task-64)"},
		{"said to repeat a later open one", &ideas.Idea{ID: "idea-9", Title: "Find tasks fast", Repo: "/r", State: ideas.Open, Repeats: "idea-12", CreatedAt: at(9)},
			[]*ideas.Idea{other("idea-12", "Other", "/r", ideas.Open, 12)}, "duplicate of idea-12"},
		{"said to repeat one not in the list", &ideas.Idea{ID: "idea-9", Title: "Find tasks fast", Repo: "/r", State: ideas.Open, Repeats: "idea-2", CreatedAt: at(9)},
			nil, "duplicate of idea-2"},
		{"not open", other("idea-9", "Search the board", "/r", ideas.Dismissed, 9), nil, "it was dismissed already"},
	}
	agreed := team("Visionaries", "dreamer", "builder")
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			c.idea.BackedBy = []string{"dreamer", "builder"}
			for _, rule := range settings.AutoAcceptRules {
				if got := ideas.NotAutoAccepted(c.idea, append(c.list, c.idea), agreed, rule); got != c.want {
					t.Errorf("%s: got %q, want %q", rule, got, c.want)
				}
			}
		})
	}
}

// The ideas the Visionaries raised again under new titles, kept in the
// log, beside ideas about other features of the same repos.
func TestAnIdeaRaisedAgainInOtherWordsIsADuplicate(t *testing.T) {
	data, err := os.ReadFile("testdata/repeated-ideas.json")
	if err != nil {
		t.Fatal(err)
	}
	var byID map[string]*ideas.Idea
	if err := json.Unmarshal(data, &byID); err != nil {
		t.Fatal(err)
	}
	repeated := map[[2]string]bool{{"idea-11", "idea-21"}: true, {"idea-12", "idea-22"}: true, {"idea-16", "idea-26"}: true,
		{"idea-17", "idea-27"}: true, {"idea-18", "idea-28"}: true}
	ids := slices.Sorted(maps.Keys(byID))
	for i, first := range ids {
		for _, second := range ids[i+1:] {
			a, b := byID[first], byID[second]
			a.ID, a.Repo, a.State, b.ID, b.Repo, b.State = first, "/r", ideas.Accepted, second, "/r", ideas.Open
			got := ideas.Duplicate(b, []*ideas.Idea{a}) != nil
			if want := repeated[[2]string{first, second}]; got != want {
				t.Errorf("%s %q and %s %q: duplicate %v, want %v (they share %.2f)", first, a.Title, second, b.Title, got, want, ideas.Similarity(a, b))
			}
		}
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

// A task from an idea takes the server's limits, unlimited ones too, whoever
// accepts it; one from an idea that follows up an epic takes the epic's, as
// the epic's stories do.
func TestATaskFromAnIdeaTakesTheDefaultLimitsOrItsEpics(t *testing.T) {
	for _, defaults := range []tasks.Limits{{}, {Budget: 5_000_000, MaxTurns: 24}} {
		for _, by := range []events.Actor{you, agentry} {
			s, state, repo := openSeeded(t)
			turnOn(t, s, state)
			mustPropose(t, s, events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{
				"title": "Improve Portside", "repo": repo, "coordinator": "tech-lead", "kind": "epic", "budget": 800, "maxTurns": 6}})
			for _, c := range []struct {
				epic string
				want tasks.Limits
			}{{"", defaults}, {"task-2", tasks.Limits{Budget: 800, MaxTurns: 6}}} {
				id := suggestAgreed(t, s, repo, map[string]any{"epic": c.epic, "title": "Follow " + c.epic})
				got, err := ideas.Accept(s, state, id, ideas.Choice{Defaults: defaults, By: by})
				if err != nil || !got.Accepted {
					t.Fatalf("accept by %s: %+v %v", by.ID, got, err)
				}
				if limits := loadTask(t, s, got.Task).Limits(); limits != c.want {
					t.Errorf("defaults %+v, by %s, epic %q: limits %+v, want %+v", defaults, by.ID, c.epic, limits, c.want)
				}
			}
		}
	}
}

// A budget you choose as you accept replaces only the budget the task's
// limits give it: its turns are still the server's, or its epic's.
func TestTheBudgetYouChooseKeepsTheTasksTurns(t *testing.T) {
	s, state, repo := openSeeded(t)
	mustPropose(t, s, events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": "Improve Portside", "repo": repo, "coordinator": "tech-lead", "kind": "epic", "budget": 800, "maxTurns": 6}})
	defaults := tasks.Limits{Budget: 5_000_000, MaxTurns: 24}
	for _, c := range []struct {
		title  string // each its own, so none looks like the task made before it
		epic   string
		budget int
		want   tasks.Limits
	}{
		{"Retry webhooks", "", 700, tasks.Limits{Budget: 700, MaxTurns: 24}},
		{"Dark sidebar", "", 0, tasks.Limits{Budget: 0, MaxTurns: 24}},
		{"Faster boot", "task-2", 700, tasks.Limits{Budget: 700, MaxTurns: 6}},
	} {
		id := suggest(t, s, repo, map[string]any{"epic": c.epic, "title": c.title})
		got, err := ideas.Accept(s, state, id, ideas.Choice{Defaults: defaults, Budget: &c.budget, Start: true})
		if err != nil || !got.Accepted {
			t.Fatalf("accept: %+v %v", got, err)
		}
		stream, err := s.Stream(got.Task)
		if err != nil || stream[0].Data["budget"] != float64(c.want.Budget) || stream[0].Data["maxTurns"] != float64(c.want.MaxTurns) {
			t.Errorf("epic %q, budget %d: task.created %v (%v), want %+v", c.epic, c.budget, stream[0].Data, err, c.want)
		}
		if limits := loadTask(t, s, got.Task).Limits(); limits != c.want {
			t.Errorf("epic %q, budget %d: limits %+v, want %+v", c.epic, c.budget, limits, c.want)
		}
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
	if err != nil || got != (ideas.Acceptance{Task: "task-2", Column: "backlog", Accepted: true}) {
		t.Fatalf("accept: %+v %v", got, err)
	}
	idea := load(t, state, id)
	if idea.State != ideas.Accepted || !idea.AutoAccepted || idea.Note != "auto-accepted: all Visionaries agreed" {
		t.Errorf("idea: %s, auto %v, note %q", idea.State, idea.AutoAccepted, idea.Note)
	}
	stream, _ := s.Stream("task-2")
	if stream[0].Actor != agentry || loadTask(t, s, "task-2").Parked != "backlog" {
		t.Errorf("the task should be Agentry's, in the Backlog: %v %q", stream[0].Actor, loadTask(t, s, "task-2").Parked)
	}

	mine := suggest(t, s, repo, map[string]any{"title": "Dark mode"})
	if got, _ := ideas.Accept(s, state, mine, ideas.Choice{}); !got.Accepted || load(t, state, mine).AutoAccepted {
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
	accept := events.Proposal{Stream: off, Actor: agentry, Type: ideas.IdeaAccepted, Data: map[string]any{"task": "task-2"}}
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

// The rules refuse Agentry's accept of an idea that repeats another, even
// one the Visionaries all agreed on, and under nobodyDoubted they let it
// accept an idea two viewpoints backed but not one only one backed.
func TestTheRulesRefuseAgentryADuplicate(t *testing.T) {
	s, state, repo := openSeeded(t)
	turnOn(t, s, state)
	mustPropose(t, s, events.Proposal{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged,
		Data: map[string]any{settings.AutoAcceptRule: settings.NobodyDoubted}})
	first := suggestAgreed(t, s, repo, map[string]any{"title": "Search the board"})
	if got, _ := ideas.Accept(s, state, first, ideas.Choice{}); !got.Accepted {
		t.Fatalf("accept: %+v", got)
	}
	accepts := func(id string) *events.Rejection {
		t.Helper()
		task, rejection, err := s.Create(events.TaskPrefix, events.Proposal{Actor: you, Type: events.TaskCreated,
			Data: map[string]any{"title": "From " + id, "repo": repo, "coordinator": "tech-lead", "idea": id}})
		if rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
		_, rejection, err = s.Propose(events.Proposal{Stream: id, Actor: agentry, Type: ideas.IdeaAccepted, Data: map[string]any{"task": task.Stream}})
		if err != nil {
			t.Fatal(err)
		}
		return rejection
	}
	for name, data := range map[string]map[string]any{
		"the same title":    {"title": "search the Board!"},
		"said to repeat it": {"title": "Find tasks fast", "repeats": first},
		"one backer":        {"title": "Alone", "backedBy": []string{"skeptic"}},
	} {
		if rejection := accepts(suggestAgreed(t, s, repo, data)); rejection == nil {
			t.Errorf("%s: Agentry's accept should be refused", name)
		}
	}
	if rejection := accepts(suggestAgreed(t, s, repo, map[string]any{"title": "Dark mode", "backedBy": []string{"skeptic", "technologist"}})); rejection != nil {
		t.Errorf("two backers and no doubt: %v", rejection)
	}
}

// Agentry makes a task from an idea only while it is open, only in your
// Backlog, and starts only a task it made from an idea it accepted, once,
// out of the Backlog into In progress, before it has run; no agent accepts
// an idea.
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
	open := suggestAgreed(t, s, repo, map[string]any{"title": "Still open"})
	next := state.NextTask() // so a task.created is refused for its idea, not its stream
	refused := []struct {
		name string
		p    events.Proposal
	}{
		{"a task from no idea", events.Proposal{Stream: next, Actor: agentry, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": repo, "coordinator": "tech-lead"}}},
		{"a task from an idea accepted already", events.Proposal{Stream: next, Actor: agentry, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": repo, "coordinator": "tech-lead", "idea": auto}}},
		{"a task from no such idea", events.Proposal{Stream: next, Actor: agentry, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": repo, "coordinator": "tech-lead", "idea": "idea-99"}}},
		{"a task from no such idea, by from", events.Proposal{Stream: next, Actor: agentry, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": repo, "coordinator": "tech-lead", "from": "idea-99"}}},
		{"a task whose from and idea differ", events.Proposal{Stream: next, Actor: agentry, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": repo, "coordinator": "tech-lead", "from": open, "idea": "idea-99"}}},
		{"a started task from an open idea", events.Proposal{Stream: next, Actor: agentry, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": repo, "coordinator": "tech-lead", "from": open, "idea": open}}},
		{"a task parked in To do from an open idea", events.Proposal{Stream: next, Actor: agentry, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": repo, "coordinator": "tech-lead", "parked": "todo", "from": open, "idea": open}}},
		{"starting the task you accepted", start("task-3")},
		{"starting the Visionaries' task", start("task-1")},
		{"parking its own", events.Proposal{Stream: "task-2", Actor: agentry, Type: events.TaskHeld, Data: map[string]any{"place": "todo"}}},
		{"moving its own to Shipping", held("task-2", "shipping", false)},
		{"moving its own to Review", held("task-2", "review", false)},
		{"sending its own back", held("task-2", "in-progress", true)},
		{"an agent accepting", events.Proposal{Stream: suggest(t, s, repo, map[string]any{"title": "Third"}), Actor: coordinator,
			Type: ideas.IdeaAccepted, Data: map[string]any{"task": "task-2"}}},
	}
	for _, c := range refused {
		if _, rejection, err := s.Propose(c.p); err != nil || rejection == nil {
			t.Errorf("%s: want refused, got %v %v", c.name, rejection, err)
		}
	}
	mustPropose(t, s, events.Proposal{Stream: next, Actor: agentry, Type: events.TaskCreated,
		Data: map[string]any{"title": "x", "repo": repo, "coordinator": "tech-lead", "parked": "backlog", "from": open}})
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

// A task you parked again before it ever ran stays parked: Agentry starts
// it only once.
func TestAgentryStartsATaskOnce(t *testing.T) {
	s, state, repo := openSeeded(t)
	turnOn(t, s, state)
	if got, _ := ideas.Accept(s, state, suggestAgreed(t, s, repo, nil), ideas.Choice{By: agentry}); got != (ideas.Acceptance{Task: "task-2", Column: "backlog", Accepted: true}) {
		t.Fatalf("accept: %+v", got)
	}
	start := events.Proposal{Stream: "task-2", Actor: agentry, Type: events.TaskHeld,
		Data: map[string]any{"place": "", "note": "", "column": "in-progress", "sentBack": false}}
	mustPropose(t, s, start)
	mustPropose(t, s, events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskHeld, Data: map[string]any{"place": "backlog", "note": ""}})
	if task := loadTask(t, s, "task-2"); len(task.Turns) > 0 || task.Parked != "backlog" {
		t.Fatalf("want parked before it ran: %d turns, parked %q", len(task.Turns), task.Parked)
	}
	if _, rejection, err := s.Propose(start); err != nil || rejection == nil {
		t.Errorf("starting again a task you parked before it ran: want refused, got %v %v", rejection, err)
	}
}

// Agentry starts the task of an idea it accepted only while you have both
// accepting and starting them turned on.
func TestAgentryStartsOnlyWhileTheSettingsSay(t *testing.T) {
	s, state, repo := openSeeded(t)
	turnOn(t, s, state)
	if got, _ := ideas.Accept(s, state, suggestAgreed(t, s, repo, nil), ideas.Choice{By: agentry}); got != (ideas.Acceptance{Task: "task-2", Column: "backlog", Accepted: true}) {
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
	if got, _ := ideas.Accept(s, state, id, ideas.Choice{By: agentry}); got != (ideas.Acceptance{Task: "task-2", Column: "backlog", Accepted: true}) {
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
