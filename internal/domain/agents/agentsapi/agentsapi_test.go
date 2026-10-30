package agentsapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/core/rules"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/extensions/ideas"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

// The agents' and teams' routes, tested over HTTP through the server serve
// runs, as web mounts them: that is where their token check is.

const token = "ui-secret"

// mirrors internal/web/api_test.go
func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	return newTestServerOn(t, filepath.Join(t.TempDir(), "agentry.db"), tasks.Limits{})
}

// newTestServerOn is newTestServer on the log at path, with the new tasks'
// limits in limits.
//
// mirrors internal/web/api_test.go
func newTestServerOn(t *testing.T, path string, limits tasks.Limits) (*httptest.Server, *store.Store) {
	t.Helper()
	s, state := storetest.Open(t, path)
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	hooks := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hooks")) })
	server := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: token, DefaultRepo: "/src/app", Clones: filepath.Join(t.TempDir(), "repos"),
		Blobs: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}, Budget: limits.Budget, MaxTurns: limits.MaxTurns}, hooks, nil).Handler())
	t.Cleanup(server.Close)
	return server, s
}

// mirrors internal/web/api_test.go
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

func TestTheAgentsRoutesTakeTheUIToken(t *testing.T) {
	server, _ := newTestServer(t)
	for _, route := range []string{"GET /api/agents", "GET /api/teams/templates", "POST /api/teams", "DELETE /api/teams/Software",
		"POST /api/teams/Software/rename", "PUT /api/teams/Software", "POST /api/teams/Software/template", "POST /api/teams/Software/link",
		"DELETE /api/teams/templates/mine", "GET /api/teams/templates/mine/export", "POST /api/teams/templates/import", "POST /api/agents",
		"PUT /api/agents/engineer", "DELETE /api/agents/engineer", "POST /api/agents/engineer/rename",
		"PUT /api/agents/engineer/knowledge", "GET /api/agents/engineer/knowledge"} {
		method, path, _ := strings.Cut(route, " ")
		request, _ := http.NewRequest(method, server.URL+path, nil)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s without the token: %d, want 401", route, response.StatusCode)
		}
	}
}

// taskDetail is a task as GET /api/tasks/{task} gives it: what these tests read.
type taskDetail struct {
	Coordinator string
	Trail       []struct{ To string }
}

// dataTeam is a custom team: its lead and two specialists.
//
// mirrors internal/web/api_test.go
const dataTeam = `{"team":"Data","agents":[
	{"name":"data-lead","engine":"claude","coordinates":true},
	{"name":"analyst","engine":"claude","description":"Analyse the numbers."},
	{"name":"charter","engine":"claude","description":"Draw the charts."}]}`

// policyServer, servePolicy, withoutReadMostly and policyEvents mirror
// their copies in internal/web/policy_test.go and
// internal/domain/policy/policyapi/policyapi_test.go: change them together.

// policyServer serves the default team and a policy file holding text.
type policyServer struct {
	*httptest.Server
	log  *store.Store
	live *policy.Live
	path string
}

// servePolicy serves the policy text; configure, if given, changes the
// server's Config first.
func servePolicy(t *testing.T, text string, configure ...func(*web.Config)) policyServer {
	t.Helper()
	data := t.TempDir()
	db := filepath.Join(data, "agentry.db")
	s, state := storetest.Open(t, db)
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	path := filepath.Join(t.TempDir(), "policy.toml")
	os.WriteFile(path, []byte(text), 0o600)
	live, err := policy.OpenLive(path, "/home/dev", []string{db, path})
	if err != nil {
		t.Fatal(err)
	}
	config := web.Config{UIToken: token, DeskToken: "desk-secret", HelperToken: "helper-secret",
		Policy: live, Local: workers.Machine{Version: "v0.9.0"}, Worktrees: filepath.Join(data, "worktrees")}
	for _, c := range configure {
		c(&config)
	}
	server := httptest.NewServer(web.NewServer(s, state, config, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return policyServer{Server: server, log: s, live: live, path: path}
}

// withoutReadMostly is the default policy as a version before read-mostly had it.
func withoutReadMostly(t *testing.T) string {
	t.Helper()
	shipped := string(policy.Default())
	start, end := strings.Index(shipped, "[profiles.read-mostly]"), strings.Index(shipped, "[profiles.autonomous]")
	if start < 0 || end < start {
		t.Fatal("the default policy should have read-mostly before autonomous")
	}
	return shipped[:start] + shipped[end:]
}

func (p policyServer) policyEvents(t *testing.T) []events.Event {
	t.Helper()
	stream, err := p.log.Stream(events.PolicyStream)
	if err != nil {
		t.Fatal(err)
	}
	return stream
}

// mirrors internal/web/api_test.go
//
// newTestServerChecking is newTestServer whose log also runs check after
// the rules, so a test can refuse one proposal.
func newTestServerChecking(t *testing.T, check func(events.Proposal) *events.Rejection) (*httptest.Server, *store.Store) {
	t.Helper()
	server, s, _ := newTestStateChecking(t, check)
	return server, s
}

// mirrors internal/web/api_test.go
//
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

// Agents are added, changed and removed from the page; the rules decide.
func TestAgentsAreEditedFromThePage(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/agents", `{"name":"designer","engine":"claude","prompt":"Design the page.","description":"Draws the UI"}`); code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	if code, body := call(t, server, "PUT", "/api/agents/engineer", `{"name":"ignored","engine":"codex","model":"gpt-5","prompt":"Write Go.","workers":[]}`); code != 200 {
		t.Fatalf("change: %d %s", code, body)
	}
	if code, _ := call(t, server, "PUT", "/api/agents/engineer", `{"engine":"claude","prompt":"x","workers":["build-box"]}`); code != http.StatusConflict {
		t.Errorf("a worker that doesn't exist: %d", code)
	}
	if code, _ := call(t, server, "DELETE", "/api/agents/tech-lead", ""); code != http.StatusConflict {
		t.Errorf("removing the only coordinator: %d", code)
	}
	if code, body := call(t, server, "DELETE", "/api/agents/designer", ""); code != 200 {
		t.Errorf("remove: %d %s", code, body)
	}

	_, body := call(t, server, "GET", "/api/agents", "")
	var roster []agents.Agent
	json.Unmarshal([]byte(body), &roster)
	engineer := roster[1]
	if len(roster) != 5 || engineer.Name != "engineer" || engineer.Engine != "codex" || engineer.Model != "gpt-5" || engineer.Workers == nil {
		t.Errorf("roster: %+v", roster)
	}
	if !strings.Contains(body, `"workers":[]`) {
		t.Error("an agent with no workers should list none, not null")
	}
}

// You can edit what an agent knows, and see who changed it.
func TestYouEditWhatAnAgentKnows(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "PUT", "/api/agents/engineer/knowledge", `{"knowledge":"  Prefer table tests.  "}`); code != 200 {
		t.Fatalf("set: %d %s", code, body)
	}
	call(t, server, "PUT", "/api/agents/engineer", `{"engine":"claude","prompt":"Write Go."}`) // settings, not knowledge
	_, body := call(t, server, "GET", "/api/agents", "")
	var roster []agents.Agent
	json.Unmarshal([]byte(body), &roster)
	if roster[1].Knowledge != "Prefer table tests." {
		t.Errorf("knowledge: %q", roster[1].Knowledge)
	}
	_, body = call(t, server, "GET", "/api/agents/engineer/knowledge", "")
	if !strings.Contains(body, `"by":"you"`) || !strings.Contains(body, "Prefer table tests.") {
		t.Errorf("history: %s", body)
	}
}

