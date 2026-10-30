package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/settingsfile"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
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

// The extensions carry what the log holds that this build skipped: the
// streams of an extension it doesn't have, and a type it doesn't know.
func TestFeaturesCountWhatThisBuildSkipped(t *testing.T) {
	spec := events.Spec{Writers: []events.ActorKind{events.System}}
	newer, err := events.NewCatalogue(
		events.Registration{Prefix: "note-", Types: map[events.Type]events.Spec{"note.written": spec}},
		events.Registration{TaskTypes: map[events.Type]events.Spec{"git.pr_opened": spec}})
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

// votes is an extension with a route behind each Auth: a read for the page, a
// read the desk may make too, two writes, one of a type it lists in Writes
// and one it doesn't, behind a route that names no Auth, and a webhook that
// raises an issue on task-1 saying the body it was signed with.
func votes() extensions.Extension {
	return extensions.Extension{Name: "votes", Writes: []events.Type{events.TaskCreated, events.IssueRaised}, Routes: func(c extensions.Core) []extensions.Route {
		write := func(p events.Proposal) http.HandlerFunc {
			return func(w http.ResponseWriter, r *http.Request) { httpapi.Propose(w, c.Log, p) }
		}
		return []extensions.Route{
			{Pattern: "GET /api/votes", Auth: extensions.UI, Handler: func(w http.ResponseWriter, r *http.Request) { httpapi.Reply(w, "votes") }},
			{Pattern: "GET /api/votes/count", Auth: extensions.Desk, Handler: func(w http.ResponseWriter, r *http.Request) { httpapi.Reply(w, 3) }},
			{Pattern: "POST /api/votes/task", Handler: write(events.Proposal{Stream: "task-2", Actor: httpapi.You, Type: events.TaskCreated,
				Data: map[string]any{"title": "Count the votes", "repo": "/src/app", "coordinator": "tech-lead"}})},
			{Pattern: "POST /api/votes/close", Handler: write(events.Proposal{Stream: "task-1", Actor: httpapi.You, Type: events.TaskDone,
				Data: map[string]any{"summary": "Counted."}})},
			{Pattern: "POST /api/votes/hook", Auth: extensions.Signed(signedWith(hookSecret)), Handler: func(w http.ResponseWriter, r *http.Request) {
				body, _ := io.ReadAll(r.Body)
				httpapi.Propose(w, c.Log, events.Proposal{Stream: "task-1", Actor: httpapi.System, Type: events.IssueRaised, Data: map[string]any{"reason": string(body)}})
			}},
		}
	}}
}

// votesServer serves votes, writing through logs, with a task-1 to write to.
func votesServer(t *testing.T, logs func(*store.Store) map[string]extensions.Proposer) (*httptest.Server, *store.Store) {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, DeskToken: "desk-secret", DefaultRepo: "/src/app",
		Extensions: []extensions.Extension{votes()}, Logs: logs(s)}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	if code, body := call(t, server, "POST", "/api/tasks", `{"title":"Vote"}`); code != 200 {
		t.Fatalf("create task-1: %d %s", code, body)
	}
	return server, s
}

// checkedVotes is votes' log as serve makes it.
func checkedVotes(s *store.Store) map[string]extensions.Proposer {
	return map[string]extensions.Proposer{"votes": extensions.Checked(votes(), events.Registration{}, s)}
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

// An extension's route is behind the wrapper its Auth names: UI takes only the
// UI token, Desk the desk's too, and a route naming no Auth is UI's.
func TestAnExtensionRouteIsBehindItsAuth(t *testing.T) {
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
		t.Errorf("an extension's read says position %q, want %s", got, want)
	}
}

