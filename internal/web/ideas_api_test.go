package web

import (
	"cmp"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/settings"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

var agentry = events.Actor{Kind: events.System, ID: "agentry"}

// suggestIdea keeps an idea from the session task about repo, as a
// Visionaries done does.
func suggestIdea(t *testing.T, s *store.Store, task, repo, title string) string {
	t.Helper()
	created, rejection, err := s.Create(ideas.Prefix, events.Proposal{Actor: agentry, Type: ideas.IdeaSuggested, Data: map[string]any{
		"title": title, "problem": "p", "proposal": "f", "size": "M", "repo": repo, "task": task, "kind": ""}})
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	return created.Stream
}

func ideaIDs(t *testing.T, body string) []string {
	t.Helper()
	var list []ideaView
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatal(err, body)
	}
	ids := []string{}
	for _, idea := range list {
		ids = append(ids, idea.ID)
	}
	return ids
}

func TestIdeasListNewestSessionFirst(t *testing.T) {
	server, s := newTestServer(t)
	repo, other := gitRepo(t), gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Older session","repo":"`+repo+`"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Newer session","repo":"`+other+`"}`)
	newer := suggestIdea(t, s, "task-2", other, "From the newer")
	older := suggestIdea(t, s, "task-1", repo, "From the older, kept last")
	again := suggestIdea(t, s, "task-2", other, "From the newer, again")

	code, body := call(t, server, "GET", "/api/ideas", "")
	if want := []string{again, newer, older}; code != 200 || !slices.Equal(ideaIDs(t, body), want) {
		t.Errorf("every idea: %d, want %v: %s", code, want, body)
	}
	if !strings.Contains(body, `"state":"open"`) || !strings.Contains(body, `"backedBy":[]`) ||
		strings.Contains(body, `"decidedAt"`) {
		t.Errorf("an idea's view: %s", body)
	}
	if _, body := call(t, server, "GET", "/api/ideas?repo="+repo, ""); !slices.Equal(ideaIDs(t, body), []string{older}) {
		t.Errorf("one repo's ideas: %s", body)
	}
	if _, body := call(t, server, "GET", "/api/ideas?repo=/no/such/repo", ""); body != "[]\n" {
		t.Errorf("no ideas is an empty list: %q", body)
	}
}

func TestListingIdeasSendsNoQuery(t *testing.T) {
	queries := storetest.CountQueries(t)
	server, s := newTestServer(t)
	repo, other := gitRepo(t), gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up more","repo":"`+other+`"}`)
	open := suggestIdea(t, s, "task-1", repo, "Search the board")
	dismissed := suggestIdea(t, s, "task-2", other, "Theme the board")
	if code, body := call(t, server, "POST", "/api/ideas/"+dismissed+"/dismiss", `{"note":"No"}`); code != 200 {
		t.Fatalf("dismiss: %d %s", code, body)
	}
	if queries.Sent() == 0 {
		t.Fatal("writing sent no query: the store isn't counting")
	}

	queries.Reset()
	if code, body := call(t, server, "GET", "/api/ideas", ""); code != 200 || !slices.Equal(ideaIDs(t, body), []string{dismissed, open}) ||
		!strings.Contains(body, `"state":"dismissed"`) {
		t.Errorf("every idea: %d %s", code, body)
	}
	if _, body := call(t, server, "GET", "/api/ideas?repo="+repo, ""); !slices.Equal(ideaIDs(t, body), []string{open}) {
		t.Errorf("one repo's ideas: %s", body)
	}
	if sent := queries.Sent(); sent != 0 {
		t.Errorf("listing ideas sent %d queries, want none", sent)
	}
}

