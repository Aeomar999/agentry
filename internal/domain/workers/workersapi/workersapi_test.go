package workersapi_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

// The Workers page's routes, tested over HTTP through the server serve
// runs, as web mounts them: that is where their token check is. A worker
// joining and taking work is tested end to end in web (workers_test.go).

const token = "ui-secret"

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	local := workers.Machine{Host: "desk.lan", OS: "linux", Arch: "amd64", Version: "v1"}
	server := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: token, Local: local}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return server
}

// mirrors internal/domain/connections/connectionsapi/connectionsapi_test.go
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

func TestTheWorkersRoutesTakeTheUIToken(t *testing.T) {
	server := newTestServer(t)
	for _, route := range []string{"GET /api/workers", "POST /api/workers/invite", "GET /api/workers/local", "DELETE /api/workers/laptop"} {
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

// With no worker joined, the page lists none and this machine, with
// empty lists rather than null.
func TestTheWorkersPageShowsThisMachine(t *testing.T) {
	server := newTestServer(t)
	code, body := call(t, server, "GET", "/api/workers", "")
	if code != 200 || !strings.Contains(body, `"workers":[]`) || !strings.Contains(body, `"tasks":[]`) || !strings.Contains(body, `"host":"desk.lan"`) {
		t.Errorf("the list: %d %s", code, body)
	}

	code, body = call(t, server, "GET", "/api/workers/local", "")
	var detail struct {
		Worker struct {
			Name   string   `json:"name"`
			Online bool     `json:"online"`
			Tasks  []string `json:"tasks"`
		} `json:"worker"`
		About   any      `json:"about"`
		Agents  []string `json:"agents"`
		History []any    `json:"history"`
	}
	json.Unmarshal([]byte(body), &detail)
	if code != 200 || detail.Worker.Name != workers.Local || !detail.Worker.Online || detail.Worker.Tasks == nil ||
		detail.About != nil || len(detail.Agents) == 0 || detail.History == nil {
		t.Errorf("this machine in detail: %d %s", code, body)
	}

	if code, body := call(t, server, "GET", "/api/workers/laptop", ""); code != http.StatusNotFound || !strings.Contains(body, "no worker called laptop") {
		t.Errorf("an unknown worker: %d %s", code, body)
	}
}

// A join code comes with the command that uses it.
func TestAnInviteGivesAJoinCode(t *testing.T) {
	server := newTestServer(t)
	code, body := call(t, server, "POST", "/api/workers/invite", "")
	var invite struct {
		Code    string `json:"code"`
		Command string `json:"command"`
	}
	json.Unmarshal([]byte(body), &invite)
	if code != 200 || invite.Code == "" || !strings.Contains(invite.Command, invite.Code) {
		t.Errorf("invite: %d %s", code, body)
	}
}
