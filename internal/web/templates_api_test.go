package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/httpapi"
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
