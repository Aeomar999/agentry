package web

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// listedTemplate is a template as GET /api/teams/templates lists it.
type listedTemplate struct {
	ID          string
	Team        string
	Description string
	UserMade    bool
	Agents      []struct{ Name string }
}

// userTemplates are the templates you saved, as GET lists them.
func userTemplates(t *testing.T, list string) []listedTemplate {
	t.Helper()
	var templates []listedTemplate
	if err := json.Unmarshal([]byte(list), &templates); err != nil {
		t.Fatalf("templates: %v %s", err, list)
	}
	var yours []listedTemplate
	for _, template := range templates {
		if template.UserMade {
			yours = append(yours, template)
		}
	}
	return yours
}

// madeAgent makes an agent of your own for a test, in team.
func madeAgent(t *testing.T, server *httptest.Server, name, team string, coordinates bool) {
	t.Helper()
	agent, _ := json.Marshal(map[string]any{"name": name, "engine": "claude", "prompt": "Work.", "coordinates": coordinates, "team": team})
	if code, body := call(t, server, "POST", "/api/agents", string(agent)); code != 200 {
		t.Fatalf("make %s: %d %s", name, code, body)
	}
}

func TestASavedTeamIsListedWithEveryAgent(t *testing.T) {
	server, s := newTestServer(t)
	code, body := call(t, server, "POST", "/api/teams/Software/template", `{"id":"my-software","description":"Ours."}`)
	if code != 200 || !strings.Contains(body, `"userMade":true`) {
		t.Fatalf("save: %d %s", code, body)
	}
	_, list := call(t, server, "GET", "/api/teams/templates", "")
	yours := userTemplates(t, list)
	if len(yours) != 1 || yours[0].ID != "my-software" || yours[0].Team != "Software" || yours[0].Description != "Ours." {
		t.Fatalf("your templates: %+v", yours)
	}
	stream, _ := s.Stream(events.AgentsStream)
	team := agents.FromEvents(stream)
	if len(yours[0].Agents) != len(team) {
		t.Errorf("the template has %d agents, the team %d", len(yours[0].Agents), len(team))
	}
	for i, agent := range team {
		if yours[0].Agents[i].Name != agent.Name {
			t.Errorf("agent %d is %s, not %s", i, yours[0].Agents[i].Name, agent.Name)
		}
	}
	if !strings.HasPrefix(list, `[{"id":"software"`) || strings.Contains(list, `"prompt"`) {
		t.Errorf("the built-in templates come first, and nothing lists prompts: %s", list)
	}
}

func TestSavingATemplateAgainUpdatesIt(t *testing.T) {
	server, _ := newTestServer(t)
	call(t, server, "POST", "/api/teams/Software/template", `{"id":"my-software","description":"Ours."}`)
	if code, body := call(t, server, "POST", "/api/teams/software/template", `{"id":"my-software","description":"Still ours."}`); code != 200 {
		t.Fatalf("save again: %d %s", code, body)
	}
	_, list := call(t, server, "GET", "/api/teams/templates", "")
	if yours := userTemplates(t, list); len(yours) != 1 || yours[0].Description != "Still ours." {
		t.Errorf("one template, updated: %+v", yours)
	}
}

func TestATemplateSavedWithoutAnIDIsNamedAfterItsTeam(t *testing.T) {
	server, _ := newTestServer(t)
	madeAgent(t, server, "planner", "Planning", true)
	code, body := call(t, server, "POST", "/api/teams/Planning/template", `{}`)
	if code != 200 || !strings.Contains(body, `"id":"planning"`) {
		t.Errorf("save: %d %s", code, body)
	}
}

func TestATeamNameWithSpacesAndPunctuationGivesAValidID(t *testing.T) {
	server, _ := newTestServer(t)
	madeAgent(t, server, "mine", "My Team!", true)
	code, body := call(t, server, "POST", "/api/teams/My%20Team%21/template", `{"description":""}`)
	if code != 200 || !strings.Contains(body, `"id":"my-team"`) || !strings.Contains(body, `"team":"My Team!"`) {
		t.Errorf("save: %d %s", code, body)
	}
}