// The history lists the same versions with the same texts whether the log
// keeps each as a full copy or as a patch on the one before, across a rename.
func TestTheKnowledgeHistoryIsTheSameFromPatches(t *testing.T) {
	versions := []string{"Use gofmt.", "Use gofmt.\nRun go vet.", "Use gofmt -s.\nRun go vet.", "Use gofmt -s.\nRun go vet ./..."}
	history := func(patches bool) string {
		server, s := newTestServer(t)
		engineer := agents.Agent{Name: "engineer"}
		by, before := events.Actor{Kind: events.Agent, ID: "engineer"}, ""
		for i, text := range versions {
			if i == 2 {
				s.Propose(engineer.Renaming(httpapi.You, "builder"))
				engineer.Name, by.ID = "builder", "builder"
			}
			p := engineer.Learned(by, text)
			if patches && i > 0 {
				p = engineer.Patching(by, agents.Diff(before, text))
			}
			if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
				t.Fatal(err, rejection)
			}
			before = text
		}
		_, body := call(t, server, "GET", "/api/agents/builder/knowledge", "")
		var changes []map[string]any
		json.Unmarshal([]byte(body), &changes)
		for _, change := range changes {
			delete(change, "at") // each log was written at its own time
		}
		listed, _ := json.Marshal(changes)
		return string(listed)
	}
	copies, patches := history(false), history(true)
	if want := `[{"by":"builder","knowledge":"Use gofmt -s.\nRun go vet ./..."},{"by":"builder","knowledge":"Use gofmt -s.\nRun go vet."}]`; copies != want {
		t.Errorf("from full copies: %s\nwant %s", copies, want)
	}
	if patches != copies {
		t.Errorf("from patches: %s\nfrom full copies: %s", patches, copies)
	}
}

// After a secret in an agent's knowledge is redacted, its history lists
// the same versions, each showing [redacted] where the secret was, those
// rebuilt from patches on it too.
func TestARedactedKnowledgeKeepsItsHistory(t *testing.T) {
	const secret = "sk-7c1e90"
	server, s := newTestServer(t)
	engineer, before := agents.Agent{Name: "engineer"}, ""
	for i, text := range []string{"Use gofmt.", "Use gofmt.\nThe token is " + secret + ".", "Use gofmt -s.\nThe token is " + secret + ".", "Use gofmt -s.\nThe token is " + secret + ".\nRun go vet."} {
		p := engineer.Patching(httpapi.You, agents.Diff(before, text))
		if i == 0 {
			p = engineer.Learned(httpapi.You, text)
		}
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatal(err, rejection)
		}
		before = text
	}
	history := func() []string {
		_, body := call(t, server, "GET", "/api/agents/engineer/knowledge", "")
		var changes []struct{ Knowledge string }
		json.Unmarshal([]byte(body), &changes)
		var texts []string
		for _, change := range changes {
			texts = append(texts, change.Knowledge)
		}
		return texts
	}
	listed := history()
	if _, rejection, err := s.Redact(events.AgentsStream, secret, httpapi.You, ""); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	var want []string
	for _, text := range listed {
		want = append(want, strings.ReplaceAll(text, secret, "[redacted]"))
	}
	if got := history(); len(listed) != 4 || !reflect.DeepEqual(got, want) {
		t.Errorf("the history after: %q\nwant %q", got, want)
	}
}

