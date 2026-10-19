package ideas_test

import (
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

var (
	you         = events.Actor{Kind: events.Human, ID: "you"}
	agentry     = events.Actor{Kind: events.System, ID: "agentry"}
	coordinator = events.Actor{Kind: events.Agent, ID: "tech-lead"}
)

// openSeeded is a store and its state with the rules, the default agents
// and task-1, the Visionaries session the ideas come from, in repo.
func openSeeded(t *testing.T) (s *store.Store, state *current.State, repo string) {
	t.Helper()
	s, state = storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	repo = gitRepo(t)
	for _, p := range append(agents.SeedProposals(nil), events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Think up features", "repo": repo, "coordinator": "tech-lead"}}) {
		mustPropose(t, s, p)
	}
	return s, state, repo
}

func gitRepo(t *testing.T) string {
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"commit", "-q", "--allow-empty", "-m", "first"}} {
		command := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@e"}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, output)
		}
	}
	return repo
}

func mustPropose(t *testing.T, s *store.Store, p events.Proposal) {
	t.Helper()
	if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
		t.Fatalf("%s: %v %v", p.Type, rejection, err)
	}
}

// suggest makes an idea about repo, with data's fields over the usual ones.
func suggest(t *testing.T, s *store.Store, repo string, data map[string]any) string {
	t.Helper()
	full := map[string]any{"title": "Search the board", "problem": "Finding a task takes scrolling", "proposal": "A search box",
		"whyNow": "The board has 200 tasks", "size": "M", "risks": "Slow on big logs", "backedBy": []string{"dreamer", "builder"},
		"doubtedBy": []string{"skeptic"}, "repo": repo, "task": "task-1", "epic": "task-9", "kind": ""}
	for field, value := range data {
		full[field] = value
	}
	created, rejection, err := s.Create("idea-", events.Proposal{Actor: agentry, Type: events.IdeaSuggested, Data: full})
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	return created.Stream
}

func load(t *testing.T, state *current.State, id string) *ideas.Idea {
	t.Helper()
	idea, err := state.Idea(id)
	if err != nil || idea == nil {
		t.Fatalf("loading %s: %v %v", id, idea, err)
	}
	return idea
}

func loadTask(t *testing.T, s *store.Store, id string) *tasks.Task {
	t.Helper()
	stream, err := s.Stream(id)
	if err != nil {
		t.Fatal(err)
	}
	return tasks.FromEvents(stream)
}

func TestASuggestedIdeaKeepsEveryField(t *testing.T) {
	s, state, repo := openSeeded(t)
	idea := load(t, state, suggest(t, s, repo, map[string]any{"kind": "epic"}))

	got := []string{idea.ID, idea.Title, idea.Problem, idea.Proposal, idea.WhyNow, idea.Size, idea.Risks,
		strings.Join(idea.BackedBy, ","), strings.Join(idea.DoubtedBy, ","), idea.Task, idea.Repo, idea.Epic, string(idea.Kind), string(idea.State)}
	want := []string{"idea-1", "Search the board", "Finding a task takes scrolling", "A search box", "The board has 200 tasks", "M",
		"Slow on big logs", "dreamer,builder", "skeptic", "task-1", repo, "task-9", "epic", "open"}
	if !slices.Equal(got, want) {
		t.Errorf("idea:\n got %q\nwant %q", got, want)
	}
	if idea.CreatedAt.IsZero() || idea.Note != "" || idea.Became != "" {
		t.Errorf("created %v, note %q, became %q", idea.CreatedAt, idea.Note, idea.Became)
	}
	if ids := state.TaskIDs(); !slices.Equal(ids, []string{"task-1"}) {
		t.Errorf("an idea showed up as a task: %v", ids)
	}
}

