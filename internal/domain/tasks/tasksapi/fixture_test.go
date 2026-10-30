package tasksapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

// The routes' tests' fixture, each piece mirroring
// internal/web/api_test.go's.

const token = "ui-secret"

// newTestServer is a server on a log of its own, with the default agents
// and no limits for new tasks.
func newTestServer(t *testing.T) (*httptest.Server, *store.Store) {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	hooks := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("hooks")) })
	server := httptest.NewServer(web.NewServer(s, state, web.Config{UIToken: token, DefaultRepo: "/src/app", Clones: filepath.Join(t.TempDir(), "repos"),
		Blobs: sharedfiles.Disk{Dir: filepath.Join(t.TempDir(), "files")}}, hooks, nil).Handler())
	t.Cleanup(server.Close)
	return server, s
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
