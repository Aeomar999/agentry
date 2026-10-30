package connectionsapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

// The connections' routes, tested over HTTP through the server serve runs,
// as web mounts them: that is where their token check is, and where the
// install script and connect a machine calls with its code are.

const token = "ui-secret"

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: token}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return server
}

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

// Claude Code on another machine connects with an install command from the
// Integrations page: its code works once, the credential it gets opens the
// API, and revoking the connection closes it again.
func TestClaudeCodeConnectsFromTheIntegrationsPage(t *testing.T) {
	server := newTestServer(t)
	code, body := call(t, server, "POST", "/api/integrations/claude-code", "")
	if code != 200 || !strings.Contains(body, "| sh") {
		t.Fatalf("invite: %d %s", code, body)
	}
	var invite struct{ Command string }
	json.Unmarshal([]byte(body), &invite)
	installCode := invite.Command[strings.Index(invite.Command, "code=")+5 : strings.LastIndex(invite.Command, "'")]

	response, err := http.Get(server.URL + "/integrations/claude-code.sh?code=" + installCode)
	if err != nil || response.StatusCode != 200 {
		t.Fatalf("script: %v %v", err, response.StatusCode)
	}
	script, _ := io.ReadAll(response.Body)
	if !strings.Contains(string(script), "code='"+installCode+"'") || !strings.Contains(string(script), "mcp connect") {
		t.Errorf("the script should carry its code:\n%s", script)
	}

	connect := func() (int, string) {
		response, _ := http.Post(server.URL+"/api/integrations/connect", "application/json",
			strings.NewReader(`{"code":"`+installCode+`","host":"laptop","user":"me","os":"darwin"}`))
		got, _ := io.ReadAll(response.Body)
		return response.StatusCode, string(got)
	}
	status, got := connect()
	var made struct{ Credential string }
	json.Unmarshal([]byte(got), &made)
	if status != 200 || made.Credential == "" {
		t.Fatalf("connect: %d %s", status, got)
	}
	if status, _ := connect(); status != http.StatusConflict {
		t.Errorf("the code worked twice: %d", status)
	}
	if response, _ := http.Get(server.URL + "/integrations/claude-code.sh?code=" + installCode); response.StatusCode != http.StatusUnauthorized {
		t.Errorf("the script came with a used code: %d", response.StatusCode)
	}

	asConnection := func() int {
		request, _ := http.NewRequest("GET", server.URL+"/api/tasks", nil)
		request.Header.Set("Authorization", "Bearer "+made.Credential)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode
	}
	if code := asConnection(); code != 200 {
		t.Errorf("the credential should open the API: %d", code)
	}
	_, body = call(t, server, "GET", "/api/integrations/connections", "")
	var listed struct{ Connections []struct{ ID, Host string } }
	json.Unmarshal([]byte(body), &listed)
	if len(listed.Connections) != 1 || listed.Connections[0].Host != "laptop" || strings.Contains(body, made.Credential) {
		t.Fatalf("connections: %s", body)
	}
	if code, body := call(t, server, "DELETE", "/api/integrations/connections/"+listed.Connections[0].ID, ""); code != 200 {
		t.Fatalf("revoke: %d %s", code, body)
	}
	if code := asConnection(); code != http.StatusUnauthorized {
		t.Errorf("a revoked credential still works: %d", code)
	}
}

// Without the UI token none of the connections' routes answers, and nothing
// is invited or revoked.
func TestTheConnectionsRoutesNeedTheToken(t *testing.T) {
	server := newTestServer(t)
	for _, route := range []struct{ method, path string }{
		{"POST", "/api/integrations/claude-code"},
		{"GET", "/api/integrations/connections"},
		{"DELETE", "/api/integrations/connections/connection-1"},
	} {
		request, _ := http.NewRequest(route.method, server.URL+route.path, nil)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusUnauthorized {
			t.Errorf("%s %s with no token: want 401, got %d", route.method, route.path, response.StatusCode)
		}
	}
	if _, body := call(t, server, "GET", "/api/integrations/connections", ""); body != `{"connections":[]}`+"\n" {
		t.Errorf("nothing should be connected: %s", body)
	}
}
