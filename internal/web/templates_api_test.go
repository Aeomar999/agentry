package web

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

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
	SavedAt     string
	From        string
	Agents      []struct{ Name, Prompt string }
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
	team := planningTeam(t, server, s)
	code, body := call(t, server, "POST", "/api/teams/Planning/template", `{"id":"my-planning","description":"Ours."}`)
	if code != 200 || !strings.Contains(body, `"userMade":true`) {
		t.Fatalf("save: %d %s", code, body)
	}
	_, list := call(t, server, "GET", "/api/teams/templates", "")
	yours := userTemplates(t, list)
	if len(yours) != 1 || yours[0].ID != "my-planning" || yours[0].Team != "Planning" || yours[0].Description != "Ours." {
		t.Fatalf("your templates: %+v", yours)
	}
	if len(yours[0].Agents) != len(team) {
		t.Errorf("the template has %d agents, the team %d", len(yours[0].Agents), len(team))
	}
	for i, agent := range team {
		if yours[0].Agents[i].Name != agent.Name {
			t.Errorf("agent %d is %s, not %s", i, yours[0].Agents[i].Name, agent.Name)
		}
		if yours[0].Agents[i].Prompt != agent.Prompt {
			t.Errorf("%s lists the prompt %q, not its own %q", agent.Name, yours[0].Agents[i].Prompt, agent.Prompt)
		}
	}
	if !strings.HasPrefix(list, `[{"id":"software"`) {
		t.Errorf("the built-in templates come first: %s", list)
	}
}

// A template is named apart from its team: by default the team's name,
// trimmed when given. The list says when you saved it and from which team.
func TestATemplateIsSavedUnderItsName(t *testing.T) {
	server, _ := newTestServer(t)
	madeAgent(t, server, "planner", "Planning", true)
	before := time.Now().UTC().Truncate(time.Second)
	for _, saving := range []struct{ name, body, id, team string }{
		{"no name", `{}`, "planning", "Planning"},
		{"a padded name", `{"name":"  Reading Room  "}`, "reading-room", "Reading Room"},
	} {
		code, body := call(t, server, "POST", "/api/teams/Planning/template", saving.body)
		var reply listedTemplate
		json.Unmarshal([]byte(body), &reply)
		if code != 200 || reply.ID != saving.id || reply.Team != saving.team || reply.From != "Planning" || !reply.UserMade {
			t.Errorf("%s: %d %s", saving.name, code, body)
		}
	}
	// A blank name is the team's, which a template has by now.
	if code, body := call(t, server, "POST", "/api/teams/Planning/template", `{"name":"   ","id":"blank"}`); code != http.StatusConflict {
		t.Errorf("a blank name: %d %s", code, body)
	}

	_, list := call(t, server, "GET", "/api/teams/templates", "")
	var all []listedTemplate
	json.Unmarshal([]byte(list), &all)
	for _, template := range all {
		if !template.UserMade && (template.SavedAt != "" || template.From != "") {
			t.Errorf("the built-in %s has a saved date or team: %+v", template.ID, template)
		}
	}
	yours := userTemplates(t, list)
	if len(yours) != 2 || yours[0].Team != "Planning" || yours[1].Team != "Reading Room" {
		t.Fatalf("your templates: %+v", yours)
	}
	for _, template := range yours {
		savedAt, err := time.Parse(time.RFC3339, template.SavedAt)
		if err != nil || savedAt.Before(before) || savedAt.After(time.Now()) || template.From != "Planning" {
			t.Errorf("%s: saved at %q (%v), from %q", template.ID, template.SavedAt, err, template.From)
		}
	}
}