func TestATeamIsAddedFromATemplateAndTakesTasks(t *testing.T) {
	server, s := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/teams", `{"template":"research"}`); code != 200 {
		t.Fatalf("adding the Research team: %d %s", code, body)
	}
	stream, _ := s.Stream(events.AgentsStream)
	roster := agents.FromEvents(stream)
	if lead, ok := roster.Coordinator("Research"); !ok || lead.Name != "research-lead" || len(roster.Assignees("Research")) != 3 {
		t.Fatalf("the Research team: %v", roster.Teams())
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Size the market","team":"Research"}`)
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if detail.Coordinator != "research-lead" {
		t.Errorf("a Research task goes to its lead, not %q", detail.Coordinator)
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Fix the login"}`)
	_, body = call(t, server, "GET", "/api/tasks/task-2", "")
	json.Unmarshal([]byte(body), &detail)
	if detail.Coordinator != "tech-lead" {
		t.Errorf("a task that names no team goes to the first team's coordinator, not %q", detail.Coordinator)
	}
}

// A team you define is added as you wrote it, its coordinator flagged, and
// an agent you gave no prompt gets a default one.
func TestACustomTeamIsAddedFromYourAgents(t *testing.T) {
	server, _ := newTestServer(t)
	request := `{"team":" Data ","agents":[
		{"name":"analyst","description":"Analyse the numbers.","engine":"claude","column":"review","color":"teal"},
		{"name":"data-lead","engine":"codex","model":"gpt-5","coordinates":true},
		{"name":"charter","engine":"claude","prompt":"Draw the charts.","column":"ship"}]}`
	if code, body := call(t, server, "POST", "/api/teams", request); code != 200 {
		t.Fatalf("adding the Data team: %d %s", code, body)
	}
	_, body := call(t, server, "GET", "/api/agents", "")
	var roster []agents.Agent
	json.Unmarshal([]byte(body), &roster)
	team := map[string]agents.Agent{}
	for _, agent := range roster {
		if agent.Team == "Data" {
			team[agent.Name] = agent
		}
	}
	lead, analyst, charter := team["data-lead"], team["analyst"], team["charter"]
	if len(team) != 3 || lead.Template != "" || analyst.Template != "" || charter.Template != "" {
		t.Fatalf("the Data team: %+v", team)
	}
	if !lead.Coordinates || analyst.Coordinates || lead.Engine != "codex" || lead.Model != "gpt-5" || !strings.Contains(lead.Prompt, "You are the coordinator") {
		t.Errorf("the lead: %+v", lead)
	}
	if !strings.Contains(analyst.Prompt, "Analyse the numbers") || analyst.Column != agents.Reviews || analyst.Color != "teal" {
		t.Errorf("the analyst: %+v", analyst)
	}
	if charter.Prompt != "Draw the charts." || charter.Column != agents.Ships {
		t.Errorf("the charter keeps its own prompt: %+v", charter)
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Chart the sign-ups","team":"Data"}`)
	_, body = call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if detail.Coordinator != "data-lead" {
		t.Errorf("a Data task goes to its lead, not %q", detail.Coordinator)
	}
}

// A custom team that can't be added is refused whole: no agent of it is
// made, even when only the last one is wrong. A team is copied only from a
// built-in template, not from one of yours.
func TestACustomTeamIsRefusedWhole(t *testing.T) {
	server, s := newTestServer(t)
	yours := `{"id":"ideas","team":"Ideas","agents":[{"name":"dreamer","engine":"claude","prompt":"Dream.","coordinates":true}]}`
	if code, body := call(t, server, "POST", "/api/teams/templates/import", yours); code != 200 {
		t.Fatalf("import: %d %s", code, body)
	}
	lead := `{"name":"data-lead","engine":"claude","coordinates":true}`
	for request, want := range map[string]string{
		`{"team":"Data","copiedFrom":"nowhere","agents":[` + lead + `]}`:                                     "nowhere isn't the id of one",
		`{"team":"Data","copiedFrom":"ideas","agents":[` + lead + `]}`:                                       "ideas isn't the id of one",
		`{"template":"research","copiedFrom":"research"}`:                                                    "not both",
		`{"team":"Data","agents":[` + lead + `,{"name":"analyst","engine":"claude","coordinates":true}]}`:    "exactly one coordinator",
		`{"team":"writing","agents":[` + lead + `]}`:                                                         "Writing template",
		`{"team":"software","agents":[` + lead + `]}`:                                                        "a team called software already",
		`{"team":"Data","agents":[` + lead + `,{"name":"analyst","engine":"gemini"}]}`:                       `analyst's engine must be one of claude, codex, not "gemini"`,
		`{"team":"Data","agents":[` + lead + `,{"name":"analyst"}]}`:                                         `analyst's engine must be one of claude, codex, not ""`,
		`{"team":"Data","agents":[` + lead + `,{"name":"analyst","engine":"claude","workers":["nowhere"]}]}`: "analyst: no worker called nowhere: no machine has joined yet",
		`{"template":"research","team":"Data","agents":[` + lead + `]}`:                                      "not both",
		`{"template":"research","agents":[` + lead + `]}`:                                                    "not both",
		`{}`:              "name a template, or a team and its agents",
		`{"team":"Data"}`: "the Data team needs agents",
	} {
		before, _ := s.Stream(events.AgentsStream)
		code, body := call(t, server, "POST", "/api/teams", request)
		var refusal struct{ Error string }
		if err := json.Unmarshal([]byte(body), &refusal); code != http.StatusBadRequest || err != nil || !strings.Contains(refusal.Error, want) {
			t.Errorf("%s: want a 400 saying %q, got %d %s", request, want, code, body)
		}
		if after, _ := s.Stream(events.AgentsStream); len(after) != len(before) {
			t.Errorf("%s: %d agent events written", request, len(after)-len(before))
		}
	}
}

// A team copied from a built-in template is made from it, every agent of
// it, so it keeps the template's extensions: one copied from the Visionaries
// suggests ideas.
func TestACopiedTeamIsMadeFromItsTemplate(t *testing.T) {
	server, s, state := newTestStateChecking(t, func(events.Proposal) *events.Rejection { return nil })
	request := `{"team":"Dreamers","copiedFrom":" visionaries ","agents":[
		{"name":"dream-lead","engine":"claude","coordinates":true},
		{"name":"dreamer","engine":"claude","prompt":"Dream."}]}`
	if code, body := call(t, server, "POST", "/api/teams", request); code != 200 {
		t.Fatalf("adding the Dreamers team: %d %s", code, body)
	}
	team := teamIn(t, s, "Dreamers")
	for _, agent := range team {
		if agent.Template != "visionaries" {
			t.Errorf("%s is made from %q", agent.Name, agent.Template)
		}
	}
	if lead, _ := state.Roster().Coordinator("Dreamers"); len(team) != 2 || !ideas.Suggests(lead, agents.BasesIn(state)) {
		t.Errorf("the Dreamers' coordinator %+v doesn't suggest ideas", lead)
	}
}

// A refusal names the agent as it will be called, without the spaces sent
// around its name.
func TestACustomTeamRefusalNamesTheTrimmedAgent(t *testing.T) {
	server, _ := newTestServer(t)
	request := `{"team":"Data","agents":[{"name":"data-lead","engine":"claude","coordinates":true},{"name":" analyst ","engine":"gemini"}]}`
	_, body := call(t, server, "POST", "/api/teams", request)
	var refusal struct{ Error string }
	json.Unmarshal([]byte(body), &refusal)
	if want := `analyst's engine must be one of claude, codex, not "gemini"`; refusal.Error != want {
		t.Errorf("want %q, got %q", want, refusal.Error)
	}
}

// An agent of a custom team can run on the machine Agentry runs on.
func TestACustomTeamCanRunOnThisMachine(t *testing.T) {
	server, s := newTestServer(t)
	request := `{"team":"Data","agents":[{"name":"data-lead","engine":"claude","coordinates":true,"workers":["` + workers.Local + `"]}]}`
	if code, body := call(t, server, "POST", "/api/teams", request); code != 200 {
		t.Fatalf("adding the Data team: %d %s", code, body)
	}
	stream, _ := s.Stream(events.AgentsStream)
	if lead, _ := agents.FromEvents(stream).Get("data-lead"); !slices.Equal(lead.Workers, []string{workers.Local}) {
		t.Errorf("the lead runs on %v", lead.Workers)
	}
}

// Editing one agent never breaks its team: the lead can't stop
// coordinating or go while the team has others, and no one else can
// become a second coordinator.
func TestEditingAnAgentKeepsItsTeamsOneCoordinator(t *testing.T) {
	server, s := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/teams", dataTeam); code != 200 {
		t.Fatalf("adding the Data team: %d %s", code, body)
	}
	before, _ := s.Stream(events.AgentsStream)
	for _, c := range []struct{ method, path, body, refusal string }{
		{"PUT", "/api/agents/analyst", `{"engine":"claude","prompt":"Lead.","team":"Data","coordinates":true}`,
			"the Data team already has a coordinator, data-lead: a team has exactly one"},
		{"PUT", "/api/agents/data-lead", `{"engine":"claude","prompt":"Lead.","team":"Data"}`, "this would leave the Data team with no coordinator"},
		{"DELETE", "/api/agents/data-lead", "", "this would leave the Data team with no coordinator"},
	} {
		if code, body := call(t, server, c.method, c.path, c.body); code != http.StatusConflict || !strings.Contains(body, c.refusal) {
			t.Errorf("%s %s: %d %s, want a 409 saying %q", c.method, c.path, code, body, c.refusal)
		}
	}
	if after, _ := s.Stream(events.AgentsStream); len(after) != len(before) {
		t.Errorf("%d agent events written", len(after)-len(before))
	}
}

// An agent added to or moved into a team named in another case joins that
// team, with its spelling, instead of starting a twin of it.
func TestAnAgentJoinsATeamNamedInAnyCase(t *testing.T) {
	server, s := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/teams", dataTeam); code != 200 {
		t.Fatalf("adding the Data team: %d %s", code, body)
	}
	for _, c := range []struct{ method, path, body string }{
		{"POST", "/api/agents", `{"name":"modeller","engine":"claude","prompt":"Model.","team":"data"}`},
		{"PUT", "/api/agents/modeller", `{"engine":"claude","prompt":"Model more.","team":" DATA "}`},
	} {
		if code, body := call(t, server, c.method, c.path, c.body); code != 200 {
			t.Fatalf("%s %s: %d %s", c.method, c.path, code, body)
		}
		stream, _ := s.Stream(events.AgentsStream)
		if modeller, _ := agents.FromEvents(stream).Get("modeller"); modeller.Team != "Data" {
			t.Errorf("%s %s: the agent is in team %q, want %q", c.method, c.path, modeller.Team, "Data")
		}
	}
	if code, body := call(t, server, "PUT", "/api/agents/analyst", `{"engine":"claude","prompt":"Lead.","team":"data","coordinates":true}`); code != http.StatusConflict ||
		!strings.Contains(body, "the Data team already has a coordinator, data-lead") {
		t.Errorf("a second coordinator named in another case: %d %s", code, body)
	}
}

