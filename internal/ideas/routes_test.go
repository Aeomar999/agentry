package ideas_test

import (
	"cmp"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/settings"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

// The ideas' routes, tested over HTTP through the server serve runs: web
// mounts them from Extension, and they write through the proposer Checked
// makes, as serve.go gives it.

const token = "ui-secret"

func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	server, s, _ := newServer(t, tasks.Limits{})
	return server, s
}

// newTestServerWith is newTestServer with the new tasks' limits in limits.
func newTestServerWith(t *testing.T, limits tasks.Limits) (*httptest.Server, *store.Store) {
	t.Helper()
	server, s, _ := newServer(t, limits)
	return server, s
}

// newServer is the server with ideas, the default agents and new tasks'
// limits in limits.
func newServer(t *testing.T, limits tasks.Limits) (*httptest.Server, *store.Store, *current.State) {
	t.Helper()
	return newServerLogging(t, limits, func(s *store.Store) extensions.Proposer {
		return extensions.Checked(ideas.Extension(), extensions.Registration(ideas.Extension()), s)
	})
}

// newServerLogging is newServer with the ideas' log made by logOf.
func newServerLogging(t *testing.T, limits tasks.Limits, logOf func(*store.Store) extensions.Proposer) (*httptest.Server, *store.Store, *current.State) {
	t.Helper()
	s, state := storetest.OpenWith(t, filepath.Join(t.TempDir(), "agentry.db"), storetest.WithExtensions())
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: token, DefaultRepo: "/src/app", Clones: filepath.Join(t.TempDir(), "repos"),
		Blobs: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}, Budget: limits.Budget, MaxTurns: limits.MaxTurns,
		Extensions: []extensions.Extension{ideas.Extension()}, Logs: map[string]extensions.Proposer{ideas.Name: logOf(s)}}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return server, s, state
}

func call(t *testing.T, server *httptest.Server, method, path, body string) (int, string) {
	t.Helper()
	request, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(content)
}

// detailOfTask is the limits a task's page shows.
func detailOfTask(t *testing.T, server *httptest.Server, id string) (detail struct{ Budget, MaxTurns int }) {
	t.Helper()
	_, body := call(t, server, "GET", "/api/tasks/"+id, "")
	json.Unmarshal([]byte(body), &detail)
	return detail
}

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
	var list []ideas.View
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
	var got ideas.AcceptReply
	json.Unmarshal([]byte(body), &got)
	if code != 200 || got != (ideas.AcceptReply{Task: "task-2", Column: "backlog", Kind: tasks.Epic, Accepted: true}) {
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
		!strings.Contains(body, `"becameColumn":"backlog"`) || !strings.Contains(body, `"becameKind":"epic"`) || !strings.Contains(body, `"autoAccepted":false`) {
		t.Errorf("the accepted idea: %s", body)
	}
}