// An extension's route writes through its checked proposer, as you: a type it
// lists in Writes is written, one it doesn't is refused with a 409 and
// nothing is stored.
func TestAnExtensionRouteWritesOnlyWhatItDeclares(t *testing.T) {
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

// An extension the server was given no log for writes nothing, even a type it
// lists in Writes; its reads still answer.
func TestAnExtensionRouteWithNoLogWritesNothing(t *testing.T) {
	server, s := votesServer(t, func(*store.Store) map[string]extensions.Proposer { return nil })
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

// An extension's route whose pattern a core route or another extension's has
// already fails to mount, naming the extension and the pattern, whether the
// core route is one of the first or the last the server registers.
func TestAnExtensionRouteThatClashesNamesItsExtension(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	one := func(name, pattern string) extensions.Extension {
		return extensions.Extension{Name: name, Routes: func(extensions.Core) []extensions.Route {
			return []extensions.Route{{Pattern: pattern, Handler: func(http.ResponseWriter, *http.Request) {}}}
		}}
	}
	for _, step := range []struct {
		list []extensions.Extension
		want string
	}{
		{[]extensions.Extension{one("votes", "GET /api/tasks")}, `the votes extension's route "GET /api/tasks" can't be mounted`},
		{[]extensions.Extension{one("votes", "DELETE /api/integrations/connections/{id}")}, `the votes extension's route "DELETE /api/integrations/connections/{id}" can't be mounted`},
		{[]extensions.Extension{one("votes", "GET /api/votes"), one("polls", "GET /api/votes")}, `the polls extension's route "GET /api/votes" can't be mounted`},
	} {
		got := func() (clash any) {
			defer func() { clash = recover() }()
			NewServer(s, state, Config{UIToken: token, Extensions: step.list}, http.NotFoundHandler(), nil).Handler()
			return nil
		}()
		if message, _ := got.(string); !strings.HasPrefix(message, step.want) {
			t.Errorf("mounting %s: panicked with %v, want %q", step.list[len(step.list)-1].Name, got, step.want)
		}
	}
}

const hookSecret = "hook-secret"

// signedWith checks a body as a git host does: X-Signature is the hex
// HMAC-SHA256 of the body under secret.
func signedWith(secret string) func(r *http.Request, body []byte) error {
	return func(r *http.Request, body []byte) error {
		given, err := hex.DecodeString(r.Header.Get("X-Signature"))
		if err != nil || len(given) == 0 {
			return errors.New("no signature")
		}
		if !hmac.Equal(given, signature(secret, body)) {
			return errors.New("the signature doesn't match")
		}
		return nil
	}
}

func signature(secret string, body []byte) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return mac.Sum(nil)
}

// callHook posts body to the votes webhook with headers.
func callHook(t *testing.T, server *httptest.Server, body string, headers map[string]string) (int, string) {
	t.Helper()
	request, _ := http.NewRequest("POST", server.URL+"/api/votes/hook", strings.NewReader(body))
	for name, value := range headers {
		request.Header.Set(name, value)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(content)
}

// A signed route takes a request whose signature verify accepts, gives the
// handler its body, and writes as System.
func TestASignedRouteWritesAsSystem(t *testing.T) {
	server, s := votesServer(t, checkedVotes)
	body := `{"pipeline":"failed"}`
	code, reply := callHook(t, server, body, map[string]string{"X-Signature": hex.EncodeToString(signature(hookSecret, []byte(body)))})
	if code != http.StatusOK {
		t.Fatalf("a good signature: %d %s", code, reply)
	}
	log, _ := s.Log("task-1")
	last := log[len(log)-1]
	if last.Type != events.IssueRaised || last.Actor != httpapi.System || last.Actor.Kind != events.System || last.Data["reason"] != body {
		t.Errorf("the hook should raise an issue as System with its body: %+v", last)
	}
}

// A signed route refuses a request whose signature is wrong or missing,
// even one carrying the UI token, and its handler writes nothing.
func TestASignedRouteRefusesABadSignature(t *testing.T) {
	server, s := votesServer(t, checkedVotes)
	body := `{"pipeline":"failed"}`
	for _, c := range []struct {
		name    string
		headers map[string]string
	}{
		{"a bad signature", map[string]string{"X-Signature": hex.EncodeToString(signature("not-the-secret", []byte(body)))}},
		{"no signature", nil},
		{"the UI token and no signature", map[string]string{"Authorization": "Bearer " + token}},
		{"the UI token and a bad signature", map[string]string{"Authorization": "Bearer " + token, "X-Signature": "00ff"}},
		{"the desk's token", map[string]string{"Authorization": "Bearer desk-secret"}},
	} {
		before := s.LastPosition()
		if code, reply := callHook(t, server, body, c.headers); code != http.StatusUnauthorized {
			t.Errorf("%s: %d %s, want 401", c.name, code, reply)
		}
		if after := s.LastPosition(); after != before {
			t.Errorf("%s: the log moved from %d to %d", c.name, before, after)
		}
	}
}

// A signed route's body is limited as a JSON body is: over MaxBody, it is
// refused before verify sees it.
func TestASignedRouteRefusesAnOversizedBody(t *testing.T) {
	server, s := votesServer(t, checkedVotes)
	body := strings.Repeat("x", httpapi.MaxBody+1)
	before := s.LastPosition()
	code, reply := callHook(t, server, body, map[string]string{"X-Signature": hex.EncodeToString(signature(hookSecret, []byte(body)))})
	if code != http.StatusBadRequest {
		t.Errorf("an oversized body: %d %s, want 400", code, reply)
	}
	if after := s.LastPosition(); after != before {
		t.Errorf("the log moved from %d to %d", before, after)
	}
}

// A body of exactly MaxBody is within the limit: verify sees all of it and
// the handler gets it.
func TestASignedRouteTakesABodyOfExactlyMaxBody(t *testing.T) {
	server, s := votesServer(t, checkedVotes)
	body := strings.Repeat("x", httpapi.MaxBody)
	code, reply := callHook(t, server, body, map[string]string{"X-Signature": hex.EncodeToString(signature(hookSecret, []byte(body)))})
	if code != http.StatusOK {
		t.Fatalf("a body of MaxBody: %d %s, want 200", code, reply)
	}
	log, _ := s.Log("task-1")
	if last := log[len(log)-1]; last.Type != events.IssueRaised || last.Data["reason"] != body {
		t.Errorf("the hook should raise an issue with the whole body: %s", last.Type)
	}
}

// A hand-built request may carry no body at all: signed reads it as empty,
// so verify refuses it for its missing signature instead of a panic.
func TestASignedRouteTakesARequestWithNoBody(t *testing.T) {
	reached := false
	handler := signed(signedWith(hookSecret), func(w http.ResponseWriter, r *http.Request) { reached = true })
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, &http.Request{Method: "POST", Header: http.Header{}})
	if response.Code != http.StatusUnauthorized || reached {
		t.Errorf("no body: %d, handler reached %v; want 401 and not reached", response.Code, reached)
	}
}

// The board's columns list Shipping only while an agent ships work or a
// task is in Shipping: Software's deployer brings it, a Writing team alone
// doesn't, and a task left in Shipping keeps it after its shipper goes.
func TestTheBoardShowsShippingOnlyWhileItMeansSomething(t *testing.T) {
	columns := func(server *httptest.Server) string {
		t.Helper()
		code, body := call(t, server, "GET", "/api/features", "")
		if code != 200 {
			t.Fatalf("features: %d %s", code, body)
		}
		var reply struct{ Board struct{ Columns []string } }
		if err := json.Unmarshal([]byte(body), &reply); err != nil {
			t.Fatal(err)
		}
		return strings.Join(reply.Board.Columns, " ")
	}
	const all, noShipping = "backlog todo in-progress review shipping done", "backlog todo in-progress review done"
	must := func(server *httptest.Server, method, path, body string) {
		t.Helper()
		if code, reply := call(t, server, method, path, body); code != 200 {
			t.Fatalf("%s %s: %d %s", method, path, code, reply)
		}
	}

	writing, _ := newTestServer(t)
	if got := columns(writing); got != all {
		t.Errorf("Software, whose deployer ships: %q, want %q", got, all)
	}
	must(writing, "POST", "/api/teams", `{"template":"Writing"}`)
	must(writing, "DELETE", "/api/teams/Software", "")
	if got := columns(writing); got != noShipping {
		t.Errorf("Writing only: %q, want %q", got, noShipping)
	}
	must(writing, "POST", "/api/teams", `{"template":"Software"}`)
	if got := columns(writing); got != all {
		t.Errorf("Software added back: %q, want %q", got, all)
	}

	shipped, _ := newTestServer(t)
	must(shipped, "POST", "/api/tasks", `{"title":"Ship it"}`)
	must(shipped, "POST", "/api/tasks/task-1/direct", `{"to":"shipping"}`)
	must(shipped, "DELETE", "/api/agents/deployer", "")
	if got := columns(shipped); got != all {
		t.Errorf("no agent ships, but task-1 is in Shipping: %q, want %q", got, all)
	}
	must(shipped, "POST", "/api/tasks/task-1/direct", `{"to":"done"}`)
	if got := columns(shipped); got != noShipping {
		t.Errorf("no agent ships and task-1 is done: %q, want %q", got, noShipping)
	}
}

const probeSettings = `
[probe]
host = "first.example"

[probe.secrets]
token = "probe-token"

[other.secrets]
hook = "other-secret"
`

// probe is an extension whose route says what its settings hold: its host, its
// token read as a setting and as a secret, and other's secret.
func probe() extensions.Extension {
	return extensions.Extension{Name: "probe", Routes: func(c extensions.Core) []extensions.Route {
		return []extensions.Route{{Pattern: "GET /api/probe", Handler: func(w http.ResponseWriter, r *http.Request) {
			host, _ := c.Settings.Setting("host")
			asSetting, _ := c.Settings.Setting("token")
			token, _ := c.Settings.Secret("token")
			hook, _ := c.Settings.Secret("hook")
			httpapi.Reply(w, map[string]string{"host": host, "tokenAsSetting": asSetting, "token": token, "hook": hook})
		}}}
	}}
}

// probeServer serves probe with settings, as serve gives them.
func probeServer(t *testing.T, settings func(extension string) extensions.Settings) *httptest.Server {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, Extensions: []extensions.Extension{probe()}, Settings: settings},
		http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return server
}