func TestAcceptMakesABacklogTaskInTheIdeasRepo(t *testing.T) {
	cases := []struct {
		name      string
		suggested string // the idea's kind
		chosen    string // yours
		want      tasks.Kind
	}{
		{"a task, as suggested", "", "", tasks.Story},
		{"an epic, as suggested", "epic", "", tasks.Epic},
		{"an epic, though a task was suggested", "", "epic", tasks.Epic},
		{"a task, though an epic was suggested", "epic", "task", tasks.Story},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s, state, repo := openSeeded(t)
			id := suggest(t, s, repo, map[string]any{"kind": c.suggested})

			got, err := ideas.Accept(s, state, id, ideas.Choice{Kind: c.chosen, Note: "Yes, soon", Defaults: tasks.Limits{Budget: 500, MaxTurns: 7}})
			if err != nil || got != (ideas.Acceptance{Task: "task-2", Accepted: true}) {
				t.Fatalf("accept: %+v %v", got, err)
			}
			task := loadTask(t, s, "task-2")
			if task.Kind != c.want || !task.KindChosen || task.Parked != "backlog" || task.Status() != tasks.Held ||
				task.Repo != repo || task.Coordinator != "tech-lead" || task.Budget != 500 || task.MaxTurns != 7 || task.Idea != id || task.Title != "Search the board" {
				t.Errorf("task: kind %q chosen %v parked %q status %s repo %q coordinator %q budget %d turns %d idea %q title %q",
					task.Kind, task.KindChosen, task.Parked, task.Status(), task.Repo, task.Coordinator, task.Budget, task.MaxTurns, task.Idea, task.Title)
			}
			for _, link := range []string{id, `"Search the board"`, "A search box", "Yes, soon"} {
				if !strings.Contains(task.Description, link) {
					t.Errorf("description has no %s:\n%s", link, task.Description)
				}
			}
			if idea := load(t, state, id); idea.State != ideas.Accepted || idea.Became != "task-2" || idea.Note != "Yes, soon" {
				t.Errorf("idea: %s, became %q, note %q", idea.State, idea.Became, idea.Note)
			}
		})
	}
}

func TestDismissKeepsYourNote(t *testing.T) {
	s, state, repo := openSeeded(t)
	kept, dropped := suggest(t, s, repo, nil), suggest(t, s, repo, map[string]any{"title": "Dark mode"})
	other := suggest(t, s, gitRepo(t), map[string]any{"title": "Elsewhere"})
	for _, id := range []string{dropped, other} {
		mustPropose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.IdeaDismissed, Data: map[string]any{"note": "Not now"}})
	}

	if idea := load(t, state, dropped); idea.State != ideas.Dismissed || idea.Note != "Not now" {
		t.Errorf("dismissed: %s %q", idea.State, idea.Note)
	}
	list := ideas.List(state, repo)
	if len(list) != 2 || list[0].ID != dropped || list[1].ID != kept {
		t.Fatalf("the repo's ideas, newest first: %v", list)
	}
	if dismissed := ideas.InState(list, ideas.Dismissed); len(dismissed) != 1 || dismissed[0].Title != "Dark mode" || dismissed[0].Note != "Not now" {
		t.Errorf("dismissals: %v", dismissed)
	}
	if all := ideas.List(state, ""); len(all) != 3 {
		t.Errorf("every repo's ideas: %d", len(all))
	}
}