// A custom team is removed as a template's is: refused while a task needs
// it, then with all its agents, its coordinator last.
func TestACustomTeamIsRemovedWithItsAgents(t *testing.T) {
	server, s := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/teams", dataTeam); code != 200 {
		t.Fatalf("adding the Data team: %d %s", code, body)
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Chart the sign-ups","team":"Data"}`)
	if code, body := call(t, server, "DELETE", "/api/teams/data", ""); code != http.StatusConflict || !strings.Contains(body, "task-1") {
		t.Errorf("a team a task needs: %d %s", code, body)
	}
	if code, body := call(t, server, "POST", "/api/tasks/task-1/direct", `{"to":"done"}`); code != 200 {
		t.Fatalf("finishing task-1: %d %s", code, body)
	}
	before, _ := s.Stream(events.AgentsStream)
	if code, body := call(t, server, "DELETE", "/api/teams/data", ""); code != 200 {
		t.Fatalf("removing Data: %d %s", code, body)
	}
	after, _ := s.Stream(events.AgentsStream)
	var removed []string
	for _, e := range after[len(before):] {
		if e.Type == events.AgentRemoved {
			removed = append(removed, e.Text("name"))
		}
	}
	if strings.Join(removed, ",") != "analyst,charter,data-lead" {
		t.Errorf("removed %v, want the specialists, then the lead", removed)
	}
	if slices.Contains(agents.FromEvents(after).Teams(), "Data") {
		t.Error("the Data team is still there")
	}
}

