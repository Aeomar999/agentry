package workersapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers/workerlink"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

// The routes a joined worker calls. Its link and a task's repo are tested
// end to end in web (workers_test.go), with a worker that runs a turn.

var workerRoutes = []string{
	"GET " + workerlink.ConnectPath,
	"GET " + workerlink.StreamPath + "s-1",
	"GET " + workerlink.GitPath + "task-1/",
	"POST " + workerlink.GitPath + "task-1/git-upload-pack",
	"POST " + workerlink.GitPath + "task-1/bundle",
	"GET " + workerlink.UpdatePath,
	"POST " + workerlink.LeavePath,
}

// workerServer is web's server, with a hub for workers when withHub, and
// the credential of laptop, a worker that joined it.
func workerServer(t *testing.T, withHub bool) (*httptest.Server, string) {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	code, invite := workers.Invite(httpapi.You, time.Now())
	credential, join := workers.Join("laptop", code, workers.Machine{Host: "laptop.lan"})
	for _, p := range []events.Proposal{invite, join} {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatal(rejection, err)
		}
	}
	config := web.Config{UIToken: token}
	if withHub {
		fleet := machines.NewFleet(&machines.Local{Label: workers.Local})
		config.Fleet, config.Workers = fleet, workerlink.NewServer(s, fleet)
		config.Binary = filepath.Join(t.TempDir(), "agentry")
		os.WriteFile(config.Binary, []byte("the agentry binary"), 0o755)
	}
	server := httptest.NewServer(web.NewServer(s, state, config, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return server, credential
}

func asWorker(t *testing.T, server *httptest.Server, route, credential string) (int, string) {
	t.Helper()
	method, path, _ := strings.Cut(route, " ")
	request, _ := http.NewRequest(method, server.URL+path, nil)
	if credential != "" {
		request.Header.Set("Authorization", "Bearer "+credential)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	return response.StatusCode, string(content)
}

// A server with no hub for workers has none of a worker's routes, so a
// joined worker's credential reaches nothing there: the page's GET /
// answers a GET 404, and a POST is 405, as no route takes it.
func TestWithNoHubAWorkersRoutesAreNotThere(t *testing.T) {
	server, credential := workerServer(t, false)
	for _, route := range workerRoutes {
		want := http.StatusNotFound
		if strings.HasPrefix(route, "POST ") {
			want = http.StatusMethodNotAllowed
		}
		if code, body := asWorker(t, server, route, credential); code != want {
			t.Errorf("%s with no hub: %d %s, want %d", route, code, body, want)
		}
	}
}

// Every route a worker calls takes its credential and nothing else: not
// none, not the UI token.
func TestAWorkersRoutesTakeOnlyItsCredential(t *testing.T) {
	server, _ := workerServer(t, true)
	for _, route := range workerRoutes {
		for _, given := range []string{"", token, "not-a-credential"} {
			if code, body := asWorker(t, server, route, given); code != http.StatusUnauthorized || !strings.Contains(body, "this worker isn't joined") {
				t.Errorf("%s with %q: %d %s, want 401", route, given, code, body)
			}
		}
	}
}

// A worker updating itself gets agentry for its kind, and leaving takes it
// off, so its credential stops working.
func TestAWorkerUpdatesItselfAndLeaves(t *testing.T) {
	server, credential := workerServer(t, true)
	update := "GET " + workerlink.UpdatePath + "?os=" + runtime.GOOS + "&arch=" + runtime.GOARCH
	if code, body := asWorker(t, server, update, credential); code != 200 || body != "the agentry binary" {
		t.Errorf("the update: %d %q", code, body)
	}
	if code, body := asWorker(t, server, "POST "+workerlink.LeavePath, credential); code != 200 {
		t.Fatalf("leaving: %d %s", code, body)
	}
	if code, body := call(t, server, "GET", "/api/workers", ""); code != 200 || !strings.Contains(body, `"workers":[]`) {
		t.Errorf("the Workers page after it left: %d %s", code, body)
	}
	if code, _ := asWorker(t, server, update, credential); code != http.StatusUnauthorized {
		t.Errorf("the credential after leaving: %d, want 401", code)
	}
}