// Saving never replaces a template: a name one of yours has, in any case,
// a built-in template's team, or an id one of yours has is refused, in the
// words the page shows under the name, and nothing is written.
func TestATemplateNameOrIDTakenIsRefused(t *testing.T) {
	server, s := newTestServer(t)
	madeAgent(t, server, "planner", "Planning", true)
	if code, body := call(t, server, "POST", "/api/teams/Planning/template", `{"name":"My Planning","description":"Ours."}`); code != 200 {
		t.Fatalf("save: %d %s", code, body)
	}
	before, _ := s.Stream(events.TemplatesStream)
	for _, refused := range []struct{ name, body, want string }{
		{"yours, in another case", `{"name":"my PLANNING","id":"other"}`, "You have a template called My Planning already. Pick another name."},
		{"yours, padded", `{"name":"  My Planning "}`, "You have a template called My Planning already. Pick another name."},
		{"a built-in one", `{"name":"software"}`, "Software is a built-in template. Pick another name."},
		{"a built-in one, with an id", `{"name":"RESEARCH","id":"my-research"}`, "Research is a built-in template. Pick another name."},
		{"the id given", `{"name":"Others","id":"my-planning"}`, "You have a template called My Planning already. Pick another name."},
	} {
		if code, body := call(t, server, "POST", "/api/teams/Planning/template", refused.body); code != http.StatusConflict || strings.TrimSpace(body) != refused.want {
			t.Errorf("%s: %d %s", refused.name, code, body)
		}
	}
	if after, _ := s.Stream(events.TemplatesStream); len(after) != len(before) {
		t.Errorf("nothing should be written: %d template events, then %d", len(before), len(after))
	}
	if saved, _ := savedTemplates(t, s).WithID("my-planning"); saved.Description != "Ours." {
		t.Errorf("the template stays as it was: %+v", saved)
	}
}

