package web

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/features"
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
