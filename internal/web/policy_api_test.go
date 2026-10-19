package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// policyServer serves the default team and a policy file holding text.
type policyServer struct {
	*httptest.Server
	log  *store.Store
	live *policy.Live
	path string
}

func servePolicy(t *testing.T, text string) policyServer {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	path := filepath.Join(t.TempDir(), "policy.toml")
	os.WriteFile(path, []byte(text), 0o600)
	live, err := policy.OpenLive(path, "/home/dev", nil)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, DeskToken: "desk-secret", HelperToken: "helper-secret",
		Policy: live, Local: workers.Machine{Version: "v0.9.0"}}, http.NotFoundHandler(), nil).Handler())
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

func TestThePolicyShowsItsDefaultsUsersAndWhatIsInForce(t *testing.T) {
	server := servePolicy(t, withoutReadMostly(t))
	_, body := call(t, server.Server, "GET", "/api/policy", "")
	var shown struct {
		Version  string
		Defaults []struct {
			Name, State, Text, Diff string
		}
		Users     map[string][]profileUser
		Effective map[string]effectiveView
	}
	if err := json.Unmarshal([]byte(body), &shown); err != nil {
		t.Fatal(err)
	}
	if shown.Version != "v0.9.0" {
		t.Errorf("version %q", shown.Version)
	}
	states := map[string]string{}
	for _, d := range shown.Defaults {
		states[d.Name] = d.State
		if d.Name == "read-mostly" && !strings.HasPrefix(d.Text, "[profiles.read-mostly]\n") {
			t.Errorf("a missing profile comes with the text it would add: %q", d.Text)
		}
	}
	if states["read-mostly"] != "missing" || states["read-only"] != "current" || len(states) != 5 {
		t.Errorf("defaults: %v", states)
	}
	if got := shown.Users["read-only"]; len(got) != 2 || got[0] != (profileUser{Agent: "reviewer"}) || got[1].Agent != "tech-lead" {
		t.Errorf("read-only's users: %+v", got)
	}
	if e := shown.Effective["reviewer"]; !e.Overridden || e.Own != "read-only" || e.Text != "read-only, overridden by allow-everything" {
		t.Errorf("the reviewer under use_for_everyone: %+v", e)
	}

	_, body = call(t, server.Server, "GET", "/api/agents", "")
	var list []agentView
	json.Unmarshal([]byte(body), &list)
	for _, a := range list {
		if a.Name == "engineer" && (a.Effective == nil || a.Effective.Text != "developer, overridden by allow-everything") {
			t.Errorf("the agents list shows each agent's effective profile: %+v", a.Effective)
		}
	}
}