func TestAnIdeaIsAcceptedOrDismissedOnce(t *testing.T) {
	s, state, repo := openSeeded(t)
	accepted, dismissed := suggest(t, s, repo, nil), suggest(t, s, repo, nil)
	if got, err := ideas.Accept(s, state, accepted, ideas.Choice{}); err != nil || !got.Accepted {
		t.Fatal(got, err)
	}
	mustPropose(t, s, events.Proposal{Stream: dismissed, Actor: you, Type: events.IdeaDismissed, Data: map[string]any{"note": "No"}})

	for _, id := range []string{accepted, dismissed} {
		if got, err := ideas.Accept(s, state, id, ideas.Choice{}); err != nil || got.Task != "" || got.Accepted || got.Refused == "" {
			t.Errorf("accepting %s again: %+v %v", id, got, err)
		}
	}
	// The rules refuse it too, not only Accept: here with a task made from
	// the idea, so only the idea's state stands in the way.
	again := events.Proposal{Stream: accepted, Actor: you, Type: events.IdeaAccepted, Data: map[string]any{"task": "task-2"}}
	for _, p := range []events.Proposal{
		again,
		{Stream: accepted, Actor: you, Type: events.IdeaDismissed, Data: map[string]any{"note": "Changed my mind"}},
		{Stream: dismissed, Actor: you, Type: events.IdeaDismissed, Data: map[string]any{"note": "Still no"}},
		{Stream: dismissed, Actor: you, Type: events.IdeaAccepted, Data: map[string]any{"task": "task-2"}},
	} {
		if _, rejection, err := s.Propose(p); err != nil || rejection == nil || !strings.Contains(rejection.Reason, "already") {
			t.Errorf("%s on %s: %v %v", p.Type, p.Stream, rejection, err)
		}
	}
	if n := len(state.TaskIDs()); n != 2 {
		t.Errorf("%d tasks, want task-1 and the one accepted", n)
	}
}

// A task made from the idea but never marked, as after a crash between the
// two writes, is the one Accept marks; no second task is made.
func TestAcceptReusesATaskAlreadyMadeFromTheIdea(t *testing.T) {
	s, state, repo := openSeeded(t)
	id := suggest(t, s, repo, nil)
	mustPropose(t, s, events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Search the board", "repo": repo, "coordinator": "tech-lead", "parked": "backlog", "idea": id}})

	got, err := ideas.Accept(s, state, id, ideas.Choice{Note: "At last"})
	if err != nil || got != (ideas.Acceptance{Task: "task-2", Accepted: true}) {
		t.Fatalf("accept: %+v %v", got, err)
	}
	if n := len(state.TaskIDs()); n != 2 {
		t.Errorf("%d tasks, want task-1 and task-2", n)
	}
	if idea := load(t, state, id); idea.State != ideas.Accepted || idea.Became != "task-2" {
		t.Errorf("idea: %s, became %q", idea.State, idea.Became)
	}
}

// A broken task made from the idea can't be marked, so Accept refuses, but
// makes no second task either; a broken task made from nothing is skipped.
func TestAcceptRefusesWhenTheTaskMadeFromTheIdeaIsBroken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	repo := gitRepo(t)
	// The rules would refuse a worktree.moved before the worktree is
	// ready, so the log is written without them, then opened with them.
	unchecked, err := store.Open(path, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range append(agents.SeedProposals(nil), events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Think up features", "repo": repo, "coordinator": "tech-lead"}}) {
		mustPropose(t, unchecked, p)
	}
	fromBroken, fresh := suggest(t, unchecked, repo, nil), suggest(t, unchecked, repo, nil)
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	for stream, idea := range map[string]string{"task-2": fromBroken, "task-3": ""} {
		mustPropose(t, unchecked, events.Proposal{Stream: stream, Actor: you, Type: events.TaskCreated,
			Data: map[string]any{"title": "Search the board", "repo": repo, "coordinator": "tech-lead", "parked": "backlog", "idea": idea}})
		mustPropose(t, unchecked, events.Proposal{Stream: stream, Actor: worker, Type: events.WorktreeMoved,
			Data: map[string]any{"worker": "local", "path": "/w", "port": 7901}})
	}
	unchecked.Close()
	s, state := storetest.Open(t, path)
	if state.Broken("task-2") == nil || state.Broken("task-3") == nil {
		t.Fatal("task-2 and task-3 should be broken")
	}

	got, err := ideas.Accept(s, state, fromBroken, ideas.Choice{})
	if err != nil || got.Task != "task-2" || got.Accepted || !strings.Contains(got.Refused, "can't be read") {
		t.Errorf("accepting %s: %+v %v", fromBroken, got, err)
	}
	if n := len(state.TaskIDs()); n != 3 {
		t.Errorf("%d tasks, want no task made beside task-1 to task-3", n)
	}
	if idea := load(t, state, fromBroken); idea.State != ideas.Open {
		t.Errorf("%s is %s", fromBroken, idea.State)
	}
	if got, err := ideas.Accept(s, state, fresh, ideas.Choice{}); err != nil || got != (ideas.Acceptance{Task: "task-4", Accepted: true}) {
		t.Errorf("accepting %s past the broken tasks: %+v %v", fresh, got, err)
	}
}

// A kind that is neither a task nor an epic is refused, whether or not a
// task was made from the idea already.
func TestAcceptRefusesAnUnknownKind(t *testing.T) {
	s, state, repo := openSeeded(t)
	fresh, made := suggest(t, s, repo, nil), suggest(t, s, repo, nil)
	mustPropose(t, s, events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Search the board", "repo": repo, "coordinator": "tech-lead", "parked": "backlog", "idea": made}})

	for _, id := range []string{fresh, made} {
		got, err := ideas.Accept(s, state, id, ideas.Choice{Kind: "bogus"})
		if err != nil || got != (ideas.Acceptance{Refused: "an idea becomes a task or an epic, not bogus"}) {
			t.Errorf("accepting %s as bogus: %+v %v", id, got, err)
		}
		if idea := load(t, state, id); idea.State != ideas.Open {
			t.Errorf("%s is %s", id, idea.State)
		}
	}
}

// You dismiss the idea after its task is made but before it is marked: the
// task stays, and Acceptance says so.
func TestAcceptSaysWhenTheTaskWasMadeButTheIdeaNotMarked(t *testing.T) {
	s, state, repo := openSeeded(t)
	id := suggest(t, s, repo, nil)
	ideas.BeforeMarking(t, func() {
		mustPropose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.IdeaDismissed, Data: map[string]any{"note": "No"}})
	})

	got, err := ideas.Accept(s, state, id, ideas.Choice{})
	if err != nil || got.Task != "task-2" || got.Accepted || !strings.Contains(got.Refused, "wasn't marked accepted") {
		t.Errorf("accept: %+v %v", got, err)
	}
	if idea := load(t, state, id); idea.State != ideas.Dismissed {
		t.Errorf("idea is %s", idea.State)
	}
}

