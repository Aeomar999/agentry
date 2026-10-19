package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

func TestFeaturesNeedTheUIToken(t *testing.T) {
	server, _ := newTestServer(t)
	response, err := server.Client().Get(server.URL + "/api/features")
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 401 {
		t.Errorf("no token: %d", response.StatusCode)
	}
}

// The features carry what the log holds that this build skipped: the
// streams of a feature it doesn't have, and a type it doesn't know.
func TestFeaturesCountWhatThisBuildSkipped(t *testing.T) {
	spec := events.Spec{Writers: []events.ActorKind{events.System}}
	newer, err := events.NewCatalogue(
		events.Registration{Prefix: "note-", Types: map[events.Type]events.Spec{"note.written": spec}, TaskTypes: map[events.Type]events.Spec{"git.pr_opened": spec}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "agentry.db")
	written, err := store.Open(path, newer, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	agentry := events.Actor{Kind: events.System, ID: "agentry"}
	for _, p := range []events.Proposal{
		{Stream: "task-1", Actor: agentry, Type: events.TaskCreated, Data: map[string]any{"title": "One", "repo": "/r", "coordinator": "tech-lead", "about": "task-0"}},
		{Stream: "note-1", Actor: agentry, Type: "note.written"},
		{Stream: "task-1", Actor: agentry, Type: "git.pr_opened"},
		{Stream: "note-2", Actor: agentry, Type: "note.written"},
		{Stream: "note-1", Actor: agentry, Type: "note.written"},
	} {
		if _, rejection, err := written.Propose(p); rejection != nil || err != nil {
			t.Fatal(p.Type, rejection, err)
		}
	}
	written.Close()

	server, _ := newTestServerOn(t, path, tasks.Limits{})
	code, body := call(t, server, "GET", "/api/features", "")
	want := `"unknown":[{"prefix":"note-","streams":2,"events":3,"last":5},{"type":"git.pr_opened","events":1,"last":3}]`
	if code != 200 || !strings.Contains(body, want) {
		t.Errorf("features: %d %s\nwant %s", code, body, want)
	}

	fresh, _ := newTestServer(t)
	if _, body := call(t, fresh, "GET", "/api/features", ""); !strings.Contains(body, `"unknown":[]`) {
		t.Errorf("a log with nothing skipped: %s", body)
	}
}

// votes is a feature with a route behind each Auth: a read for the page, a
// read the desk may make too, and two writes, one of a type it lists in
// Writes and one it doesn't, behind a route that names no Auth.
func votes() features.Feature {
	return features.Feature{Name: "votes", Writes: []events.Type{events.TaskCreated}, Routes: func(c features.Core) []features.Route {
		write := func(p events.Proposal) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) { httpapi.Propose(w, c.Log, p) }
		}
		return []features.Route{
			{Pattern: "GET /api/votes", Auth: features.UI, Handler: func(w http.ResponseWriter, r *http.Request) { httpapi.Reply(w, "votes") }},
			{Pattern: "GET /api/votes/count", Auth: features.Desk, Handler: func(w http.ResponseWriter, r *http.Request) { httpapi.Reply(w, 3) }},
			{Pattern: "POST /api/votes/task", Handler: write(events.Proposal{Stream: "task-2", Actor: httpapi.You, Type: events.TaskCreated,
				Data: map[string]any{"title": "Count the votes", "repo": "/src/app", "coordinator": "tech-lead"}})},
			{Pattern: "POST /api/votes/close", Handler: write(events.Proposal{Stream: "task-1", Actor: httpapi.You, Type: events.TaskDone,
				Data: map[string]any{"summary": "Counted."}})},
		}
	}}
}

// votesServer serves votes, writing through logs, with a task-1 to write to.
func votesServer(t *testing.T, logs func(*store.Store) map[string]features.Proposer) (*httptest.Server, *store.Store) {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, DeskToken: "desk-secret", DefaultRepo: "/src/app",
		Features: []features.Feature{votes()}, Logs: logs(s)}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	if code, body := call(t, server, "POST", "/api/tasks", `{"title":"Vote"}`); code != 200 {
		t.Fatalf("create task-1: %d %s", code, body)
	}
	return server, s
}

// checkedVotes is votes' log as serve makes it.
func checkedVotes(s *store.Store) map[string]features.Proposer {
	return map[string]features.Proposer{"votes": features.Checked(votes(), events.Registration{}, s)}
}