// A team named in another case is that team, as everywhere else, and the
// template keeps the team's name as the roster has it.
func TestATeamNamedInAnotherCaseIsSaved(t *testing.T) {
	server, s := newTestServer(t)
	madeAgent(t, server, "planner", "Planning", true)
	if code, body := call(t, server, "POST", "/api/teams/PLANNING/template", `{"id":"my-planning"}`); code != 200 {
		t.Fatalf("save: %d %s", code, body)
	}
	saved, _ := savedTemplates(t, s).WithID("my-planning")
	if saved.Team != "Planning" || len(saved.Agents) != 1 || saved.Agents[0].Name != "planner" {
		t.Errorf("saved %+v", saved)
	}
}

func TestSavingATeamThereIsntIsNotFound(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/teams/Nobody/template", `{"id":"nobody"}`); code != http.StatusNotFound {
		t.Errorf("an unknown team: %d %s", code, body)
	}
}

func TestATemplateWithALongAgentNameIsRefused(t *testing.T) {
	server, _ := newTestServer(t)
	name := "a-coordinator-with-a-long-name" // 30 characters: an agent's name, but too long for a template
	madeAgent(t, server, name, "Long", true)
	code, body := call(t, server, "POST", "/api/teams/Long/template", `{"id":"long"}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "agent "+name+": a template's agent names are at most 28 characters") {
		t.Errorf("a long name: %d %s", code, body)
	}
}

func TestATemplateCantTakeABuiltInTemplatesID(t *testing.T) {
	server, _ := newTestServer(t)
	code, body := call(t, server, "POST", "/api/teams/Software/template", `{"id":"research"}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "the built-in research template has that id") {
		t.Errorf("a built-in id: %d %s", code, body)
	}
}

func TestARefusedIDMadeFromTheTeamsNameSaysSo(t *testing.T) {
	server, _ := newTestServer(t)
	code, body := call(t, server, "POST", "/api/teams/Software/template", `{}`)
	want := `the built-in software template has that id (made from the team's name "Software"): give the template an id`
	if code != http.StatusBadRequest || strings.TrimSpace(body) != want {
		t.Errorf("a built-in id from the team's name: %d %s", code, body)
	}
	madeAgent(t, server, "counter", "2025 Team", true)
	code, body = call(t, server, "POST", "/api/teams/2025%20Team/template", `{}`)
	want = `a template's id is lowercase letters, digits and dashes, starting with a letter, at most 31 characters: 2025-team (made from the team's name "2025 Team"): give the template an id`
	if code != http.StatusBadRequest || strings.TrimSpace(body) != want {
		t.Errorf("an id from a team name starting with a digit: %d %s", code, body)
	}
}

// Giving an id won't fix a refusal of something else, so it gets no hint.
func TestARefusalNotOfTheIDMadeFromTheTeamsNameHasNoHint(t *testing.T) {
	server, _ := newTestServer(t)
	long := "a-coordinator-with-a-long-name"
	madeAgent(t, server, long, "Long Names", true)
	code, body := call(t, server, "POST", "/api/teams/Long%20Names/template", `{}`)
	want := "agent " + long + ": a template's agent names are at most 28 characters, to leave room for a number when the name is taken"
	if code != http.StatusBadRequest || strings.TrimSpace(body) != want {
		t.Errorf("a long agent name in a team saved without an id: %d %s", code, body)
	}
}

func TestARefusedIDYouGaveIsNamedAsItIs(t *testing.T) {
	server, _ := newTestServer(t)
	code, body := call(t, server, "POST", "/api/teams/Software/template", `{"id":"software"}`)
	if code != http.StatusBadRequest || strings.TrimSpace(body) != "the built-in software template has that id" {
		t.Errorf("a built-in id you gave: %d %s", code, body)
	}
}

