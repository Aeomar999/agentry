package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// A server with a policy, for web's own routes that read or change it: a
// team's profiles, and the position a read was made at. The policy's own
// routes are tested in policyapi.

// policyServer, servePolicy, withoutReadMostly, hash and policyEvents are
// the same as their copies in
// internal/domain/policy/policyapi/policyapi_test.go: change both together.

// policyServer serves the default team and a policy file holding text.
type policyServer struct {
	*httptest.Server
	log  *store.Store
	live *policy.Live
	path string
}

// servePolicy serves the policy text; configure, if given, changes the
// server's Config first.
func servePolicy(t *testing.T, text string, configure ...func(*Config)) policyServer {
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
	config := Config{UIToken: token, DeskToken: "desk-secret", HelperToken: "helper-secret",
		Policy: live, Local: workers.Machine{Version: "v0.9.0"}, Worktrees: filepath.Join(data, "worktrees")}
	for _, c := range configure {
		c(&config)
	}
	server := httptest.NewServer(NewServer(s, state, config, http.NotFoundHandler(), nil).Handler())
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

func (p policyServer) hash() string { return p.live.Current().Hash() }

func (p policyServer) policyEvents(t *testing.T) []events.Event {
	t.Helper()
	stream, err := p.log.Stream(events.PolicyStream)
	if err != nil {
		t.Fatal(err)
	}
	return stream
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

// A version's read carries the log position like every other read, so the
// page's shared cache reads it again only when the log moves past it.
func TestAVersionReadSaysThePositionItWasReadAt(t *testing.T) {
	server := servePolicy(t, string(policy.Default()))
	server.log.Propose(policy.Loaded(server.path, &policy.Change{Policy: server.live.Current()}, "first loaded", policy.Agentry))
	call(t, server.Server, "POST", "/api/policy/use-for-everyone/off", `{"hash":"`+server.hash()+`"}`)
	path := "/api/policy/history/" + itoa(server.policyEvents(t)[0].Position)
	if code, body := call(t, server.Server, "GET", path, ""); code != 200 {
		t.Fatalf("GET %s: %d %s", path, code, body)
	}
	if got, at := positionOf(t, server.Server, "GET", path), itoa(server.log.LastPosition()); got != at {
		t.Errorf("GET %s: position %q, want %s", path, got, at)
	}
}