// A team renamed, named in any case, moves with its agents, and its open
// tasks stay with its coordinator; a name taken by another team or a
// template's is refused.
func TestATeamIsRenamedWithItsTasks(t *testing.T) {
	server, s := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/teams", dataTeam); code != 200 {
		t.Fatalf("adding the Data team: %d %s", code, body)
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Chart the sign-ups","team":"Data"}`)

	if code, body := call(t, server, "POST", "/api/teams/data/rename", `{"to":" Analytics "}`); code != 200 || body != `{"team":"Data","to":"Analytics"}`+"\n" {
		t.Fatalf("renaming data: %d %s", code, body)
	}
	stream, _ := s.Stream(events.AgentsStream)
	roster := agents.FromEvents(stream)
	if lead, ok := roster.Coordinator("Analytics"); !ok || lead.Name != "data-lead" || len(roster.Assignees("Analytics")) != 2 || slices.Contains(roster.Teams(), "Data") {
		t.Errorf("the team after its rename: %v", roster.Teams())
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if detail.Coordinator != "data-lead" {
		t.Errorf("the open task stays with data-lead, not %q", detail.Coordinator)
	}
	call(t, server, "POST", "/api/tasks", `{"title":"Chart the churn","team":"analytics"}`)
	_, body = call(t, server, "GET", "/api/tasks/task-2", "")
	json.Unmarshal([]byte(body), &detail)
	if detail.Coordinator != "data-lead" {
		t.Errorf("a task for the new name goes to data-lead, not %q", detail.Coordinator)
	}

	for _, c := range []struct {
		team, to string
		code     int
		reason   string
	}{
		{"Data", "Numbers", http.StatusNotFound, "there is no team called Data"},
		{"Analytics", "SOFTWARE", http.StatusConflict, "there is a team called SOFTWARE already"},
		{"Analytics", "writing", http.StatusConflict, "writing is the name of the Writing template's team; choose another"},
		{"Analytics", "", http.StatusConflict, "a team needs a name"},
		{"Analytics", "ANALYTICS", http.StatusOK, ""},
		{"Software", "software", http.StatusOK, ""},
	} {
		code, body := call(t, server, "POST", "/api/teams/"+c.team+"/rename", `{"to":"`+c.to+`"}`)
		if code != c.code || !strings.Contains(body, c.reason) {
			t.Errorf("renaming %s to %q: %d %s, want %d %s", c.team, c.to, code, body, c.code, c.reason)
		}
	}
	// An agent of Software respelled stays in it when edited, not in a new Software.
	edited := `{"engine":"claude","prompt":"Lead.","coordinates":true,"team":"Software"}`
	if code, body := call(t, server, "PUT", "/api/agents/tech-lead", edited); code != 200 {
		t.Fatalf("editing tech-lead: %d %s", code, body)
	}
	stream, _ = s.Stream(events.AgentsStream)
	if lead, _ := agents.FromEvents(stream).Get("tech-lead"); lead.Team != "software" {
		t.Errorf("tech-lead is in team %q, want software", lead.Team)
	}
}

// An agent sent with no team joins the default team, as agents from before
// teams do, and once Software is respelled it joins the respelled team, not a
// coordinator-less twin of it.
func TestAnAgentWithNoTeamJoinsTheDefaultTeam(t *testing.T) {
	server, s := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/agents", `{"name":"helper","engine":"claude","prompt":"Help."}`); code != 200 {
		t.Fatalf("adding helper: %d %s", code, body)
	}
	stream, _ := s.Stream(events.AgentsStream)
	if helper, _ := agents.FromEvents(stream).Get("helper"); helper.Team != "" {
		t.Errorf("helper is in team %q, want the default team kept as \"\"", helper.Team)
	}

	if code, body := call(t, server, "POST", "/api/teams/software/rename", `{"to":"software"}`); code != 200 {
		t.Fatalf("respelling Software: %d %s", code, body)
	}
	if code, body := call(t, server, "POST", "/api/agents", `{"name":"aide","engine":"claude","prompt":"Help."}`); code != 200 {
		t.Fatalf("adding aide: %d %s", code, body)
	}
	stream, _ = s.Stream(events.AgentsStream)
	roster := agents.FromEvents(stream)
	if !slices.Equal(roster.Teams(), []string{"software"}) {
		t.Errorf("teams %v, want only software", roster.Teams())
	}
	if aide, _ := roster.Get("aide"); aide.Team != "software" {
		t.Errorf("aide is in team %q, want software", aide.Team)
	}

	if code, body := call(t, server, "PUT", "/api/agents/helper", `{"engine":"claude","prompt":"Help more.","team":""}`); code != 200 {
		t.Fatalf("editing helper: %d %s", code, body)
	}
	stream, _ = s.Stream(events.AgentsStream)
	roster = agents.FromEvents(stream)
	if !slices.Equal(roster.Teams(), []string{"software"}) {
		t.Errorf("after editing helper, teams %v, want only software", roster.Teams())
	}
	if helper, _ := roster.Get("helper"); helper.Team != "software" {
		t.Errorf("helper is in team %q, want software", helper.Team)
	}
}