// An idea that looks like an open task is refused with a 409 naming that
// task, so the page can show it, and nothing is written; with anyway it
// becomes a second task.
func TestAnIdeaLikeAnOpenTaskIsRefusedUntilYouSayAnyway(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	first := suggestIdea(t, s, "task-1", repo, "Make a ship the user chose actually push, and count it shipped only after the push")
	second := suggestIdea(t, s, "task-1", repo, "Make a ship the user chose push, and count it shipped only when it pushed")
	if code, body := call(t, server, "POST", "/api/ideas/"+first+"/accept", `{}`); code != 200 {
		t.Fatalf("accept the first: %d %s", code, body)
	}

	before := s.LastPosition()
	code, body := call(t, server, "POST", "/api/ideas/"+second+"/accept", `{}`)
	var refusal struct {
		Error     string
		Duplicate struct{ ID, Title, Column string }
	}
	json.Unmarshal([]byte(body), &refusal)
	if code != http.StatusConflict || refusal.Duplicate.ID != "task-2" || refusal.Duplicate.Column != "backlog" ||
		!strings.HasPrefix(refusal.Duplicate.Title, "Make a ship the user chose actually push") || !strings.Contains(refusal.Error, "looks like the open task task-2") {
		t.Fatalf("accept the second: %d %s, want a 409 naming task-2", code, body)
	}
	if s.LastPosition() != before {
		t.Errorf("the refused accept wrote: log at %d from %d", s.LastPosition(), before)
	}

	code, body = call(t, server, "POST", "/api/ideas/"+second+"/accept", `{"anyway":true}`)
	var got ideas.AcceptReply
	json.Unmarshal([]byte(body), &got)
	if code != 200 || got != (ideas.AcceptReply{Task: "task-3", Column: "backlog", Accepted: true}) {
		t.Errorf("accept anyway: %d %s", code, body)
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
	server, s, state := newServer(t, tasks.Limits{Budget: 5_000_000, MaxTurns: 12})
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
		var got ideas.AcceptReply
		json.Unmarshal([]byte(body), &got)
		if code != 200 || got != (ideas.AcceptReply{Task: c.task, Column: cmp.Or(c.parked, "in-progress"), Accepted: true}) {
			t.Fatalf("%s: %d %s", c.body, code, body)
		}
		task, err := state.Task(c.task)
		if err != nil || task.Parked != c.parked || task.Budget != c.budget || task.MaxTurns != 12 {
			t.Errorf("%s: parked %q budget %d turns %d, want %q %d 12 (%v)", c.body, task.Parked, task.Budget, task.MaxTurns, c.parked, c.budget, err)
		}
	}

	_, body := call(t, server, "GET", "/api/ideas", "")
	var list []ideas.View
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
	visionaries, _ := agents.TemplateNamed("Visionaries")
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
	var list []ideas.View
	if err := json.Unmarshal([]byte(body), &list); err != nil {
		t.Fatal(err, body)
	}
	for _, idea := range list {
		want := ideas.View{}
		if idea.ID == after {
			want = ideas.View{About: "task-1", AboutTitle: "Ideas list"}
		}
		if idea.ID != after && idea.ID != plain || idea.About != want.About || idea.AboutTitle != want.AboutTitle {
			t.Errorf("%s: about %q %q, want %q %q", idea.ID, idea.About, idea.AboutTitle, want.About, want.AboutTitle)
		}
	}
}

// Every open idea says why Agentry wouldn't accept it for you, under your
// rule, even with accepting turned off; "" when it would. The Visionaries
// are the built-in team or a team from your template based on it.
func TestAnOpenIdeaSaysWhyItWasntAutoAccepted(t *testing.T) {
	visionaries, _ := agents.TemplateNamed("Visionaries")
	dreams := agents.UserTemplate{ID: "dreams", Team: "Dreams", BasedOn: "visionaries"}
	for _, agent := range visionaries.Prompted() {
		agent.Team, agent.Template = "", "" // adding the template sets them
		dreams.Agents = append(dreams.Agents, agent)
	}
	for name, team := range map[string][]events.Proposal{
		"the Visionaries":                         agents.AddTeam(visionaries, nil, you),
		"a team from your template based on them": append([]events.Proposal{dreams.Save(you)}, agents.AddTeam(dreams.Template(), nil, you)...),
	} {
		t.Run(name, func(t *testing.T) { openIdeasSayWhy(t, team) })
	}
}

func openIdeasSayWhy(t *testing.T, team []events.Proposal) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	for _, p := range team {
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
	like := suggest("Search the whole board", every, nil) // no other idea's title, but close to task-2's

	reasons := func() map[string]string {
		t.Helper()
		_, body := call(t, server, "GET", "/api/ideas", "")
		var list []ideas.View
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
		two: "not backed by product-strategist, user-advocate", agreed: "", again: "duplicate of " + original + " (task-2)",
		like: "looks like the open task task-2"}
	if got := reasons(); !maps.Equal(got, strict) {
		t.Errorf("everyoneBacked, accepting off:\ngot  %v\nwant %v", got, strict)
	}
	call(t, server, "PUT", "/api/settings", `{"autoAcceptIdeas":true,"autoAcceptRule":"nobodyDoubted"}`)
	loose := map[string]string{original: "", doubted: "doubted by skeptic", alone: "only one backer", two: "", agreed: "",
		again: "duplicate of " + original + " (task-2)", like: "looks like the open task task-2"}
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
		var detail struct{ MadeFrom string }
		if err := json.Unmarshal([]byte(body), &detail); err != nil || detail.MadeFrom != want {
			t.Errorf("%s made from %q, want %q: %v", task, detail.MadeFrom, want, err)
		}
	}
}