func TestATemplateIsRemoved(t *testing.T) {
	server, _ := newTestServer(t)
	call(t, server, "POST", "/api/teams/Software/template", `{"id":"my-software"}`)
	if code, body := call(t, server, "DELETE", "/api/teams/templates/my-software", ""); code != 200 {
		t.Fatalf("remove: %d %s", code, body)
	}
	_, list := call(t, server, "GET", "/api/teams/templates", "")
	if yours := userTemplates(t, list); len(yours) != 0 {
		t.Errorf("the template should be gone: %+v", yours)
	}
	if code, body := call(t, server, "DELETE", "/api/teams/templates/my-software", ""); code != http.StatusNotFound {
		t.Errorf("removing it again: %d %s", code, body)
	}
	if code, body := call(t, server, "DELETE", "/api/teams/templates/research", ""); code != http.StatusConflict || !strings.Contains(body, "comes with Agentry") {
		t.Errorf("a built-in template: %d %s", code, body)
	}
	if _, list := call(t, server, "GET", "/api/teams/templates", ""); !strings.Contains(list, `"id":"research"`) {
		t.Errorf("the built-in template stays: %s", list)
	}
}

// A template you saved comes back from the log when Agentry starts again.
func TestASavedTemplateIsListedAfterARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	state := storetest.NewState(events.Core())
	written, err := store.Open(path, events.Core(), state, func(p events.Proposal) *events.Rejection { return rules.Check(p, state, events.Core()) })
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range agents.SeedProposals(nil) {
		written.Propose(p)
	}
	saving, err := agents.SaveTemplate(state.Roster(), agents.DefaultTeam, "my-software", "Ours.", httpapi.You)
	if err != nil {
		t.Fatal(err)
	}
	if _, rejection, err := written.Propose(saving); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	written.Close()

	server, _ := newTestServerOn(t, path, tasks.Limits{})
	_, list := call(t, server, "GET", "/api/teams/templates", "")
	if yours := userTemplates(t, list); len(yours) != 1 || yours[0].ID != "my-software" || len(yours[0].Agents) != len(state.Roster()) {
		t.Errorf("after a restart: %+v", yours)
	}
}

// Exporting a template, removing it and importing the file gives back the
// same template, every agent and prompt in it.
func TestAnExportedTemplateImportsAsItWas(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/teams/Software/template", `{"id":"my-software","description":"Ours."}`)
	want, _ := savedTemplates(t, s).WithID("my-software")

	request, _ := http.NewRequest("GET", server.URL+"/api/teams/templates/my-software/export", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	exported, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || response.Header.Get("Content-Disposition") != `attachment; filename="my-software.json"` {
		t.Fatalf("export: %d %q %s", response.StatusCode, response.Header.Get("Content-Disposition"), exported)
	}

	call(t, server, "DELETE", "/api/teams/templates/my-software", "")
	if code, body := call(t, server, "POST", "/api/teams/templates/import", string(exported)); code != 200 || !strings.Contains(body, `"userMade":true`) {
		t.Fatalf("import: %d %s", code, body)
	}
	got, found := savedTemplates(t, s).WithID("my-software")
	if !found || !reflect.DeepEqual(got, want) {
		t.Errorf("imported:\n%+v\nwant:\n%+v", got, want)
	}
}

func TestImportingATemplateYouHaveReplacesIt(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/teams/Software/template", `{"id":"my-software","description":"Ours."}`)
	_, exported := call(t, server, "GET", "/api/teams/templates/my-software/export", "")
	changed := strings.Replace(exported, `"description":"Ours."`, `"description":"Imported."`, 1)
	if code, body := call(t, server, "POST", "/api/teams/templates/import", changed); code != 200 {
		t.Fatalf("import: %d %s", code, body)
	}
	if saved := savedTemplates(t, s); len(saved) != 1 || saved[0].Description != "Imported." {
		t.Errorf("one template, replaced: %+v", saved)
	}
}

// An imported team and description are trimmed, as a saved description is.
func TestAPaddedImportIsSavedTrimmed(t *testing.T) {
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/teams/Software/template", `{"id":"my-software","description":"Ours."}`)
	_, exported := call(t, server, "GET", "/api/teams/templates/my-software/export", "")
	padded := strings.Replace(exported, `"description":"Ours."`, `"description":"  Ours.  "`, 1)
	padded = strings.Replace(padded, `"team":"Software"`, `"team":"  Software  "`, 1)
	if padded == exported {
		t.Fatal("nothing padded")
	}
	if code, body := call(t, server, "POST", "/api/teams/templates/import", padded); code != 200 {
		t.Fatalf("import: %d %s", code, body)
	}
	if saved, _ := savedTemplates(t, s).WithID("my-software"); saved.Team != "Software" || saved.Description != "Ours." {
		t.Errorf("saved team %q, description %q", saved.Team, saved.Description)
	}
	if _, reexported := call(t, server, "GET", "/api/teams/templates/my-software/export", ""); reexported != exported {
		t.Errorf("exported again:\n%s\nwant:\n%s", reexported, exported)
	}
}

func TestExportingATemplateYouHaventIsRefused(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "GET", "/api/teams/templates/nobody/export", ""); code != http.StatusNotFound {
		t.Errorf("an unknown template: %d %s", code, body)
	}
	code, body := call(t, server, "GET", "/api/teams/templates/software/export", "")
	if code != http.StatusBadRequest || strings.TrimSpace(body) != "the software template comes with Agentry, so it isn't exported: only your own templates are" {
		t.Errorf("a built-in template: %d %s", code, body)
	}
}