func TestAMissingProfileIsAddedFromThePageAndLogged(t *testing.T) {
	server := servePolicy(t, withoutReadMostly(t))
	before := server.hash()
	for _, refused := range []string{`{}`, `{"hash":"0ld"}`} {
		if code, body := call(t, server.Server, "POST", "/api/policy/profiles/read-mostly/add", refused); code != http.StatusConflict || !strings.Contains(body, "reload") {
			t.Errorf("%s: %d %s", refused, code, body)
		}
	}
	code, body := call(t, server.Server, "POST", "/api/policy/profiles/read-mostly/add", `{"hash":"`+before+`"}`)
	if code != 200 {
		t.Fatalf("add: %d %s", code, body)
	}
	text, _ := os.ReadFile(server.path)
	if !strings.Contains(string(text), "# Added by Agentry v0.9.0, the default profile of that version.\n[profiles.read-mostly]\n") {
		t.Error("the file should have read-mostly, under a comment naming the version")
	}
	stream := server.policyEvents(t)
	logged := stream[len(stream)-1]
	if len(stream) != 1 || logged.Actor != you || logged.Text("change") != "added profile read-mostly" || logged.Text("text") != string(text) ||
		!strings.Contains(logged.Text("diff"), "+[profiles.read-mostly]\n") {
		t.Errorf("the change is logged as yours, with its diff: %d events, %s %q", len(stream), logged.Actor, logged.Text("change"))
	}
	if saved, err := os.ReadFile(logged.Text("backup")); err != nil || string(saved) != withoutReadMostly(t) {
		t.Errorf("the backup %q should hold the file as it was: %v", logged.Text("backup"), err)
	}

	if code, body := call(t, server.Server, "POST", "/api/policy/profiles/read-mostly/add", `{"hash":"`+before+`"}`); code != http.StatusConflict {
		t.Errorf("the same change against the old version: %d %s", code, body)
	}
	if code, body := call(t, server.Server, "POST", "/api/policy/profiles/read-mostly/add", `{"hash":"`+server.hash()+`"}`); code != http.StatusConflict || !strings.Contains(body, "already has") {
		t.Errorf("adding it twice: %d %s", code, body)
	}
	if code, _ := call(t, server.Server, "POST", "/api/policy/profiles/dreamer/add", `{"hash":"`+server.hash()+`"}`); code != http.StatusNotFound {
		t.Errorf("a profile Agentry doesn't ship: %d", code)
	}
	if len(server.policyEvents(t)) != 1 {
		t.Error("refused changes are not logged")
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

func TestTurningOffUseForEveryoneGivesEachAgentItsOwnProfile(t *testing.T) {
	server := servePolicy(t, string(policy.Default()))
	if code, body := call(t, server.Server, "POST", "/api/policy/use-for-everyone/off", `{"hash":"`+server.hash()+`"}`); code != 200 {
		t.Fatalf("turn off: %d %s", code, body)
	}
	if server.live.Current().UseForEveryone() != "" {
		t.Error("use_for_everyone should be off")
	}
	_, body := call(t, server.Server, "GET", "/api/agents", "")
	var list []agentView
	json.Unmarshal([]byte(body), &list)
	for _, a := range list {
		if a.Name == "reviewer" && (a.Effective.Overridden || a.Effective.Text != "read-only") {
			t.Errorf("the reviewer: %+v", a.Effective)
		}
	}
	if code, body := call(t, server.Server, "POST", "/api/policy/use-for-everyone/off", `{"hash":"`+server.hash()+`"}`); code != http.StatusConflict || !strings.Contains(body, "isn't set") {
		t.Errorf("turning it off twice: %d %s", code, body)
	}
}

func TestEveryVersionShowsInTheHistoryAndCanBeRestored(t *testing.T) {
	server := servePolicy(t, string(policy.Default()))
	first := server.live.Current()
	// One from before versions carried their text, then the one at start.
	server.log.Propose(events.Proposal{Stream: events.PolicyStream, Actor: policy.Agentry, Type: events.PolicyLoaded,
		Data: map[string]any{"hash": "0ld", "path": server.path}})
	server.log.Propose(policy.Loaded(server.path, &policy.Change{Policy: first}, "first loaded", policy.Agentry))
	call(t, server.Server, "POST", "/api/policy/use-for-everyone/off", `{"hash":"`+server.hash()+`"}`)

	_, body := call(t, server.Server, "GET", "/api/policy/history", "")
	var history []policy.Version
	json.Unmarshal([]byte(body), &history)
	if len(history) != 3 || history[0].Change != "turned off use_for_everyone" || history[1].Change != "first loaded" || history[2].Change != "loaded" {
		t.Fatalf("history: %+v", history)
	}
	if !history[0].CanRevert || !history[1].CanRevert || history[2].CanRevert {
		t.Errorf("only versions with their text can be restored: %+v", history)
	}

	code, body := call(t, server.Server, "POST", "/api/policy/revert", `{"position":`+itoa(history[1].Position)+`,"hash":"`+server.hash()+`"}`)
	if code != 200 {
		t.Fatalf("revert: %d %s", code, body)
	}
	if text, _ := os.ReadFile(server.path); string(text) != first.Text() || server.hash() != first.Hash() {
		t.Error("the file should be the first version again")
	}
	stream := server.policyEvents(t)
	if last := stream[len(stream)-1]; !strings.HasPrefix(last.Text("change"), "reverted to the version of ") || last.Actor != you {
		t.Errorf("the revert is a new version, yours: %q %s", last.Text("change"), last.Actor)
	}
	if code, _ := call(t, server.Server, "POST", "/api/policy/revert", `{"position":`+itoa(history[2].Position)+`,"hash":"`+server.hash()+`"}`); code != http.StatusNotFound {
		t.Errorf("a version without its text: %d", code)
	}
	if code, body := call(t, server.Server, "POST", "/api/policy/revert", `{"position":`+itoa(history[1].Position)+`,"hash":"`+server.hash()+`"}`); code != http.StatusConflict || !strings.Contains(body, "in force") {
		t.Errorf("the version in force: %d %s", code, body)
	}
}

func TestARevertMayNotTakeAProfileFromItsAgents(t *testing.T) {
	server := servePolicy(t, withoutReadMostly(t))
	server.log.Propose(policy.Loaded(server.path, &policy.Change{Policy: server.live.Current()}, "first loaded", policy.Agentry))
	before := server.policyEvents(t)[0].Position
	call(t, server.Server, "POST", "/api/teams", `{"template":"Visionaries","addMissingProfiles":true}`)
	text, _ := os.ReadFile(server.path)

	code, body := call(t, server.Server, "POST", "/api/policy/revert", `{"position":`+itoa(before)+`,"hash":"`+server.hash()+`"}`)
	if code != http.StatusConflict || !strings.Contains(body, `profile "read-mostly" is still used by`) || !strings.Contains(body, "visionary-lead") {
		t.Errorf("a revert to before read-mostly, with the Visionaries using it: %d %s", code, body)
	}
	if now, _ := os.ReadFile(server.path); string(now) != string(text) {
		t.Error("a refused revert leaves the file as it was")
	}
}

// Only the page changes the policy: no agent's token reaches these.
func TestOnlyTheUITokenChangesThePolicy(t *testing.T) {
	server := servePolicy(t, withoutReadMostly(t))
	body := `{"hash":"` + server.hash() + `"}`
	for _, path := range []string{"/api/policy/profiles/read-mostly/add", "/api/policy/profiles/read-only/update", "/api/policy/profiles/read-only/use-default",
		"/api/policy/use-for-everyone/off", "/api/policy/revert", "/api/policy/history"} {
		for _, bearer := range []string{"", "desk-secret", "helper-secret", "hook-secret"} {
			method := "POST"
			if strings.HasSuffix(path, "/history") {
				method = "GET"
			}
			request, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
			if bearer != "" {
				request.Header.Set("Authorization", "Bearer "+bearer)
			}
			response, err := http.DefaultClient.Do(request)
			if err != nil {
				t.Fatal(err)
			}
			response.Body.Close()
			if response.StatusCode != http.StatusUnauthorized {
				t.Errorf("%s %s with %q: %d, want 401", method, path, bearer, response.StatusCode)
			}
		}
	}
	if text, _ := os.ReadFile(server.path); string(text) != withoutReadMostly(t) || len(server.policyEvents(t)) != 0 {
		t.Error("nothing should have changed")
	}
}