// Two Accepts at once, as from a double click, make one task.
func TestAcceptsAtOnceMakeOneTask(t *testing.T) {
	s, state, repo := openSeeded(t)
	id := suggest(t, s, repo, nil)
	results := make(chan ideas.Acceptance, 2)
	for range 2 {
		go func() {
			got, _ := ideas.Accept(s, state, id, ideas.Choice{})
			results <- got
		}()
	}
	if first, second := <-results, <-results; first.Accepted == second.Accepted {
		t.Errorf("want one accepted: %+v, %+v", first, second)
	}
	if n := len(state.TaskIDs()); n != 2 {
		t.Errorf("%d tasks, want task-1 and one made from the idea", n)
	}
}

// Agentry may accept an idea, but only for a task made from it (see
// TestTheRulesKeepAgentryToIdeasItAccepted).
func TestOnlyYouAcceptOrDismiss(t *testing.T) {
	s, state, repo := openSeeded(t)
	id := suggest(t, s, repo, nil)
	for _, by := range []events.Actor{coordinator, agentry} {
		for _, p := range []events.Proposal{
			{Stream: id, Actor: by, Type: events.IdeaAccepted, Data: map[string]any{"task": "task-1"}},
			{Stream: id, Actor: by, Type: events.IdeaDismissed, Data: map[string]any{"note": "No"}},
		} {
			check := "who"
			if by == agentry && p.Type == events.IdeaAccepted {
				check = "order"
			}
			if _, rejection, err := s.Propose(p); err != nil || rejection == nil || rejection.Check != check {
				t.Errorf("%s by %s: %v %v", p.Type, by, rejection, err)
			}
		}
	}
	if idea := load(t, state, id); idea.State != ideas.Open {
		t.Errorf("idea is %s", idea.State)
	}
}

