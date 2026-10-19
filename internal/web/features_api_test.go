package web

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// ideasLine is Ideas' line in GET /api/features.
func ideasLine(t *testing.T, server *httptest.Server) features.Line {
	t.Helper()
	code, body := call(t, server, "GET", "/api/features", "")
	var manifest struct{ Features []features.Line }
	if err := json.Unmarshal([]byte(body), &manifest); code != 200 || err != nil {
		t.Fatalf("features: %d %v %s", code, err, body)
	}
	for _, line := range manifest.Features {
		if line.Name == "ideas" {
			return line
		}
	}
	t.Fatalf("no ideas in %s", body)
	return features.Line{}
}

func TestIdeasHiddenWithNoTeamAndNoIdeas(t *testing.T) {
	server, _ := newTestServer(t)
	line := ideasLine(t, server)
	if line.Enabled || line.Visible || line.Badge != 0 || !strings.Contains(line.Why, "add the Visionaries team") {
		t.Errorf("no team, no ideas: %+v", line)
	}
}

func TestIdeasOnWithTheVisionariesTeam(t *testing.T) {
	server, _ := newTestServer(t)
	if code, body := call(t, server, "POST", "/api/teams", `{"template":"Visionaries"}`); code != 200 {
		t.Fatalf("add Visionaries: %d %s", code, body)
	}
	line := ideasLine(t, server)
	if !line.Enabled || !line.Visible || line.Why != "the Visionaries team suggests ideas" {
		t.Errorf("with the team: %+v", line)
	}
}

func TestIdeasKeptForTheirIdeasWhenTheTeamIsRemoved(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/teams", `{"template":"Visionaries"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	suggestIdea(t, s, "task-1", repo, "Search the board")
	suggestIdea(t, s, "task-1", repo, "Theme the board")
	if code, body := call(t, server, "DELETE", "/api/teams/Visionaries", ""); code != 200 {
		t.Fatalf("remove Visionaries: %d %s", code, body)
	}
	line := ideasLine(t, server)
	if line.Enabled || !line.Visible || line.Why != "kept while 2 ideas remain; the Visionaries team was removed" {
		t.Errorf("team removed, ideas left: %+v", line)
	}
}

func TestIdeasBadgeCountsOpenIdeas(t *testing.T) {
	server, s := newTestServer(t)
	repo := gitRepo(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Think up features","repo":"`+repo+`"}`)
	suggestIdea(t, s, "task-1", repo, "Search the board")
	accepted := suggestIdea(t, s, "task-1", repo, "Theme the board")
	dismissed := suggestIdea(t, s, "task-1", repo, "Sound on done")
	suggestIdea(t, s, "task-1", repo, "Export the board")
	if code, body := call(t, server, "POST", "/api/ideas/"+accepted+"/accept", `{}`); code != 200 {
		t.Fatalf("accept: %d %s", code, body)
	}
	if code, body := call(t, server, "POST", "/api/ideas/"+dismissed+"/dismiss", `{"note":"No"}`); code != 200 {
		t.Fatalf("dismiss: %d %s", code, body)
	}
	if line := ideasLine(t, server); line.Badge != 2 || !line.Visible {
		t.Errorf("two of four ideas open: %+v", line)
	}
}

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
	newer, err := events.NewCatalogue(ideas.Registration(),
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