// ideasLine is Ideas' line in GET /api/features.
func ideasLine(t *testing.T, server *httptest.Server) extensions.Line {
	t.Helper()
	code, body := call(t, server, "GET", "/api/features", "")
	var manifest struct{ Features []extensions.Line }
	if err := json.Unmarshal([]byte(body), &manifest); code != 200 || err != nil {
		t.Fatalf("features: %d %v %s", code, err, body)
	}
	for _, line := range manifest.Features {
		if line.Name == "ideas" {
			return line
		}
	}
	t.Fatalf("no ideas in %s", body)
	return extensions.Line{}
}

func TestIdeasHiddenWithNoTeamAndNoIdeas(t *testing.T) {
	server, _ := newTestServer(t)
	line := ideasLine(t, server)
	if line.Enabled || line.Visible || line.Badge != 0 || !strings.Contains(line.Why, "add a team from the Visionaries template") {
		t.Errorf("no team, no ideas: %+v", line)
	}
}

func TestIdeasOnWithTheVisionariesTeam(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/teams", `{"template":"Visionaries"}`); code != 200 {
		t.Fatalf("add Visionaries: %d %s", code, body)
	}
	line := ideasLine(t, server)
	if !line.Enabled || !line.Visible || line.Why != "the Visionaries team suggests ideas" {
		t.Errorf("with the team: %+v", line)
	}
}

func TestIdeasKeptForTheirIdeasWhenTheTeamIsRemoved(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/teams", `{"template":"Visionaries"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	suggestIdea(t, s, "task-1", repo, "Search the board")
	suggestIdea(t, s, "task-1", repo, "Theme the board")
	if code, body := call(t, server, "DELETE", "/api/teams/Visionaries", ""); code != 200 {
		t.Fatalf("remove Visionaries: %d %s", code, body)
	}
	line := ideasLine(t, server)
	if line.Enabled || !line.Visible || line.Why != "kept while 2 ideas remain; no team from the Visionaries template or one based on it now" {
		t.Errorf("team removed, ideas left: %+v", line)
	}
}

func TestIdeasBadgeCountsOpenIdeas(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	suggestIdea(t, s, "task-1", repo, "Search the board")
	accepted := suggestIdea(t, s, "task-1", repo, "Theme the board")
	dismissed := suggestIdea(t, s, "task-1", repo, "Sound on done")
	suggestIdea(t, s, "task-1", repo, "Export the board")
	if code, body := call(t, server, "POST", "/api/ideas/"+accepted+"/accept", `{}`); code != 200 {
		t.Fatalf("accept: %d %s", code, body)
	}
	if code, body := call(t, server, "POST", "/api/ideas/"+dismissed+"/dismiss", `{"note":"No"}`); code != 200 {
		t.Fatalf("dismiss: %d %s", code, body)
	}
	if line := ideasLine(t, server); line.Badge != 2 || !line.Visible {
		t.Errorf("two of four ideas open: %+v", line)
	}
}

// Accept writes through the ideas' checked proposer, never the store: given
// one that doesn't let Ideas write task.created, accepting is refused with
// the features check and makes no task, while a dismiss, of Ideas' own
// type, still goes through.
func TestAcceptWritesOnlyThroughTheCheckedLog(t *testing.T) {
	server, s, state := newServerLogging(t, tasks.Limits{}, func(s *store.Store) extensions.Proposer {
		return extensions.Checked(extensions.Extension{Name: ideas.Name}, extensions.Registration(ideas.Extension()), s)
	})
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	accepted, dismissed := suggestIdea(t, s, "task-1", repo, "Search the board"), suggestIdea(t, s, "task-1", repo, "Dark mode")

	before := s.LastPosition()
	if code, body := call(t, server, "POST", "/api/ideas/"+accepted+"/accept", `{}`); code != http.StatusConflict ||
		!strings.Contains(body, "the ideas feature may not write task.created") {
		t.Errorf("accept: %d %s, want a 409 from the features check", code, body)
	}
	if ids := state.TaskIDs(); !slices.Equal(ids, []string{"task-1"}) || s.LastPosition() != before {
		t.Errorf("a refused accept wrote: tasks %v, log at %d from %d", ids, s.LastPosition(), before)
	}
	if code, body := call(t, server, "POST", "/api/ideas/"+dismissed+"/dismiss", `{"note":"No"}`); code != 200 {
		t.Errorf("dismiss: %d %s", code, body)
	}
}