func TestRenamingAnAgentKeepsItsTasksGoing(t *testing.T) {
	server, s := newTestServer(t)
	stream, _ := s.Stream(events.AgentsStream)
	lead, _ := agents.FromEvents(stream).Coordinator("")
	call(t, server, "POST", "/api/tasks", `{"title":"Fix the login"}`)
	call(t, server, "PUT", "/api/agents/"+lead.Name+"/knowledge", `{"knowledge":"Slice big tasks."}`)

	if code, body := call(t, server, "POST", "/api/agents/"+lead.Name+"/rename", `{"to":"lead-dev"}`); code != 200 {
		t.Fatalf("renaming: %d %s", code, body)
	}
	stream, _ = s.Stream(events.AgentsStream)
	renamed, ok := agents.FromEvents(stream).Get("lead-dev")
	if !ok || !renamed.Coordinates || renamed.Prompt != lead.Prompt || renamed.Knowledge != "Slice big tasks." {
		t.Errorf("the agent keeps everything but its name: %+v", renamed)
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	json.Unmarshal([]byte(body), &detail)
	if detail.Coordinator != "lead-dev" || detail.Trail[0].To != lead.Name {
		t.Errorf("the task goes on with lead-dev, and its history keeps the old name: coordinator %q, first step to %q", detail.Coordinator, detail.Trail[0].To)
	}
	if code, _ := call(t, server, "POST", "/api/agents/lead-dev/rename", `{"to":"engineer"}`); code != http.StatusConflict {
		t.Errorf("renaming to a name in use: %d", code)
	}
}

// A team goes with all its agents, once no unfinished task needs them; the
// last team stays.
func TestATeamIsRemovedWithItsAgents(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "DELETE", "/api/teams/Software", ""); code != http.StatusConflict || !strings.Contains(body, "the only one") {
		t.Errorf("the last team: %d %s", code, body)
	}
	if code, body := call(t, server, "POST", "/api/teams", `{"template":"Writing"}`); code != 200 {
		t.Fatalf("add Writing: %d %s", code, body)
	}
	call(t, server, "POST", "/api/tasks", `{"description":"Write the launch post","team":"Writing"}`)
	if code, body := call(t, server, "DELETE", "/api/teams/writing", ""); code != http.StatusConflict || !strings.Contains(body, "task-1") {
		t.Errorf("a team a task needs: %d %s", code, body)
	}
	if code, body := call(t, server, "DELETE", "/api/agents/editor", ""); code != http.StatusConflict || !strings.Contains(body, "task-1") {
		t.Errorf("an agent a task needs: %d %s", code, body)
	}
	if code, body := call(t, server, "DELETE", "/api/agents/fact-checker", ""); code != 200 {
		t.Errorf("an agent no task needs: %d %s", code, body)
	}
	call(t, server, "POST", "/api/teams", `{"template":"Research"}`)
	if code, body := call(t, server, "DELETE", "/api/teams/Research", ""); code != 200 {
		t.Fatalf("remove Research: %d %s", code, body)
	}
	_, body := call(t, server, "GET", "/api/agents", "")
	if strings.Contains(body, `"Research"`) || strings.Contains(body, "research-lead") {
		t.Errorf("Research should be gone: %s", body)
	}
}