// An imported template meets the rules a saved one does.
func TestAnImportTheRulesRefuseIsABadRequest(t *testing.T) {
	server, _ := newTestServer(t)
	agent := func(name string, coordinates bool) map[string]any {
		return map[string]any{"name": name, "engine": "claude", "prompt": "Work.", "coordinates": coordinates}
	}
	withWorkers := agent("lead", true)
	withWorkers["workers"] = []any{"box"}
	for _, refused := range []struct {
		name     string
		template map[string]any
		reason   string
	}{
		{"a built-in id", map[string]any{"id": "software", "team": "Ours", "agents": []any{agent("lead", true)}}, "the built-in software template has that id"},
		{"two coordinators", map[string]any{"id": "ours", "team": "Ours", "agents": []any{agent("lead", true), agent("other-lead", true)}}, "a template has exactly one coordinator"},
		{"no agents", map[string]any{"id": "ours", "team": "Ours"}, "a template needs at least one agent"},
		{"workers", map[string]any{"id": "ours", "team": "Ours", "agents": []any{withWorkers}}, "agent lead: a template's agents name no workers, as the machines may not be there when it is added"},
		{"nothing", map[string]any{}, "a template needs an id"},
	} {
		body, _ := json.Marshal(refused.template)
		if code, reply := call(t, server, "POST", "/api/teams/templates/import", string(body)); code != http.StatusBadRequest || strings.TrimSpace(reply) != refused.reason {
			t.Errorf("%s: %d %s", refused.name, code, reply)
		}
	}
}

func TestAnImportThatIsntATemplateIsABadRequest(t *testing.T) {
	server, _ := newTestServer(t)
	for _, body := range []string{`{"id":"ours","agents":"lead"}`, `{"id":7}`, `[]`, `not json`} {
		if code, reply := call(t, server, "POST", "/api/teams/templates/import", body); code != http.StatusBadRequest || !strings.HasPrefix(reply, "bad request: ") {
			t.Errorf("%s: %d %s", body, code, reply)
		}
	}
}