// An extension's route reads its own setting and secret in the settings file,
// never another extension's or a secret as a setting, and sees an edit to the
// file with no restart.
func TestARouteReadsItsOwnSettings(t *testing.T) {
	path := filepath.Join(t.TempDir(), "features.toml")
	writeFile(t, path, probeSettings)
	live := settingsfile.Open(path)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go live.Watch(ctx, 5*time.Millisecond)
	server := probeServer(t, live.For)

	want := `{"hook":"","host":"first.example","token":"probe-token","tokenAsSetting":""}`
	if code, body := call(t, server, "GET", "/api/probe", ""); code != 200 || strings.TrimSpace(body) != want {
		t.Errorf("probe: %d %s\nwant %s", code, body, want)
	}
	writeFile(t, path, strings.Replace(probeSettings, "first.example", "second.example", 1))
	for deadline := time.Now().Add(5 * time.Second); ; time.Sleep(5 * time.Millisecond) {
		if _, body := call(t, server, "GET", "/api/probe", ""); strings.Contains(body, `"host":"second.example"`) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the edit never reached the route")
		}
	}
}

// A server given no settings file gives the routes none.
func TestARouteWithNoSettingsFileHasNone(t *testing.T) {
	server := probeServer(t, nil)
	want := `{"hook":"","host":"","token":"","tokenAsSetting":""}`
	if code, body := call(t, server, "GET", "/api/probe", ""); code != 200 || strings.TrimSpace(body) != want {
		t.Errorf("probe: %d %s\nwant %s", code, body, want)
	}
}

func writeFile(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}