func TestYouAcceptAnIdeaOnce(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	id := suggestIdea(t, s, "task-1", repo, "Search the board")

	code, body := call(t, server, "POST", "/api/ideas/"+id+"/accept", `{"kind":"epic","note":"Soon"}`)
	var got acceptance
	json.Unmarshal([]byte(body), &got)
	if code != 200 || got != (acceptance{Task: "task-2", Column: "backlog", Accepted: true}) {
		t.Fatalf("accept: %d %s", code, body)
	}
	if _, body := call(t, server, "GET", "/api/tasks/task-2", ""); !strings.Contains(body, `"kind":"epic"`) || !strings.Contains(body, `"parked":"backlog"`) ||
		!strings.Contains(body, `"madeFrom":"`+id+`"`) {
		t.Errorf("the task made from it: %s", body)
	}
	if _, body := call(t, server, "GET", "/api/tasks", ""); !strings.Contains(body, `"id":"task-2"`) || !strings.Contains(body, `"madeFrom":"`+id+`"`) {
		t.Errorf("the task list: %s", body)
	}
	if code, body := call(t, server, "POST", "/api/ideas/"+id+"/accept", `{}`); code != http.StatusConflict || !strings.Contains(body, "accepted already") {
		t.Errorf("a second accept: %d %s", code, body)
	}
	if code, _ := call(t, server, "POST", "/api/ideas/"+id+"/dismiss", `{"note":"No"}`); code != http.StatusConflict {
		t.Errorf("dismissing an accepted idea: %d", code)
	}
	if _, body := call(t, server, "GET", "/api/ideas", ""); !strings.Contains(body, `"state":"accepted"`) || !strings.Contains(body, `"became":"task-2"`) ||
		!strings.Contains(body, `"becameColumn":"backlog"`) || !strings.Contains(body, `"autoAccepted":false`) {
		t.Errorf("the accepted idea: %s", body)
	}
}

// Your accept gives the task the server's limits, unlimited ones too.
func TestATaskFromAnIdeaYouAcceptTakesTheServersLimits(t *testing.T) {
	for _, limits := range []tasks.Limits{{}, {Budget: 5_000_000, MaxTurns: 24}} {
		server, s := newTestServerWith(t, limits)
		repo := gitRepo(t)
		call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`","budget":-1,"maxTurns":-1}`)
		id := suggestIdea(t, s, "task-1", repo, "Search the board")
		if code, body := call(t, server, "POST", "/api/ideas/"+id+"/accept", `{}`); code != 200 {
			t.Fatalf("accept: %d %s", code, body)
		}
		if d := detailOfTask(t, server, "task-2"); d.Budget != limits.Budget || d.MaxTurns != limits.MaxTurns {
			t.Errorf("budget %d, max turns %d, want %+v", d.Budget, d.MaxTurns, limits)
		}
	}
}

// Accept and start makes the task out of the Backlog, with the budget you
// gave or the server's, and always the server's turns; a bad start or
// budget is a 400 that makes nothing.
func TestYouAcceptAndStartAnIdea(t *testing.T) {
	s, state := storetest.OpenWith(t, filepath.Join(t.TempDir(), "agentry.db"), storetest.WithIdeas())
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, DefaultRepo: "/r", Budget: 5_000_000, MaxTurns: 12,
		Features: []features.Feature{ideas.Feature(state)}}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)

	for _, bad := range []string{`{"start":"yes"}`, `{"start":1}`, `{"budget":-2}`, `{"budget":"lots"}`} {
		id := suggestIdea(t, s, "task-1", repo, "Bad "+bad)
		if code, body := call(t, server, "POST", "/api/ideas/"+id+"/accept", bad); code != http.StatusBadRequest {
			t.Errorf("%s: %d %s", bad, code, body)
		}
	}
	if ids := state.TaskIDs(); !slices.Equal(ids, []string{"task-1"}) {
		t.Fatalf("a bad accept made a task: %v", ids)
	}

	for _, c := range []struct {
		body   string
		task   string
		parked string
		budget int
	}{
		{`{"start":true,"budget":700}`, "task-2", "", 700},
		{`{"start":true}`, "task-3", "", 5_000_000},
		{`{"start":true,"budget":-1}`, "task-4", "", 0},
		{`{"start":false,"budget":0}`, "task-5", "backlog", 5_000_000},
	} {
		id := suggestIdea(t, s, "task-1", repo, "Idea for "+c.task)
		code, body := call(t, server, "POST", "/api/ideas/"+id+"/accept", c.body)
		var got acceptance
		json.Unmarshal([]byte(body), &got)
		if code != 200 || got != (acceptance{Task: c.task, Column: cmp.Or(c.parked, "in-progress"), Accepted: true}) {
			t.Fatalf("%s: %d %s", c.body, code, body)
		}
		task, err := state.Task(c.task)
		if err != nil || task.Parked != c.parked || task.Budget != c.budget || task.MaxTurns != 12 {
			t.Errorf("%s: parked %q budget %d turns %d, want %q %d 12 (%v)", c.body, task.Parked, task.Budget, task.MaxTurns, c.parked, c.budget, err)
		}
	}

	_, body := call(t, server, "GET", "/api/ideas", "")
	var list []ideaView
	json.Unmarshal([]byte(body), &list)
	columns := map[string]string{}
	for _, idea := range list {
		columns[idea.Became] = idea.BecameColumn
	}
	if want := map[string]string{"": "", "task-2": "in-progress", "task-3": "in-progress", "task-4": "in-progress", "task-5": "backlog"}; !maps.Equal(columns, want) {
		t.Errorf("the columns the ideas' tasks are in: %v, want %v", columns, want)
	}
}