// planningTeam makes a team of your own, Planning, whose agents differ in
// everything a template keeps, and returns it as the roster has it.
func planningTeam(t *testing.T, server *httptest.Server, s *store.Store) agents.Roster {
	t.Helper()
	team := `{"team":"Planning","agents":[` +
		`{"name":"planner","engine":"claude","model":"opus","profile":"developer","coordinates":true,"prompt":"Plan it.","color":"teal","description":"Plans"},` +
		`{"name":"checker","engine":"codex","profile":"read-only","prompt":"Check it.","color":"violet","column":"review","description":"Checks"}]}`
	if code, body := call(t, server, "POST", "/api/teams", team); code != 200 {
		t.Fatalf("make Planning: %d %s", code, body)
	}
	return teamIn(t, s, "Planning")
}

func rosterOf(t *testing.T, s *store.Store) agents.Roster {
	t.Helper()
	stream, err := s.Stream(events.AgentsStream)
	if err != nil {
		t.Fatal(err)
	}
	return agents.FromEvents(stream)
}

// teamIn is every agent of team in the log, its coordinator too.
func teamIn(t *testing.T, s *store.Store, team string) agents.Roster {
	t.Helper()
	var members agents.Roster
	for _, agent := range rosterOf(t, s) {
		if agent.TeamName() == team {
			members = append(members, agent)
		}
	}
	return members
}

// The story: a team saved as a template, then removed, comes back from the
// template as it was, every agent with its prompt, engine, profile, colour,
// column and coordination, now made from the template.
func TestATeamSavedRemovedAndAddedAgainIsAsItWas(t *testing.T) {
	server, s := newTestServer(t)
	team := planningTeam(t, server, s)
	if code, body := call(t, server, "POST", "/api/teams/Planning/template", `{"id":"my-planning"}`); code != 200 {
		t.Fatalf("save: %d %s", code, body)
	}
	if code, body := call(t, server, "DELETE", "/api/teams/Planning", ""); code != 200 {
		t.Fatalf("remove: %d %s", code, body)
	}
	if code, body := call(t, server, "POST", "/api/teams", `{"template":"my-planning"}`); code != 200 {
		t.Fatalf("add from the template: %d %s", code, body)
	}
	again := teamIn(t, s, "Planning")
	if len(team) != 2 || len(again) != len(team) {
		t.Fatalf("Planning has %d agents again, not %d", len(again), len(team))
	}
	for i, want := range team {
		want.Template = "my-planning"
		if !reflect.DeepEqual(again[i], want) {
			t.Errorf("agent %d:\n%+v\nwant:\n%+v", i, again[i], want)
		}
	}
}

// A template added while its team is still there makes a second team, named
// as a built-in template's second team is.
func TestATemplateAddedBesideItsTeamIsNumbered(t *testing.T) {
	server, s := newTestServer(t)
	planningTeam(t, server, s)
	call(t, server, "POST", "/api/teams/Planning/template", `{"id":"my-planning"}`)
	if code, body := call(t, server, "POST", "/api/teams", `{"template":"my-planning"}`); code != 200 {
		t.Fatalf("add from the template: %d %s", code, body)
	}
	second := teamIn(t, s, "Planning 2")
	if len(second) != 2 || second[0].Name != "planner-2" || !second[0].Coordinates || second[1].Name != "checker-2" || second[1].Prompt != "Check it." {
		t.Errorf("Planning 2: %+v", second)
	}
	if lead, _ := rosterOf(t, s).Coordinator("Planning"); lead.Name != "planner" {
		t.Errorf("Planning keeps its own coordinator, not %q", lead.Name)
	}
}

// importOf is the body of an import of a one-agent template, its
// coordinator's profile and the team named.
func importOf(id, team, profile string) string {
	body, _ := json.Marshal(map[string]any{"id": id, "team": team, "agents": []any{
		map[string]any{"name": "lead", "engine": "claude", "prompt": "Lead.", "coordinates": true, "profile": profile}}})
	return string(body)
}