// Saves sent at once can all pass the check before saving; the rules then
// refuse all but the first, which the page hears as the same 409.
func TestSavesOfOneNameAtOnceKeepTheFirst(t *testing.T) {
	server, s := newTestServer(t)
	madeAgent(t, server, "planner", "Planning", true)
	const saves = 20
	codes, bodies := make([]int, saves), make([]string, saves)
	var wg sync.WaitGroup
	for i := range saves {
		wg.Go(func() {
			codes[i], bodies[i] = call(t, server, "POST", "/api/teams/Planning/template", `{"name":"My Planning"}`)
		})
	}
	wg.Wait()
	saved := 0
	for i, code := range codes {
		switch {
		case code == 200:
			saved++
		case code != http.StatusConflict || strings.TrimSpace(bodies[i]) != "You have a template called My Planning already. Pick another name.":
			t.Errorf("save %d: %d %s", i, code, bodies[i])
		}
	}
	if stream, _ := s.Stream(events.TemplatesStream); saved != 1 || len(stream) != 1 {
		t.Errorf("%d saves answered as saved and %d written, want one", saved, len(stream))
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

// A team named with spaces around it is that team, as TeamNamed finds it.
func TestATeamNamedWithSpacesIsSaved(t *testing.T) {
	server, s := newTestServer(t)
	madeAgent(t, server, "planner", "Planning", true)
	if code, body := call(t, server, "POST", "/api/teams/%20Planning/template", `{"id":"my-planning"}`); code != 200 {
		t.Fatalf("save: %d %s", code, body)
	}
	if saved, _ := savedTemplates(t, s).WithID("my-planning"); saved.Team != "Planning" || len(saved.Agents) != 1 {
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
	madeAgent(t, server, "planner", "Planning", true)
	code, body := call(t, server, "POST", "/api/teams/Planning/template", `{"id":"research"}`)
	if code != http.StatusBadRequest || !strings.Contains(body, "the built-in research template has that id") {
		t.Errorf("a built-in id: %d %s", code, body)
	}
}

// The page has no id field, so an id made from the name is always one a
// template may have, whatever the name.
func TestAnIDMadeFromTheNameIsAlwaysValid(t *testing.T) {
	server, _ := newTestServer(t)
	madeAgent(t, server, "planner", "Planning", true)
	for _, made := range []struct{ name, id string }{
		{"Research!", "research-2"},
		{"2025 Team", "team-2025-team"},
		{"!!!", "team"},
		{"Ünïcödé", "n-c-d"},
		{"The Team That Reads Every Paper Twice", "the-team-that-reads-every-paper"},
		{"A Team Whose Name Ends Up Cuts At A Dash", "a-team-whose-name-ends-up-cuts"},
		{"0123456789012345678901234567890123", "team-01234567890123456789012345"},
	} {
		body, _ := json.Marshal(map[string]string{"name": made.name})
		code, reply := call(t, server, "POST", "/api/teams/Planning/template", string(body))
		if code != 200 || !strings.Contains(reply, `"id":"`+made.id+`"`) {
			t.Errorf("%s: want id %s, got %d %s", made.name, made.id, code, reply)
		}
	}
}

// An id made from the name that a template of yours has, as it is or once
// cut, is numbered, still within an id's length.
func TestAnIDMadeFromTheNameThatIsTakenIsNumbered(t *testing.T) {
	server, _ := newTestServer(t)
	madeAgent(t, server, "planner", "Planning", true)
	long := "The Team That Reads Every Paper Twice"
	for _, made := range []struct{ name, id string }{
		{"Planning", "planning"},
		{"Planning!", "planning-2"},
		{"Planning?", "planning-3"},
		{long, "the-team-that-reads-every-paper"},
		{long + " Again", "the-team-that-reads-every-pap-2"},
		{"A Team Whose Name Ends Up Cu Ts", "a-team-whose-name-ends-up-cu-ts"},
		{"A Team Whose Name Ends Up Cu Tsk", "a-team-whose-name-ends-up-cu-2"},
	} {
		body, _ := json.Marshal(map[string]string{"name": made.name})
		code, reply := call(t, server, "POST", "/api/teams/Planning/template", string(body))
		if code != 200 || !strings.Contains(reply, `"id":"`+made.id+`"`) {
			t.Errorf("%s: want id %s, got %d %s", made.name, made.id, code, reply)
		}
	}
}

// A refusal of the team itself is the rules' own words: no id would fix it.
func TestATeamTheRulesRefuseSavedWithoutAnIDSaysWhy(t *testing.T) {
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
	madeAgent(t, server, "planner", "Planning", true)
	code, body := call(t, server, "POST", "/api/teams/Planning/template", `{"id":"software"}`)
	if code != http.StatusBadRequest || strings.TrimSpace(body) != "the built-in software template has that id" {
		t.Errorf("a built-in id you gave: %d %s", code, body)
	}
}

// A team is never added under a built-in template's team name, so a
// template of that name is refused however it comes, saved under the team's
// own name, with an id or without one, or imported, as a name taken (409).
func TestATemplateOfABuiltInTeamIsRefused(t *testing.T) {
	server, _ := newTestServer(t)
	for _, refused := range []struct {
		name, path, body string
		code             int
		want             string
	}{
		{"saved with an id", "/api/teams/Software/template", `{"id":"my-software"}`, http.StatusConflict,
			"Software is a built-in template. Pick another name."},
		{"saved without one", "/api/teams/Software/template", `{}`, http.StatusConflict,
			"Software is a built-in template. Pick another name."},
		{"imported", "/api/teams/templates/import", importOf("my-writing", "writing", ""), http.StatusConflict,
			"Writing is a built-in template. Pick another name."},
	} {
		if code, body := call(t, server, "POST", refused.path, refused.body); code != refused.code || strings.TrimSpace(body) != refused.want {
			t.Errorf("%s: %d %s", refused.name, code, body)
		}
	}
}

// The Software team saved under a name of its own is a template like any.
func TestABuiltInTeamSavedUnderANameOfItsOwnIsSaved(t *testing.T) {
	server, _ := newTestServer(t)
	code, body := call(t, server, "POST", "/api/teams/Software/template", `{"name":"Our Software"}`)
	if code != 200 || !strings.Contains(body, `"id":"our-software"`) || !strings.Contains(body, `"from":"Software"`) {
		t.Errorf("save: %d %s", code, body)
	}
}

func TestATemplateIsRemoved(t *testing.T) {
	server, _ := newTestServer(t)
	madeAgent(t, server, "planner", "Planning", true)
	call(t, server, "POST", "/api/teams/Planning/template", `{"id":"my-planning"}`)
	if code, body := call(t, server, "DELETE", "/api/teams/templates/my-planning", ""); code != 200 {
		t.Fatalf("remove: %d %s", code, body)
	}
	_, list := call(t, server, "GET", "/api/teams/templates", "")
	if yours := userTemplates(t, list); len(yours) != 0 {
		t.Errorf("the template should be gone: %+v", yours)
	}
	if code, body := call(t, server, "DELETE", "/api/teams/templates/my-planning", ""); code != http.StatusNotFound {
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
	members := []agents.Agent{{Name: "planner", Engine: "claude", Prompt: "Plan it.", Coordinates: true}}
	for _, p := range append(agents.SeedProposals(nil), agents.CustomTeam("Planning", members, httpapi.You)...) {
		written.Propose(p)
	}
	saving, err := agents.SaveTemplate(state.Roster(), state.Templates(), state.RemovedTemplates(), "Planning", "Planning", "my-planning", "Ours.", httpapi.You)
	if err != nil {
		t.Fatal(err)
	}
	if _, rejection, err := written.Propose(saving); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	written.Close()

	server, _ := newTestServerOn(t, path, tasks.Limits{})
	_, list := call(t, server, "GET", "/api/teams/templates", "")
	if yours := userTemplates(t, list); len(yours) != 1 || yours[0].ID != "my-planning" || len(yours[0].Agents) != len(members) {
		t.Errorf("after a restart: %+v", yours)
	}
}

// Exporting a template, removing it and importing the file gives back the
// same template, every agent and prompt in it.
func TestAnExportedTemplateImportsAsItWas(t *testing.T) {
	server, s := newTestServer(t)
	planningTeam(t, server, s)
	call(t, server, "POST", "/api/teams/Planning/template", `{"id":"my-planning","description":"Ours."}`)
	want, _ := savedTemplates(t, s).WithID("my-planning")

	request, _ := http.NewRequest("GET", server.URL+"/api/teams/templates/my-planning/export", nil)
	request.Header.Set("Authorization", "Bearer "+token)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	exported, _ := io.ReadAll(response.Body)
	if response.StatusCode != 200 || response.Header.Get("Content-Disposition") != `attachment; filename="my-planning.json"` {
		t.Fatalf("export: %d %q %s", response.StatusCode, response.Header.Get("Content-Disposition"), exported)
	}

	call(t, server, "DELETE", "/api/teams/templates/my-planning", "")
	if code, body := call(t, server, "POST", "/api/teams/templates/import", string(exported)); code != 200 || !strings.Contains(body, `"userMade":true`) {
		t.Fatalf("import: %d %s", code, body)
	}
	got, found := savedTemplates(t, s).WithID("my-planning")
	// Imported, it is saved anew.
	if !found || got.SavedAt.Before(want.SavedAt) {
		t.Fatalf("imported at %v, saved at %v", got.SavedAt, want.SavedAt)
	}
	got.SavedAt, want.SavedAt = time.Time{}, time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("imported:\n%+v\nwant:\n%+v", got, want)
	}
}

// A template keeps the built-in template it is based on through an export
// and an import, and a team added from it passes the base on to a template
// saved from that team.
func TestATemplateKeepsItsBase(t *testing.T) {
	server, s := newTestServer(t)
	ideas := map[string]any{"id": "ideas", "team": "Ideas", "basedOn": "visionaries", "agents": []any{
		map[string]any{"name": "dreamer", "engine": "claude", "prompt": "Dream.", "coordinates": true}}}
	body, _ := json.Marshal(ideas)
	if code, reply := call(t, server, "POST", "/api/teams/templates/import", string(body)); code != 200 {
		t.Fatalf("import: %d %s", code, reply)
	}
	code, exported := call(t, server, "GET", "/api/teams/templates/ideas/export", "")
	call(t, server, "DELETE", "/api/teams/templates/ideas", "")
	if code != 200 || !strings.Contains(exported, `"basedOn":"visionaries"`) {
		t.Fatalf("export: %d %s", code, exported)
	}
	if code, reply := call(t, server, "POST", "/api/teams/templates/import", exported); code != 200 {
		t.Fatalf("import again: %d %s", code, reply)
	}
	if imported, _ := savedTemplates(t, s).WithID("ideas"); imported.BasedOn != "visionaries" {
		t.Fatalf("imported: %+v", imported)
	}

	if code, reply := call(t, server, "POST", "/api/teams", `{"template":"ideas"}`); code != 200 {
		t.Fatalf("add Ideas: %d %s", code, reply)
	}
	if code, reply := call(t, server, "POST", "/api/teams/Ideas/template", `{"name":"More Ideas","id":"more-ideas"}`); code != 200 {
		t.Fatalf("save: %d %s", code, reply)
	}
	if saved, _ := savedTemplates(t, s).WithID("more-ideas"); saved.BasedOn != "visionaries" {
		t.Errorf("saved from a team of yours: %+v", saved)
	}
}

// A team added from a template you then removed still names its id, so a
// template saved from it keeps the base the removed one had.
func TestATeamFromARemovedTemplateKeepsItsBase(t *testing.T) {
	server, s := newTestServer(t)
	ideas := map[string]any{"id": "ideas", "team": "Ideas", "basedOn": "visionaries", "agents": []any{
		map[string]any{"name": "dreamer", "engine": "claude", "prompt": "Dream.", "coordinates": true}}}
	body, _ := json.Marshal(ideas)
	call(t, server, "POST", "/api/teams/templates/import", string(body))
	if code, reply := call(t, server, "POST", "/api/teams", `{"template":"ideas"}`); code != 200 {
		t.Fatalf("add Ideas: %d %s", code, reply)
	}
	call(t, server, "DELETE", "/api/teams/templates/ideas", "")
	if code, reply := call(t, server, "POST", "/api/teams/Ideas/template", `{"name":"More Ideas","id":"more-ideas"}`); code != 200 {
		t.Fatalf("save: %d %s", code, reply)
	}
	if saved, _ := savedTemplates(t, s).WithID("more-ideas"); saved.BasedOn != "visionaries" {
		t.Errorf("saved from a removed template's team: %+v", saved)
	}
}

// A removed template's id stays taken, as its teams still name it: a save
// giving it is refused, one without an id gets another, and only importing
// the removed template's file brings it back.
func TestARemovedTemplatesIDStaysTaken(t *testing.T) {
	server, s := newTestServer(t)
	madeAgent(t, server, "planner", "Planning", true)
	call(t, server, "POST", "/api/teams/Planning/template", `{"description":"Ours."}`)
	_, exported := call(t, server, "GET", "/api/teams/templates/planning/export", "")
	call(t, server, "DELETE", "/api/teams/templates/planning", "")

	want := "You removed a template with the id planning. Import its file to bring it back, or pick another id."
	if code, body := call(t, server, "POST", "/api/teams/Planning/template", `{"name":"Others","id":"planning"}`); code != http.StatusConflict || strings.TrimSpace(body) != want {
		t.Errorf("a new save with the removed id: %d %s", code, body)
	}
	if code, body := call(t, server, "POST", "/api/teams/Planning/template", `{}`); code != 200 || !strings.Contains(body, `"id":"planning-2"`) {
		t.Errorf("a new save without an id: %d %s", code, body)
	}
	call(t, server, "DELETE", "/api/teams/templates/planning-2", "")
	if code, body := call(t, server, "POST", "/api/teams/templates/import", exported); code != 200 {
		t.Fatalf("import: %d %s", code, body)
	}
	if saved, found := savedTemplates(t, s).WithID("planning"); !found || saved.Description != "Ours." {
		t.Errorf("the import brought back %+v", saved)
	}
}

func TestImportingATemplateYouHaveReplacesIt(t *testing.T) {
	server, s := newTestServer(t)
	planningTeam(t, server, s)
	call(t, server, "POST", "/api/teams/Planning/template", `{"id":"my-planning","description":"Ours."}`)
	_, exported := call(t, server, "GET", "/api/teams/templates/my-planning/export", "")
	changed := strings.Replace(exported, `"description":"Ours."`, `"description":"Imported."`, 1)
	if code, body := call(t, server, "POST", "/api/teams/templates/import", changed); code != 200 {
		t.Fatalf("import: %d %s", code, body)
	}
	if saved := savedTemplates(t, s); len(saved) != 1 || saved[0].Description != "Imported." {
		t.Errorf("one template, replaced: %+v", saved)
	}
}

// An import is refused a name, in any case, that another of yours has, in
// the words a save gets, and nothing is written. Under the id of one of
// yours it replaces that one, so the name it has is free.
func TestAnImportTakesNoNameOfAnotherOfYours(t *testing.T) {
	server, s := newTestServer(t)
	for _, first := range []string{importOf("readers", "Readers", ""), importOf("writers", "Writers", "")} {
		if code, body := call(t, server, "POST", "/api/teams/templates/import", first); code != 200 {
			t.Fatalf("import: %d %s", code, body)
		}
	}
	before, _ := s.Stream(events.TemplatesStream)
	for _, refused := range []struct{ name, body string }{
		{"the same name, another id", importOf("readers-2", "Readers", "")},
		{"the name in another case", importOf("readers-2", "READERS", "")},
		{"the same id, another's name", importOf("writers", "readers", "")},
	} {
		if code, body := call(t, server, "POST", "/api/teams/templates/import", refused.body); code != http.StatusConflict ||
			strings.TrimSpace(body) != "You have a template called Readers already. Pick another name." {
			t.Errorf("%s: %d %s", refused.name, code, body)
		}
	}
	if after, _ := s.Stream(events.TemplatesStream); len(after) != len(before) {
		t.Errorf("nothing should be written: %d template events, then %d", len(before), len(after))
	}
	for _, replacing := range []string{importOf("readers", "READERS", ""), importOf("readers", "Reading Room", "")} {
		if code, body := call(t, server, "POST", "/api/teams/templates/import", replacing); code != 200 {
			t.Errorf("replacing readers: %d %s", code, body)
		}
	}
	if saved := savedTemplates(t, s); len(saved) != 2 || saved[0].ID != "readers" || saved[0].Team != "Reading Room" {
		t.Errorf("readers replaced in its place: %+v", saved)
	}
}

// Imports of one name sent at once can all pass the check; the rules then
// refuse all but the first, which the page hears as the same 409.
func TestImportsOfOneNameAtOnceKeepTheFirst(t *testing.T) {
	server, s := newTestServer(t)
	const imports = 20
	codes, bodies := make([]int, imports), make([]string, imports)
	var wg sync.WaitGroup
	for i := range imports {
		wg.Go(func() {
			codes[i], bodies[i] = call(t, server, "POST", "/api/teams/templates/import", importOf(fmt.Sprintf("readers-%d", i), "Readers", ""))
		})
	}
	wg.Wait()
	imported := 0
	for i, code := range codes {
		switch {
		case code == 200:
			imported++
		case code != http.StatusConflict || strings.TrimSpace(bodies[i]) != "You have a template called Readers already. Pick another name.":
			t.Errorf("import %d: %d %s", i, code, bodies[i])
		}
	}
	if stream, _ := s.Stream(events.TemplatesStream); imported != 1 || len(stream) != 1 {
		t.Errorf("%d imports answered as imported and %d written, want one", imported, len(stream))
	}
}

// An imported team and description are trimmed, as a saved description is.
func TestAPaddedImportIsSavedTrimmed(t *testing.T) {
	server, s := newTestServer(t)
	planningTeam(t, server, s)
	call(t, server, "POST", "/api/teams/Planning/template", `{"id":"my-planning","description":"Ours."}`)
	_, exported := call(t, server, "GET", "/api/teams/templates/my-planning/export", "")
	padded := exported
	for target, padding := range map[string]string{`"description":"Ours."`: `"description":"  Ours.  "`, `"team":"Planning"`: `"team":"  Planning  "`} {
		if !strings.Contains(padded, target) {
			t.Fatalf("nothing to pad: no %s in %s", target, exported)
		}
		padded = strings.Replace(padded, target, padding, 1)
	}
	if code, body := call(t, server, "POST", "/api/teams/templates/import", padded); code != 200 {
		t.Fatalf("import: %d %s", code, body)
	}
	if saved, _ := savedTemplates(t, s).WithID("my-planning"); saved.Team != "Planning" || saved.Description != "Ours." {
		t.Errorf("saved team %q, description %q", saved.Team, saved.Description)
	}
	if _, reexported := call(t, server, "GET", "/api/teams/templates/my-planning/export", ""); reexported != exported {
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
		{"nothing", map[string]any{}, "a template needs a team name"},
		{"an unknown base", map[string]any{"id": "ours", "team": "Ours", "basedOn": "dreamers", "agents": []any{agent("lead", true)}},
			"a template is based on a built-in template, and there is no built-in template dreamers"},
	} {
		body, _ := json.Marshal(refused.template)
		if code, reply := call(t, server, "POST", "/api/teams/templates/import", string(body)); code != http.StatusBadRequest || strings.TrimSpace(reply) != refused.reason {
			t.Errorf("%s: %d %s", refused.name, code, reply)
		}
	}
}

func TestAnImportThatIsntATemplateIsABadRequest(t *testing.T) {
	server, _ := newTestServer(t)
	for _, body := range []string{`{"id":"ours","agents":"lead"}`, `{"id":7}`, `{"id":"ours","basedOn":7}`, `[]`, `not json`} {
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

// A team added from your template whose coordinator has no description
// takes the template's, so other coordinators know what it is for; a
// coordinator with one keeps it.
func TestYourTemplatesDescriptionDescribesItsCoordinator(t *testing.T) {
	server, s := newTestServer(t)
	for _, c := range []struct{ id, team, lead, want string }{
		{"quiet", "Quiet", "", "Keeps the minutes."},
		{"loud", "Loud", "Shouts the news.", "Shouts the news."},
	} {
		body, _ := json.Marshal(map[string]any{"id": c.id, "team": c.team, "description": "Keeps the minutes.", "agents": []any{
			map[string]any{"name": c.id + "-lead", "engine": "claude", "prompt": "Lead.", "coordinates": true, "profile": "developer", "description": c.lead}}})
		if code, reply := call(t, server, "POST", "/api/teams/templates/import", string(body)); code != 200 {
			t.Fatalf("%s: import: %d %s", c.id, code, reply)
		}
		if code, reply := call(t, server, "POST", "/api/teams", `{"template":"`+c.id+`"}`); code != 200 {
			t.Fatalf("%s: add: %d %s", c.id, code, reply)
		}
		if lead, _ := rosterOf(t, s).Coordinator(c.team); lead.Description != c.want {
			t.Errorf("%s: the lead's description is %q, want %q", c.id, lead.Description, c.want)
		}
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

// A template saved before templates were named has no "from" in the log:
// it is listed with when it was saved and no source team.
func TestATemplateSavedWithoutFromIsListedWithout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	state := storetest.NewState(events.Core())
	written, err := store.Open(path, events.Core(), state, func(p events.Proposal) *events.Rejection { return rules.Check(p, state, events.Core()) })
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range agents.SeedProposals(nil) {
		written.Propose(p)
	}
	old := events.Proposal{Stream: events.TemplatesStream, Actor: httpapi.You, Type: events.TemplateSaved, Data: map[string]any{
		"id": "papers", "team": "Papers", "description": "Reads papers.", "agents": []agents.Agent{
			{Name: "lead", Engine: "claude", Prompt: "Lead.", Coordinates: true}}}}
	if _, rejection, err := written.Propose(old); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	written.Close()

	server, _ := newTestServerOn(t, path, tasks.Limits{})
	_, list := call(t, server, "GET", "/api/teams/templates", "")
	yours := userTemplates(t, list)
	if len(yours) != 1 || yours[0].Team != "Papers" || yours[0].From != "" || strings.Contains(list, `"from"`) {
		t.Fatalf("an old template: %+v", yours)
	}
	if _, err := time.Parse(time.RFC3339, yours[0].SavedAt); err != nil {
		t.Errorf("saved at %q: %v", yours[0].SavedAt, err)
	}
}
