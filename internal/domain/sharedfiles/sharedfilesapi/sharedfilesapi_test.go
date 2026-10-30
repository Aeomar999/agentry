package sharedfilesapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

// The shared files' routes, tested over HTTP through the server serve runs,
// as web mounts them: that is where their token check is.

const token = "ui-secret"

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: token, DefaultRepo: "/src/app",
		Blobs: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}}, http.NotFoundHandler(), nil).Handler())
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

// You share files with a task, download them, and take them away.
func TestYouShareFilesWithATask(t *testing.T) {
	server := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Crunch the numbers"}`)
	request, _ := http.NewRequest("POST", server.URL+"/api/tasks/task-1/shared?name=data/sales.csv", strings.NewReader("a,b\n1,2\n"))
	request.Header.Set("Authorization", "Bearer "+token)
	if response, err := http.DefaultClient.Do(request); err != nil || response.StatusCode != 200 {
		t.Fatalf("upload: %v %v", err, response.StatusCode)
	}
	_, body := call(t, server, "GET", "/api/tasks/task-1/shared", "")
	if !strings.Contains(body, `"name":"data/sales.csv"`) || !strings.Contains(body, `"size":8`) || !strings.Contains(body, `"by":"you"`) {
		t.Errorf("list: %s", body)
	}
	if _, body := call(t, server, "GET", "/api/tasks/task-1", ""); !strings.Contains(body, `"shared":[{"name":"data/sales.csv"`) {
		t.Errorf("the task should show it: %s", body)
	}
	if code, body := call(t, server, "GET", "/api/tasks/task-1/shared/file?name=data/sales.csv", ""); code != 200 || body != "a,b\n1,2\n" {
		t.Errorf("download: %d %q", code, body)
	}
	bad, _ := http.NewRequest("POST", server.URL+"/api/tasks/task-1/shared?name=../escape", strings.NewReader("x"))
	bad.Header.Set("Authorization", "Bearer "+token)
	if response, _ := http.DefaultClient.Do(bad); response.StatusCode != http.StatusBadRequest {
		t.Errorf("a name outside the folder: %d", response.StatusCode)
	}
	if code, body := call(t, server, "DELETE", "/api/tasks/task-1/shared?name=data/sales.csv", ""); code != 200 {
		t.Fatalf("remove: %d %s", code, body)
	}
	if _, body := call(t, server, "GET", "/api/tasks/task-1/shared", ""); strings.TrimSpace(body) != "[]" {
		t.Errorf("after removing: %s", body)
	}
}
