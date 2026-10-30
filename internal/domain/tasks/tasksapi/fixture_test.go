package tasksapi_test

import (
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks/tasksapi"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

// The routes' tests' fixture, each piece mirroring
// internal/web/api_test.go's.

const token = "ui-secret"

// newTestServer is a server on a log of its own, with the default agents
// and no limits for new tasks.
func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	hooks := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hooks")) })
	server := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: token, DefaultRepo: "/src/app", Clones: filepath.Join(t.TempDir(), "repos"),
		Blobs: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}}, hooks, nil).Handler())
	t.Cleanup(server.Close)
	return server, s
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

// The views the tests decode, as tasksapi's export_test.go hands them out.
type (
	taskDetail  = tasksapi.TaskDetail
	taskSummary = tasks.Summary
	eventEntry  = tasksapi.EventEntry
	eventsPage  = tasksapi.EventsPage
	erased      = tasksapi.Erased
)

const (
	eventPage     = tasksapi.EventPage
	eventPageMax  = tasksapi.EventPageMax
	eventTypesMax = tasksapi.EventTypesMax
)

var entryOf = tasksapi.EntryOf

// limitsServer is a server whose new tasks get a 5M budget and 2 turns, and
// ways to give a task turns and to stop it with a question, with more of the
// event's data: the loop's limit stops say "limit".
func limitsServer(t *testing.T) (server *httptest.Server, work func(id string, turns, tokens int), ask func(id string, by events.Actor, more map[string]any)) {
	t.Helper()
	core := func(p events.Proposal, state rules.State, catalogue *events.Catalogue) *events.Rejection {
		return rules.Check(p, state, catalogue)
	}
	return limitsServerChecked(t, core)
}

// limitsServerChecked is limitsServer with its own rules, to refuse what the
// real rules would refuse only in a race.
func limitsServerChecked(t *testing.T, check func(events.Proposal, rules.State, *events.Catalogue) *events.Rejection) (server *httptest.Server, work func(id string, turns, tokens int), ask func(id string, by events.Actor, more map[string]any)) {
	t.Helper()
	catalogue := events.Core()
	state := current.New(catalogue)
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, err := store.Open(path, catalogue, state, func(p events.Proposal) *events.Rejection { return check(p, state, catalogue) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	t.Cleanup(func() { // cleanups run last first: before the close
		storetest.CheckEqual(t, s, state)
		storetest.CheckSnapshots(t, path, catalogue, s.LastPosition())
	})
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server = httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: token, DefaultRepo: "/r", Budget: 5_000_000, MaxTurns: 2}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	propose := func(p events.Proposal) {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatalf("%s: %v %v", p.Type, err, rejection)
		}
	}
	worker := events.Actor{Kind: events.Worker, ID: "local"}
	work = func(id string, turns, tokens int) {
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/" + id, "branch": "b", "port": 7901}})
		for i := range turns {
			turn := fmt.Sprintf("turn-%d", i+1)
			propose(events.Proposal{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: turn, Data: map[string]any{"agent": "tech-lead", "hash": "h"}})
			propose(events.Proposal{Stream: id, Actor: worker, Type: events.TurnEnded, Correlation: turn,
				Data: map[string]any{"agent": "tech-lead", "report": "r", "tokens": tokens / turns}})
		}
	}
	ask = func(id string, by events.Actor, more map[string]any) {
		data := map[string]any{"question": "Go on?"}
		maps.Copy(data, more)
		propose(events.Proposal{Stream: id, Actor: by, Type: events.TaskNeedsYou, Data: data})
	}
	return server, work, ask
}

func atLimit(limit string) map[string]any { return map[string]any{"limit": limit} }

// detailOfTask mirrors internal/web/api_test.go's.
func detailOfTask(t *testing.T, server *httptest.Server, id string) taskDetail {
	t.Helper()
	_, body := call(t, server, "GET", "/api/tasks/"+id, "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	return detail
}

func lastSaid(d taskDetail) string { return d.Trail[len(d.Trail)-1].Text }

var (
	theLoop  = events.Actor{Kind: events.System, ID: "coordinator-loop"}
	techLead = events.Actor{Kind: events.Agent, ID: "tech-lead"} // mirrors internal/web/api_test.go's

	// copyQuestion is the loop's question to copy a file from outside a worktree.
	copyQuestion = map[string]any{"question": "Copy it?", "options": []string{"Copy it", "Don't copy it"},
		"fetchMachine": "local", "fetchPath": "/etc/hosts", "fetchAs": "hosts"}
)

// gitRepo mirrors internal/web/workers_test.go's.
func gitRepo(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	for _, args := range [][]string{{"init", "-q", "-b", "dev"}, {"commit", "-q", "--allow-empty", "-m", "first"}} {
		command := exec.Command("git", append([]string{"-C", repo, "-c", "user.name=t", "-c", "user.email=t@e"}, args...)...)
		if output, err := command.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v %s", args, err, output)
		}
	}
	return repo
}

// dataTeam is a custom team: its lead and two specialists.
const dataTeam = `{"team":"Data","agents":[
	{"name":"data-lead","engine":"claude","coordinates":true},
	{"name":"analyst","engine":"claude","description":"Analyse the numbers."},
	{"name":"charter","engine":"claude","description":"Draw the charts."}]}`

// newTestServerChecking is newTestServer whose log also runs check after
// the rules, so a test can refuse one proposal.
func newTestServerChecking(t *testing.T, check func(events.Proposal) *events.Rejection) (*httptest.Server, *store.Store) {
	t.Helper()
	server, s, _ := newTestStateChecking(t, check)
	return server, s
}

// newTestStateChecking is newTestServerChecking and the state its log
// keeps, for a test that archives a task in it.
func newTestStateChecking(t *testing.T, check func(events.Proposal) *events.Rejection) (*httptest.Server, *store.Store, *current.State) {
	t.Helper()
	catalogue := events.Core()
	state := storetest.NewState(catalogue)
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), catalogue, state, func(p events.Proposal) *events.Rejection {
		if rejection := rules.Check(p, state, catalogue, storetest.Installed()...); rejection != nil {
			return rejection
		}
		return check(p)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: token, DefaultRepo: "/src/app", Clones: filepath.Join(t.TempDir(), "repos"),
		Blobs: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return server, s, state
}

// callAs is call with bearer as the credential, none when it is ""; it
// mirrors internal/web/extensions_api_test.go's.
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