func TestAnIdeaAgentryAcceptedSaysSo(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	visionaries, _ := agents.TemplateNamed(ideas.VisionariesTeam)
	created, rejection, err := s.Create(ideas.Prefix, events.Proposal{Actor: agentry, Type: ideas.IdeaSuggested, Data: map[string]any{
		"title": "Search the board", "problem": "p", "proposal": "f", "size": "M", "repo": repo, "task": "task-1", "kind": "",
		"backedBy": []string{"product-strategist", "user-advocate", "technologist", "skeptic"}}})
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	id := created.Stream
	for _, p := range append(agents.AddTeam(visionaries, nil, you), []events.Proposal{
		{Stream: events.SettingsStream, Actor: you, Type: events.SettingsChanged, Data: map[string]any{settings.AutoAcceptIdeas: true}},
		{Stream: "task-2", Actor: agentry, Type: events.TaskCreated, Data: map[string]any{"title": "Search the board", "repo": repo,
			"coordinator": "tech-lead", "parked": "backlog", "idea": id}},
		{Stream: id, Actor: agentry, Type: ideas.IdeaAccepted, Data: map[string]any{"task": "task-2", "note": "auto-accepted: all Visionaries agreed"}},
	}...) {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	if _, body := call(t, server, "GET", "/api/ideas", ""); !strings.Contains(body, `"state":"accepted"`) || !strings.Contains(body, `"autoAccepted":true`) {
		t.Errorf("the idea Agentry accepted: %s", body)
	}
}

func TestYouDismissAnIdea(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	id := suggestIdea(t, s, "task-1", repo, "Dark mode")

	if code, body := call(t, server, "POST", "/api/ideas/"+id+"/dismiss", `{"note":" The OS does it "}`); code != 200 {
		t.Fatalf("dismiss: %d %s", code, body)
	}
	if _, body := call(t, server, "GET", "/api/ideas", ""); !strings.Contains(body, `"state":"dismissed"`) || !strings.Contains(body, `"note":"The OS does it"`) ||
		!strings.Contains(body, `"decidedAt":"`) {
		t.Errorf("the dismissed idea: %s", body)
	}
	if code, _ := call(t, server, "POST", "/api/ideas/"+id+"/accept", `{}`); code != http.StatusConflict {
		t.Errorf("accepting a dismissed idea: %d", code)
	}
}

func TestAnUnknownIdeaIsNotFound(t *testing.T) {
	server, _ := newTestServer(t)
	for _, path := range []string{"/api/ideas/idea-9/accept", "/api/ideas/idea-9/dismiss", "/api/ideas/task-1/dismiss"} {
		if code, _ := call(t, server, "POST", path, `{}`); code != http.StatusNotFound {
			t.Errorf("%s: %d", path, code)
		}
	}
}

// An idea from the session Agentry started after an epic closed names the
// epic; others name none.
func TestAnIdeaNamesTheEpicItsSessionWasAbout(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Ideas list", "repo": repo,
			"coordinator": "tech-lead", "kind": "epic"}},
		{Stream: "task-2", Actor: agentry, Type: events.TaskCreated, Data: map[string]any{"title": "Follow-ups after Ideas list",
			"repo": repo, "coordinator": "tech-lead", "about": "task-1", "close": 9}},
		{Stream: "task-3", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Think", "repo": repo, "coordinator": "tech-lead"}},
	} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	after, plain := suggestIdea(t, s, "task-2", repo, "Search ideas"), suggestIdea(t, s, "task-3", repo, "Dark mode")

	_, body := call(t, server, "GET", "/api/ideas", "")
	var list []ideaView
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatal(err, body)
	}
	for _, idea := range list {
		want := ideaView{}
		if idea.ID == after {
			want = ideaView{About: "task-1", AboutTitle: "Ideas list"}
		}
		if idea.ID != after && idea.ID != plain || idea.About != want.About || idea.AboutTitle != want.AboutTitle {
			t.Errorf("%s: about %q %q, want %q %q", idea.ID, idea.About, idea.AboutTitle, want.About, want.AboutTitle)
		}
	}
}