// callAs is call with bearer as the credential, none when it is "".
func callAs(t *testing.T, server *httptest.Server, bearer, method, path string) (int, string) {
	t.Helper()
	request, _ := http.NewRequest(method, server.URL+path, strings.NewReader("{}"))
	if bearer != "" {
		request.Header.Set("Authorization", "Bearer "+bearer)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(content)
}

// A feature's route is behind the wrapper its Auth names: UI takes only the
// UI token, Desk the desk's too, and a route naming no Auth is UI's.
func TestAFeatureRouteIsBehindItsAuth(t *testing.T) {
	server, s := votesServer(t, checkedVotes)
	for _, c := range []struct {
		method, path, bearer string
		want                 int
	}{
		{"GET", "/api/votes", "", http.StatusUnauthorized},
		{"GET", "/api/votes", "desk-secret", http.StatusUnauthorized},
		{"GET", "/api/votes", token, http.StatusOK},
		{"GET", "/api/votes/count", "", http.StatusUnauthorized},
		{"GET", "/api/votes/count", "desk-secret", http.StatusOK},
		{"GET", "/api/votes/count", token, http.StatusOK},
		{"POST", "/api/votes/task", "", http.StatusUnauthorized},
		{"POST", "/api/votes/task", "desk-secret", http.StatusUnauthorized},
	} {
		if code, body := callAs(t, server, c.bearer, c.method, c.path); code != c.want {
			t.Errorf("%s %s with %q: %d %s, want %d", c.method, c.path, c.bearer, code, body, c.want)
		}
	}
	if got, want := positionOf(t, server, "GET", "/api/votes"), itoa(s.LastPosition()); got != want {
		t.Errorf("a feature's read says position %q, want %s", got, want)
	}
}

// A feature's route writes through its checked proposer, as you: a type it
// lists in Writes is written, one it doesn't is refused with a 409 and
// nothing is stored.
func TestAFeatureRouteWritesOnlyWhatItDeclares(t *testing.T) {
	server, s := votesServer(t, checkedVotes)
	before := s.LastPosition()
	code, body := call(t, server, "POST", "/api/votes/close", `{}`)
	if code != http.StatusConflict || !strings.Contains(body, "the votes feature may not write task.done") {
		t.Errorf("an undeclared type: %d %s, want a 409 from the features check", code, body)
	}
	if after := s.LastPosition(); after != before {
		t.Errorf("the refused write moved the log from %d to %d", before, after)
	}
	if code, body := call(t, server, "POST", "/api/votes/task", `{}`); code != 200 || !strings.Contains(body, `"stream":"task-2"`) {
		t.Fatalf("a declared type: %d %s", code, body)
	}
	if created, _ := s.Log("task-2"); len(created) != 1 || created[0].Actor != httpapi.You {
		t.Errorf("task-2 should be made once, as you: %+v", created)
	}
}

// A feature the server was given no log for writes nothing, even a type it
// lists in Writes; its reads still answer.
func TestAFeatureRouteWithNoLogWritesNothing(t *testing.T) {
	server, s := votesServer(t, func(*store.Store) map[string]features.Proposer { return nil })
	before := s.LastPosition()
	if code, body := call(t, server, "POST", "/api/votes/task", `{}`); code != http.StatusConflict || !strings.Contains(body, "votes") {
		t.Errorf("a write with no log: %d %s, want a 409", code, body)
	}
	if after := s.LastPosition(); after != before {
		t.Errorf("the log moved from %d to %d", before, after)
	}
	if code, body := call(t, server, "GET", "/api/votes", ""); code != 200 || body != "\"votes\"\n" {
		t.Errorf("a read with no log: %d %q", code, body)
	}
}

// A feature's route whose pattern a core route or another feature's has
// already fails to mount, naming the feature and the pattern, whether the
// core route is one of the first or the last the server registers.
func TestAFeatureRouteThatClashesNamesItsFeature(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	one := func(name, pattern string) features.Feature {
		return features.Feature{Name: name, Routes: func(features.Core) []features.Route {
			return []features.Route{{Pattern: pattern, Handler: func(http.ResponseWriter, *http.Request) {}}}
		}}
	}
	for _, step := range []struct {
		list []features.Feature
		want string
	}{
		{[]features.Feature{one("votes", "GET /api/tasks")}, `the votes feature's route "GET /api/tasks" can't be mounted`},
		{[]features.Feature{one("votes", "DELETE /api/integrations/connections/{id}")}, `the votes feature's route "DELETE /api/integrations/connections/{id}" can't be mounted`},
		{[]features.Feature{one("votes", "GET /api/votes"), one("polls", "GET /api/votes")}, `the polls feature's route "GET /api/votes" can't be mounted`},
	} {
		got := func() (clash any) {
			defer func() { clash = recover() }()
			NewServer(s, state, Config{UIToken: token, Features: step.list}, http.NotFoundHandler(), nil).Handler()
			return nil
		}()
		if message, _ := got.(string); !strings.HasPrefix(message, step.want) {
			t.Errorf("mounting %s: panicked with %v, want %q", step.list[len(step.list)-1].Name, got, step.want)
		}
	}
}