// A template of yours whose profile the policy lacks gets the offer a
// built-in one gets, and is added once you take it.
func TestYourTemplateNeedsItsProfilesInThePolicy(t *testing.T) {
	server := servePolicy(t, withoutReadMostly(t))
	if code, body := call(t, server.Server, "POST", "/api/teams/templates/import", importOf("careful", "Careful", "read-mostly")); code != 200 {
		t.Fatalf("import: %d %s", code, body)
	}
	agentsBefore, _ := server.log.Stream(events.AgentsStream)
	code, body := call(t, server.Server, "POST", "/api/teams", `{"template":"careful"}`)
	var refusal struct {
		Error           string
		MissingProfiles []string
	}
	json.Unmarshal([]byte(body), &refusal)
	if code != http.StatusConflict || !reflect.DeepEqual(refusal.MissingProfiles, []string{"read-mostly"}) || !strings.Contains(refusal.Error, "the Careful team needs profile read-mostly") {
		t.Errorf("a profile the policy lacks: %d %s", code, body)
	}
	if after, _ := server.log.Stream(events.AgentsStream); len(after) != len(agentsBefore) {
		t.Errorf("nothing should be proposed: %d agent events, then %d", len(agentsBefore), len(after))
	}
	if code, body := call(t, server.Server, "POST", "/api/teams", `{"template":"careful","addMissingProfiles":true}`); code != 200 {
		t.Fatalf("adding Careful with its profile: %d %s", code, body)
	}
	if stream := server.policyEvents(t); len(stream) != 1 || stream[0].Text("change") != "added profile read-mostly for team Careful" {
		t.Errorf("one logged change of the policy: %d events", len(stream))
	}
	if lead, ok := rosterOf(t, server.log).Get("lead"); !ok || lead.Team != "Careful" || lead.Template != "careful" {
		t.Errorf("the lead: %+v", lead)
	}
}

// A template saved under the template rules can still fail the rules of a
// team you define, as the policy or the templates changed since: it is
// refused before anything is proposed.
func TestYourTemplateIsAddedOnlyAsACustomTeamWouldBe(t *testing.T) {
	server := servePolicy(t, string(policy.Default()))
	for _, refused := range []struct {
		name, template, reason string
	}{
		{"a profile nobody ships", importOf("dreamers", "Dreamers", "dreaming"),
			"lead's profile dreaming isn't in policy.toml, and Agentry doesn't ship it: write it in policy.toml, or pick another profile"},
		{"a built-in template's team", importOf("my-writing", "Writing", ""),
			"Writing is the name of the Writing template's team; choose another"},
	} {
		if code, body := call(t, server.Server, "POST", "/api/teams/templates/import", refused.template); code != 200 {
			t.Fatalf("%s: import: %d %s", refused.name, code, body)
		}
		var template agents.UserTemplate
		json.Unmarshal([]byte(refused.template), &template)
		agentsBefore, _ := server.log.Stream(events.AgentsStream)
		code, body := call(t, server.Server, "POST", "/api/teams", `{"template":"`+template.ID+`","addMissingProfiles":true}`)
		var refusal struct{ Error string }
		json.Unmarshal([]byte(body), &refusal)
		if code != http.StatusBadRequest || refusal.Error != refused.reason {
			t.Errorf("%s: %d %s", refused.name, code, body)
		}
		if after, _ := server.log.Stream(events.AgentsStream); len(after) != len(agentsBefore) || len(server.policyEvents(t)) != 0 {
			t.Errorf("%s: nothing should be proposed", refused.name)
		}
	}
}

func TestAddingATemplateThereIsntIsNotFound(t *testing.T) {
	server, _ := newTestServer(t)
	code, body := call(t, server, "POST", "/api/teams", `{"template":"nobody"}`)
	if code != http.StatusNotFound || !strings.Contains(body, "no team template called nobody") {
		t.Errorf("an unknown template: %d %s", code, body)
	}
}

// savedTemplates are the templates the log holds now.
func savedTemplates(t *testing.T, s *store.Store) agents.UserTemplates {
	t.Helper()
	stream, err := s.Stream(events.TemplatesStream)
	if err != nil {
		t.Fatal(err)
	}
	return agents.UserTemplatesFromEvents(stream)
}