// The templates say their ID and the extensions they turn on, so the page can
// find the team made from one by template, whatever it is called.
func TestTeamTemplatesSayTheirIDAndFeatures(t *testing.T) {
	server, _ := newTestServer(t)
	_, body := call(t, server, "GET", "/api/teams/templates", "")
	var templates []struct {
		ID       string
		Team     string
		Features []string
	}
	if err := json.Unmarshal([]byte(body), &templates); err != nil || len(templates) != len(agents.Templates) {
		t.Fatalf("templates: %v %s", err, body)
	}
	for i, template := range templates {
		want := agents.Templates[i]
		if template.ID != want.ID || template.Team != want.Team || !slices.Equal(template.Features, want.Features) {
			t.Errorf("template %d is %+v, not %s (%s) with %v", i, template, want.ID, want.Team, want.Features)
		}
	}
	if !strings.Contains(body, `"id":"visionaries"`) || !strings.Contains(body, `"features":["ideas"]`) || !strings.Contains(body, `"features":[]`) {
		t.Errorf("Visionaries should list ideas, and a template with none an empty list:\n%s", body)
	}
}

// Each template's agents come with what a team made from it gets, so the
// page can copy one: the prompt POST /api/teams {template} writes, its
// engine, profile, colour and column.
func TestTeamTemplatesSayWhatTheirAgentsGet(t *testing.T) {
	server, _ := newTestServer(t)
	_, body := call(t, server, "GET", "/api/teams/templates", "")
	var templates []struct {
		Team   string
		Agents []agents.Agent
	}
	if err := json.Unmarshal([]byte(body), &templates); err != nil || len(templates) != len(agents.Templates) {
		t.Fatalf("templates: %v %s", err, body)
	}
	for _, template := range templates {
		added := addedFrom(t, template.Team)
		if len(added) != len(template.Agents) {
			t.Fatalf("%s: the page shows %d agents, adding it makes %d", template.Team, len(template.Agents), len(added))
		}
		for i, shown := range template.Agents {
			made := added[i]
			if shown.Prompt == "" || shown.Engine == "" {
				t.Errorf("%s's %s has no prompt or engine: %+v", template.Team, shown.Name, shown)
			}
			if shown.Prompt != made.Prompt || shown.Engine != made.Engine || shown.Model != made.Model || shown.Profile != made.Profile ||
				shown.Color != made.Color || shown.Column != made.Column || shown.Coordinates != made.Coordinates {
				t.Errorf("%s's %s shows\n%+v\nbut is added as\n%+v", template.Team, shown.Name, shown, made)
			}
		}
	}
}

// addedFrom is the agents POST /api/teams {template} adds for this template,
// in the order it adds them, on a fresh server.
func addedFrom(t *testing.T, template string) []agents.Agent {
	t.Helper()
	server, s := newTestServer(t)
	stream, _ := s.Stream(events.AgentsStream)
	before := len(stream)
	request, _ := json.Marshal(map[string]any{"template": template, "addMissingProfiles": true})
	if code, body := call(t, server, "POST", "/api/teams", string(request)); code != 200 {
		t.Fatalf("adding %s: %d %s", template, code, body)
	}
	stream, _ = s.Stream(events.AgentsStream)
	return agents.FromEvents(stream[before:])
}

// Editing an agent keeps the template it came from, whatever the page sends,
// so its team keeps the extensions the template turns on.
func TestEditingAnAgentKeepsItsTemplate(t *testing.T) {
	server, s := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/teams", `{"template":"Visionaries"}`); code != 200 {
		t.Fatalf("add Visionaries: %d %s", code, body)
	}
	for _, change := range []string{
		`{"engine":"claude","profile":"read-mostly","prompt":"Lead.","coordinates":true,"team":"Visionaries"}`,
		`{"engine":"claude","profile":"read-mostly","prompt":"Lead.","coordinates":true,"team":"Visionaries","template":"software"}`,
	} {
		if code, body := call(t, server, "PUT", "/api/agents/visionary-lead", change); code != 200 {
			t.Fatalf("change: %d %s", code, body)
		}
		stream, _ := s.Stream(events.AgentsStream)
		if template := agents.FromEvents(stream).TemplateOf("Visionaries"); template != "visionaries" {
			t.Errorf("%s: the team's template is %q, not visionaries", change, template)
		}
	}
	_, body := call(t, server, "GET", "/api/agents", "")
	var roster []agents.Agent
	json.Unmarshal([]byte(body), &roster)
	for _, agent := range roster {
		if agent.Name == "visionary-lead" && agent.Template != "visionaries" {
			t.Errorf("the page should show the lead's template: %+v", agent)
		}
	}
	if !strings.Contains(body, `"template":"visionaries"`) {
		t.Errorf("GET /api/agents should show template: %s", body)
	}
}

// Only adding a team makes agents from a template: one you make yourself has
// none, even if the page sends one.
func TestAnAgentYouMakeHasNoTemplate(t *testing.T) {
	server, s := newTestServer(t)
	agent := `{"name":"thinker","engine":"claude","prompt":"Think.","coordinates":true,"team":"Thinkers","template":"visionaries"}`
	if code, body := call(t, server, "POST", "/api/agents", agent); code != 200 {
		t.Fatalf("create: %d %s", code, body)
	}
	stream, _ := s.Stream(events.AgentsStream)
	if template := agents.FromEvents(stream).TemplateOf("Thinkers"); template != "" {
		t.Errorf("a team you made has template %q", template)
	}
}

