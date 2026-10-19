package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
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
	server := httptest.NewServer(NewServer(s, state, Config{UIToken: token, DeskToken: "desk-secret", HelperToken: "helper-secret",
		Policy: live, Local: workers.Machine{Version: "v0.9.0"}, Worktrees: filepath.Join(data, "worktrees")}, http.NotFoundHandler(), nil).Handler())
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
		if code, body := call(t, server.Server, "POST", "/api/policy/profiles/read-mostly/add", refused); code != http.StatusConflict || !strings.Contains(body, "changed since the page loaded it: try again") {
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

// beforeV010 is the policy of a build before v0.1.0, whose read-only
// profile is outdated now.
func beforeV010(t *testing.T) string {
	t.Helper()
	text, err := os.ReadFile("../policy/shipped/01-v0.1.0-dev.toml")
	if err != nil {
		t.Fatal(err)
	}
	return string(text)
}

func TestAnOutdatedProfileIsUpdatedAndAnEditedOneOnlyByChoice(t *testing.T) {
	server := servePolicy(t, beforeV010(t))
	code, body := call(t, server.Server, "POST", "/api/policy/profiles/read-only/update", `{"hash":"`+server.hash()+`"}`)
	if code != 200 {
		t.Fatalf("update: %d %s", code, body)
	}
	shipped, _ := policy.ShippedProfile("read-only")
	if text, _ := os.ReadFile(server.path); !strings.Contains(string(text), shipped) {
		t.Error("the file should have this version's read-only")
	}
	stream := server.policyEvents(t)
	if len(stream) != 1 || stream[0].Text("change") != "updated read-only to the v0.9.0 default" || stream[0].Actor != you ||
		!strings.Contains(stream[0].Text("diff"), `+  name = "keeps its own notes"`) {
		t.Errorf("the update is logged as yours with its change: %d events", len(stream))
	}

	edited := strings.Replace(string(policy.Default()), `why = "For agents that judge work`, `why = "Mine: for agents that judge work`, 1)
	os.WriteFile(server.path, []byte(edited), 0o600)
	server.live.Reload()
	if code, body := call(t, server.Server, "POST", "/api/policy/profiles/read-only/update", `{"hash":"`+server.hash()+`"}`); code != http.StatusConflict ||
		!strings.Contains(body, "won't overwrite it") {
		t.Errorf("updating a profile you changed: %d %s", code, body)
	}
	if text, _ := os.ReadFile(server.path); string(text) != edited {
		t.Error("a refused update leaves your profile as it is")
	}
	if code, body := call(t, server.Server, "POST", "/api/policy/profiles/read-only/use-default", `{"hash":"`+server.hash()+`"}`); code != 200 {
		t.Fatalf("use the default: %d %s", code, body)
	}
	if text, _ := os.ReadFile(server.path); string(text) != string(policy.Default()) {
		t.Error("taking the default should leave the file as Agentry ships it")
	}
	stream = server.policyEvents(t)
	if last := stream[len(stream)-1]; len(stream) != 2 || last.Text("change") != "replaced your read-only with the v0.9.0 default" ||
		!strings.Contains(last.Text("diff"), `-why = "Mine: for agents that judge work`) {
		t.Errorf("taking the default is logged with its change: %d events, %q", len(stream), last.Text("change"))
	}
}

// A change Agentry refuses is a 409; failing to write the file is a 500.
func TestAChangeThatCantBeWrittenIsAFailureNotARefusal(t *testing.T) {
	server := servePolicy(t, string(policy.Default()))
	if code, body := call(t, server.Server, "POST", "/api/policy/profiles/read-only/add", `{"hash":"`+server.hash()+`"}`); code != http.StatusConflict ||
		!strings.Contains(body, "already has") {
		t.Errorf("a refusal: %d %s", code, body)
	}
	dir := filepath.Dir(server.path)
	os.Chmod(dir, 0o500)
	t.Cleanup(func() { os.Chmod(dir, 0o700) })
	if code, body := call(t, server.Server, "POST", "/api/policy/use-for-everyone/off", `{"hash":"`+server.hash()+`"}`); code != http.StatusInternalServerError {
		t.Errorf("a folder Agentry can't write to: %d %s", code, body)
	}
	if len(server.policyEvents(t)) != 0 {
		t.Error("nothing should be logged")
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

func TestAProfileDroppedOnDiskShowsWithTheAgentsThatNeedIt(t *testing.T) {
	server := servePolicy(t, string(policy.Default()))
	if code, body := call(t, server.Server, "POST", "/api/teams", `{"template":"Visionaries"}`); code != 200 {
		t.Fatalf("adding the Visionaries: %d %s", code, body)
	}
	os.WriteFile(server.path, []byte(withoutReadMostly(t)), 0o600) // a hand edit, which nothing could refuse
	if _, err := server.live.Reload(); err != nil {
		t.Fatal(err)
	}
	visionaries := []string{"product-strategist", "skeptic", "technologist", "user-advocate", "visionary-lead"}

	_, body := call(t, server.Server, "GET", "/api/policy", "")
	var shown struct {
		Missing   map[string][]string
		Effective map[string]effectiveView
	}
	json.Unmarshal([]byte(body), &shown)
	if got := shown.Missing["read-mostly"]; len(shown.Missing) != 1 || !slices.Equal(got, visionaries) {
		t.Errorf("the Policy page should name the agents needing read-mostly: %v", shown.Missing)
	}
	if e := shown.Effective["visionary-lead"]; !e.Missing || e.Text != "read-mostly (missing from policy.toml), overridden by allow-everything" {
		t.Errorf("the lead's card: %+v", e)
	}

	_, body = call(t, server.Server, "GET", "/api/config", "")
	var config struct{ MissingProfiles map[string][]string }
	json.Unmarshal([]byte(body), &config)
	if got := config.MissingProfiles["read-mostly"]; len(config.MissingProfiles) != 1 || !slices.Equal(got, visionaries) {
		t.Errorf("the status bar should hear of it too: %s", body)
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
	if history[0].OnDisk || history[1].OnDisk {
		t.Errorf("a change on the page, or the first version, isn't a hand edit: %+v", history)
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
	if code, body := call(t, server.Server, "POST", "/api/policy/revert", `{"position":`+itoa(history[2].Position)+`,"hash":"`+server.hash()+`"}`); code != http.StatusConflict ||
		!strings.Contains(body, "can't be restored") {
		t.Errorf("a version without its text: %d %s", code, body)
	}
	if code, _ := call(t, server.Server, "POST", "/api/policy/revert", `{"position":999,"hash":"`+server.hash()+`"}`); code != http.StatusNotFound {
		t.Errorf("a position with no version: %d", code)
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

// Before a revert the page shows what it would change: the diff from the
// version in force, and whether the version puts every agent under one profile.
func TestAVersionShowsWhatRevertingToItWouldChange(t *testing.T) {
	server := servePolicy(t, string(policy.Default()))
	first := server.live.Current()
	server.log.Propose(events.Proposal{Stream: events.PolicyStream, Actor: policy.Agentry, Type: events.PolicyLoaded,
		Data: map[string]any{"hash": "0ld", "path": server.path}})
	server.log.Propose(policy.Loaded(server.path, &policy.Change{Policy: first}, "first loaded", policy.Agentry))
	server.log.Propose(events.Proposal{Stream: events.PolicyStream, Actor: policy.Agentry, Type: events.PolicyLoaded,
		Data: map[string]any{"hash": "b4d", "path": server.path, "text": "version = "}})
	call(t, server.Server, "POST", "/api/policy/use-for-everyone/off", `{"hash":"`+server.hash()+`"}`)
	stream := server.policyEvents(t)
	noText, firstLoaded, badText, inForce := stream[0].Position, stream[1].Position, stream[2].Position, stream[3].Position

	code, body := call(t, server.Server, "GET", "/api/policy/history/"+itoa(firstLoaded), "")
	type versionChange struct {
		Position              int64
		Diff                  string
		UseForEveryone        string
		InForceUseForEveryone string
	}
	var version versionChange
	if err := json.Unmarshal([]byte(body), &version); code != 200 || err != nil {
		t.Fatalf("the first version: %d %s", code, body)
	}
	if version.Position != firstLoaded || version.UseForEveryone != "allow-everything" || version.InForceUseForEveryone != "" {
		t.Errorf("the first version puts everyone under allow-everything, the one in force doesn't: %+v", version)
	}
	newName := "version of " + stream[1].At.Local().Format("2 Jan 2006 15:04:05")
	if want := policy.LineDiff("in force", newName, server.live.Current().Text(), first.Text()); version.Diff != want {
		t.Errorf("the diff runs from the version in force to the first:\n%s\nwant\n%s", version.Diff, want)
	}
	if !strings.Contains(version.Diff, "\n-# use_for_everyone = \"allow-everything\"\n+use_for_everyone = \"allow-everything\"\n") {
		t.Errorf("reverting turns use_for_everyone back on:\n%s", version.Diff)
	}

	code, body = call(t, server.Server, "GET", "/api/policy/history/"+itoa(inForce), "")
	var inForceVersion versionChange
	if err := json.Unmarshal([]byte(body), &inForceVersion); code != 200 || err != nil || inForceVersion.Diff != "" || inForceVersion.UseForEveryone != "" {
		t.Errorf("the version in force changes nothing: %d %s", code, body)
	}
	for _, c := range []struct {
		position string
		code     int
		want     string
	}{
		{itoa(noText), http.StatusConflict, "can't be restored"},
		{itoa(badText), http.StatusConflict, "doesn't load"},
		{"999", http.StatusNotFound, "no such version"},
		{"first", http.StatusBadRequest, "number"},
	} {
		if code, body := call(t, server.Server, "GET", "/api/policy/history/"+c.position, ""); code != c.code || !strings.Contains(body, c.want) {
			t.Errorf("version %s: %d %s, want %d %q", c.position, code, body, c.code, c.want)
		}
	}
}

func TestThereIsNoVersionToShowWithoutAPolicy(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "GET", "/api/policy/history/1", ""); code != http.StatusNotFound || !strings.Contains(body, "no policy") {
		t.Errorf("no policy: %d %s", code, body)
	}
}

// Only the page changes the policy: no agent's token reaches these.
func TestOnlyTheUITokenChangesThePolicy(t *testing.T) {
	server := servePolicy(t, withoutReadMostly(t))
	body := `{"hash":"` + server.hash() + `"}`
	for _, path := range []string{"/api/policy/profiles/read-mostly/add", "/api/policy/profiles/read-only/update", "/api/policy/profiles/read-only/use-default",
		"/api/policy/use-for-everyone/off", "/api/policy/revert", "/api/policy/history", "/api/policy/history/1", "/api/policy/try"} {
		for _, bearer := range []string{"", "desk-secret", "helper-secret", "hook-secret"} {
			method := "POST"
			if strings.Contains(path, "/history") {
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

// tried is a reply to POST /api/policy/try.
type tried struct {
	Verdict   verdictView
	Workspace string
	Override  *struct {
		Profile string
		Verdict verdictView
	}
}

func try(t *testing.T, server policyServer, request string) tried {
	t.Helper()
	code, body := call(t, server.Server, "POST", "/api/policy/try", request)
	if code != http.StatusOK {
		t.Fatalf("%s: %d %s", request, code, body)
	}
	var got tried
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func TestACallIsTriedAsAProfileWouldJudgeIt(t *testing.T) {
	server := servePolicy(t, string(policy.Default()))
	cases := []struct {
		request, decision, rule string
	}{
		{`{"profile":"developer","tool":"Bash","input":"go test ./..."}`, "allow", "stays inside the workspace"},
		{`{"profile":"developer","tool":"Bash","input":"git push origin feature"}`, "ask", "every push leaves this machine: you approve it"},
		{`{"profile":"developer","tool":"Bash","input":"sudo ls"}`, "deny", "never sudo"},
		{`{"profile":"read-only","tool":"Edit","input":"main.go"}`, "deny", "this agent changes nothing"},
		{`{"profile":"developer","tool":"Read","input":"~/.ssh/id_ed25519"}`, "deny", "never touch secrets"},
	}
	for _, c := range cases {
		got := try(t, server, c.request)
		if string(got.Verdict.Decision) != c.decision || got.Verdict.Rule != c.rule || got.Verdict.Why == "" || got.Verdict.Policy != server.hash() {
			t.Errorf("%s: got %+v, want %s by %q", c.request, got.Verdict, c.decision, c.rule)
		}
		// The shipped policy sets use_for_everyone, so today allow-everything decides.
		if got.Override == nil || got.Override.Profile != "allow-everything" || got.Override.Verdict.Decision != policy.Allow {
			t.Errorf("%s: the override should say allow-everything allows it, got %+v", c.request, got.Override)
		}
	}
}

func TestATriedCallRunsInAStandInWorktreeThatIsntAgentrys(t *testing.T) {
	server := servePolicy(t, string(policy.Default()))
	got := try(t, server, `{"profile":"developer","tool":"Read","input":"main.go"}`)
	if filepath.Base(got.Workspace) != "example" || filepath.Base(filepath.Dir(got.Workspace)) != "worktrees" {
		t.Errorf("the stand-in should be a worktree beside the tasks', got %q", got.Workspace)
	}
	if got.Verdict.Decision != policy.Allow || got.Verdict.Rule != "reading is always allowed" {
		t.Errorf("reading a file in the stand-in: got %+v", got.Verdict)
	}
	inside := try(t, server, `{"profile":"developer","tool":"Edit","input":"main.go"}`)
	if inside.Verdict.Rule != "edit inside the workspace, the diff is reviewed" {
		t.Errorf("a relative path is inside the stand-in: got %+v", inside.Verdict)
	}
	own := try(t, server, fmt.Sprintf(`{"profile":"developer","tool":"Read","input":%q}`, server.path))
	if own.Verdict.Decision != policy.Deny || own.Verdict.Rule != "Agentry's own files are off limits" {
		t.Errorf("the policy itself stays off limits: got %+v", own.Verdict)
	}

	named := try(t, server, `{"profile":"developer","tool":"Edit","input":"/srv/app/main.go","workspace":"/srv/app/"}`)
	if named.Workspace != "/srv/app" || named.Verdict.Rule != "edit inside the workspace, the diff is reviewed" {
		t.Errorf("a named workspace is used, cleaned: got %q %+v", named.Workspace, named.Verdict)
	}
}

func TestATriedCallSaysWhoDecidesOnlyWhenAnotherProfileDoes(t *testing.T) {
	server := servePolicy(t, string(policy.Default()))
	if got := try(t, server, `{"profile":"allow-everything","tool":"Bash","input":"ls"}`); got.Override != nil {
		t.Errorf("allow-everything is the one use_for_everyone names: got %+v", got.Override)
	}
	off := servePolicy(t, strings.Replace(string(policy.Default()), "\nuse_for_everyone =", "\n# use_for_everyone =", 1))
	if got := try(t, off, `{"profile":"developer","tool":"Bash","input":"git push origin feature"}`); got.Override != nil || got.Verdict.Decision != policy.Ask {
		t.Errorf("with use_for_everyone off, developer decides alone: got %+v", got)
	}
}

func TestTryingACallChangesNothing(t *testing.T) {
	server := servePolicy(t, string(policy.Default()))
	before, hash := server.log.LastPosition(), server.hash()
	try(t, server, `{"profile":"developer","tool":"Bash","input":"git push origin feature"}`)
	if after := server.log.LastPosition(); after != before || server.hash() != hash {
		t.Errorf("trying a call logged events (%d to %d) or changed the policy", before, after)
	}
}

func TestATriedCallIsRefusedWhenItCantBeJudged(t *testing.T) {
	server := servePolicy(t, string(policy.Default()))
	cases := []struct {
		request string
		code    int
	}{
		{`{"profile":"haunted","tool":"Bash","input":"ls"}`, http.StatusNotFound},
		{`{"profile":"developer","tool":"","input":"ls"}`, http.StatusBadRequest},
		{`{"profile":"developer","tool":"Bash","input":"ls","workspace":"work/app"}`, http.StatusBadRequest},
		{`{"profile":`, http.StatusBadRequest},
	}
	for _, c := range cases {
		if code, body := call(t, server.Server, "POST", "/api/policy/try", c.request); code != c.code {
			t.Errorf("%s: %d %s, want %d", c.request, code, body, c.code)
		}
	}
	if code, _ := call(t, server.Server, "POST", "/api/policy/try", `{"profile":"developer","tool":"TodoWrite"}`); code != http.StatusOK {
		t.Errorf("a tool with no input is judged: %d", code)
	}
}