// Every open idea says why Agentry wouldn't accept it for you, under your
// rule, even with accepting turned off; "" when it would.
func TestAnOpenIdeaSaysWhyItWasntAutoAccepted(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	visionaries, _ := agents.TemplateNamed(ideas.VisionariesTeam)
	for _, p := range agents.AddTeam(visionaries, nil, you) {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}
	every := []string{"product-strategist", "user-advocate", "technologist", "skeptic"}
	suggest := func(title string, backedBy, doubtedBy []string) string {
		t.Helper()
		created, rejection, err := s.Create(ideas.Prefix, events.Proposal{Actor: agentry, Type: ideas.IdeaSuggested, Data: map[string]any{
			"title": title, "problem": "p", "proposal": "f", "size": "M", "repo": repo, "task": "task-1", "kind": "",
			"backedBy": backedBy, "doubtedBy": doubtedBy}})
		if rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
		return created.Stream
	}
	original := suggest("Search the board", every, nil)
	if code, body := call(t, server, "POST", "/api/ideas/"+original+"/accept", `{}`); code != 200 {
		t.Fatalf("accept: %d %s", code, body)
	}
	doubted := suggest("Dark mode", every, []string{"skeptic"})
	alone := suggest("Export", []string{"skeptic"}, nil)
	two := suggest("Undo", []string{"skeptic", "technologist"}, nil)
	agreed := suggest("Keyboard shortcuts", every, nil)
	again := suggest("search the Board!", every, nil)

	reasons := func() map[string]string {
		t.Helper()
		_, body := call(t, server, "GET", "/api/ideas", "")
		var list []ideaView
		if err := json.Unmarshal([]byte(body), &list); err != nil {
			t.Fatal(err, body)
		}
		got := map[string]string{}
		for _, idea := range list {
			got[idea.ID] = idea.NotAutoAccepted
		}
		return got
	}
	strict := map[string]string{original: "", doubted: "doubted by skeptic", alone: "not backed by product-strategist, user-advocate, technologist",
		two: "not backed by product-strategist, user-advocate", agreed: "", again: "duplicate of " + original + " (task-2)"}
	if got := reasons(); !maps.Equal(got, strict) {
		t.Errorf("everyoneBacked, accepting off:\ngot  %v\nwant %v", got, strict)
	}
	call(t, server, "PUT", "/api/settings", `{"autoAcceptIdeas":true,"autoAcceptRule":"nobodyDoubted"}`)
	loose := map[string]string{original: "", doubted: "doubted by skeptic", alone: "only one backer", two: "", agreed: "",
		again: "duplicate of " + original + " (task-2)"}
	if got := reasons(); !maps.Equal(got, loose) {
		t.Errorf("nobodyDoubted:\ngot  %v\nwant %v", got, loose)
	}
}

// A task an old log made from an idea names it only as "idea"; its page
// says it was made from the idea all the same. A task made from nothing says
// nothing.
func TestATaskFromAnOldLogShowsTheIdeaItWasMadeFrom(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	id := suggestIdea(t, s, "task-1", repo, "Search the board")
	if _, rejection, err := s.Propose(events.Proposal{Stream: "task-2", Actor: events.Actor{Kind: events.Human, ID: "you"}, Type: events.TaskCreated,
		Data: map[string]any{"title": "Search the board", "repo": repo, "coordinator": "tech-lead", "parked": "backlog", "idea": id}}); err != nil || rejection != nil {
		t.Fatalf("old task.created: %v %v", rejection, err)
	}
	for task, want := range map[string]string{"task-2": id, "task-1": ""} {
		_, body := call(t, server, "GET", "/api/tasks/"+task, "")
		var detail taskDetail
		if err := json.Unmarshal([]byte(body), &detail); err != nil || detail.MadeFrom != want {
			t.Errorf("%s made from %q, want %q: %v", task, detail.MadeFrom, want, err)
		}
	}
}