func TestAcceptRefusesAnUnknownRepo(t *testing.T) {
	s, state, _ := openSeeded(t)
	id := suggest(t, s, t.TempDir(), nil) // a folder, but no git repo

	got, err := ideas.Accept(s, state, id, ideas.Choice{})
	if err != nil || got.Task != "" || got.Accepted || !strings.Contains(got.Refused, "isn't a git repository") {
		t.Errorf("accept: %+v %v", got, err)
	}
	if n := len(state.TaskIDs()); n != 1 {
		t.Errorf("%d tasks, want only task-1", n)
	}
	if idea := load(t, state, id); idea.State != ideas.Open {
		t.Errorf("idea is %s", idea.State)
	}
}

func TestAnAcceptedIdeaPointsAtATaskMadeFromIt(t *testing.T) {
	s, _, repo := openSeeded(t)
	id := suggest(t, s, repo, nil)
	for _, task := range []string{"task-1", "task-99"} { // not made from it; not made at all
		_, rejection, err := s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.IdeaAccepted, Data: map[string]any{"task": task}})
		if err != nil || rejection == nil {
			t.Errorf("accepted as %s: %v", task, err)
		}
	}
}

func TestASuggestionComesFromATask(t *testing.T) {
	s, _, repo := openSeeded(t)
	id := suggest(t, s, repo, nil)
	usual := map[string]any{"title": "x", "problem": "p", "proposal": "f", "size": "S", "repo": repo, "task": "task-1"}
	with := func(field string, value any) map[string]any {
		data := map[string]any{}
		for k, v := range usual {
			data[k] = v
		}
		data[field] = value
		return data
	}
	for _, c := range []struct {
		name     string
		stream   string
		by       events.Actor
		data     map[string]any
		accepted bool
	}{
		{"the usual", "idea-2", agentry, usual, true},
		{"twice in one stream", id, agentry, usual, false},
		{"no title", "idea-3", agentry, with("title", " "), false},
		{"an unknown size", "idea-3", agentry, with("size", "XL"), false},
		{"an unknown kind", "idea-3", agentry, with("kind", "story"), false},
		{"from no task", "idea-3", agentry, with("task", "task-99"), false},
		{"by an agent not working on the task", "idea-3", coordinator, usual, false},
	} {
		_, rejection, err := s.Propose(events.Proposal{Stream: c.stream, Actor: c.by, Type: events.IdeaSuggested, Data: c.data})
		if err != nil || (rejection == nil) != c.accepted {
			t.Errorf("%s: %v %v", c.name, rejection, err)
		}
	}
}

// A log from before ideas has none: its tasks build as they did, and the
// next task and the first idea are named as before.
func TestALogWithNoIdeas(t *testing.T) {
	s, state, repo := openSeeded(t)
	before := loadTask(t, s, "task-1")
	if list := ideas.List(state, ""); list != nil {
		t.Errorf("ideas: %v", list)
	}
	if before.Idea != "" || before.Parked != "" || before.Status() != tasks.Working {
		t.Errorf("task-1: idea %q parked %q status %s", before.Idea, before.Parked, before.Status())
	}
	if id := suggest(t, s, repo, nil); id != "idea-1" {
		t.Errorf("first idea is %s", id)
	}
	created, rejection, err := s.Create("task-", events.Proposal{Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Next", "repo": repo, "coordinator": "tech-lead"}})
	if err != nil || rejection != nil || created.Stream != "task-2" {
		t.Errorf("next task: %s %v %v", created.Stream, rejection, err)
	}
}