// A policy from before a profile was added doesn't have it: a team whose
// agents need it is refused, not added with agents that can do nothing,
// unless you ask for the profile to be added too.
func TestATeamNeedsItsProfilesInThePolicy(t *testing.T) {
	server := servePolicy(t, withoutReadMostly(t))
	agentsBefore, _ := server.log.Stream(events.AgentsStream)
	code, body := call(t, server.Server, "POST", "/api/teams", `{"template":"Visionaries"}`)
	var refusal struct {
		Error           string
		MissingProfiles []string
	}
	json.Unmarshal([]byte(body), &refusal)
	if code != http.StatusConflict || len(refusal.MissingProfiles) != 1 || refusal.MissingProfiles[0] != "read-mostly" ||
		!strings.Contains(refusal.Error, "Policy page") || strings.Contains(body, "default.toml") {
		t.Errorf("a team whose profile the policy lacks: %d %s", code, body)
	}
	if after, _ := server.log.Stream(events.AgentsStream); len(after) != len(agentsBefore) {
		t.Errorf("nothing should be proposed: %d agent events, then %d", len(agentsBefore), len(after))
	}

	for request, want := range map[string]int{`{"template":"Dreamers"}`: http.StatusNotFound, `{"template":`: http.StatusBadRequest,
		`{"template":"Visionaries","addMissingProfiles":true,"hash":"0ld"}`: http.StatusConflict} {
		code, body := call(t, server.Server, "POST", "/api/teams", request)
		var refusal struct{ Error string }
		if err := json.Unmarshal([]byte(body), &refusal); code != want || err != nil || refusal.Error == "" {
			t.Errorf("%s: every error from adding a team is JSON: %d %s", request, code, body)
		}
	}

	if code, body := call(t, server.Server, "POST", "/api/teams", `{"template":"Visionaries","addMissingProfiles":true}`); code != 200 {
		t.Fatalf("adding the Visionaries with their profile: %d %s", code, body)
	}
	if stream := server.policyEvents(t); len(stream) != 1 || stream[0].Text("change") != "added profile read-mostly for team Visionaries" {
		t.Errorf("one logged change of the policy: %d events", len(stream))
	}
	stream, _ := server.log.Stream(events.AgentsStream)
	roster := agents.FromEvents(stream)
	if lead, ok := roster.Coordinator("Visionaries"); !ok || lead.Name != "visionary-lead" || len(roster.Assignees("Visionaries")) != 4 {
		t.Errorf("the Visionaries team: %v", roster.Teams())
	}

	code, body = call(t, server.Server, "POST", "/api/agents", `{"name":"dreamer","engine":"claude","prompt":"Dream.","profile":"dreaming"}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "no profile called dreaming") || strings.Contains(body, server.path) {
		t.Errorf("an agent with a profile nobody wrote: %d %s", code, body)
	}
}

// A custom team gets the same offer as a template team for a profile
// Agentry ships, and a plain refusal for one only you can write.
func TestACustomTeamNeedsItsProfilesInThePolicy(t *testing.T) {
	server := servePolicy(t, withoutReadMostly(t))
	team := func(profile string, more string) string {
		return `{"team":"Data","agents":[{"name":"data-lead","engine":"claude","coordinates":true},` +
			`{"name":"analyst","engine":"claude","profile":"` + profile + `"}]` + more + `}`
	}
	agentsBefore, _ := server.log.Stream(events.AgentsStream)
	code, body := call(t, server.Server, "POST", "/api/teams", team("dreaming", `,"addMissingProfiles":true`))
	if code != http.StatusBadRequest || !strings.Contains(body, "analyst's profile dreaming") || !strings.Contains(body, "doesn't ship") {
		t.Errorf("a profile nobody ships: %d %s", code, body)
	}

	code, body = call(t, server.Server, "POST", "/api/teams", team(" read-mostly ", ""))
	var refusal struct {
		Error           string
		MissingProfiles []string
	}
	json.Unmarshal([]byte(body), &refusal)
	if code != http.StatusConflict || !slices.Equal(refusal.MissingProfiles, []string{"read-mostly"}) || !strings.Contains(refusal.Error, "the Data team needs profile read-mostly") {
		t.Errorf("a shipped profile the policy lacks: %d %s", code, body)
	}
	if after, _ := server.log.Stream(events.AgentsStream); len(after) != len(agentsBefore) || len(server.policyEvents(t)) != 0 {
		t.Errorf("nothing should be proposed: %d agent events, then %d", len(agentsBefore), len(after))
	}

	if code, body := call(t, server.Server, "POST", "/api/teams", team("read-mostly", `,"addMissingProfiles":true`)); code != 200 {
		t.Fatalf("adding the Data team with its profile: %d %s", code, body)
	}
	if stream := server.policyEvents(t); len(stream) != 1 || stream[0].Text("change") != "added profile read-mostly for team Data" {
		t.Errorf("one logged change of the policy: %d events", len(stream))
	}
	stream, _ := server.log.Stream(events.AgentsStream)
	if analyst, ok := agents.FromEvents(stream).Get("analyst"); !ok || analyst.Profile != "read-mostly" || analyst.Team != "Data" {
		t.Errorf("the analyst: %+v", analyst)
	}
}
