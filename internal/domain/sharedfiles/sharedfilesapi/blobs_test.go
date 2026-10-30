package sharedfilesapi_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers/workerlink"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
	"gitlab.com/dev.chempa/agentry/internal/web"
)

// The shared files a worker fetches and sends by their hash, with its
// credential. A worker's whole turn with them is tested end to end in web
// (workers_test.go).

const missing = "0000000000000000000000000000000000000000000000000000000000000000"

var blobRoutes = []string{
	"HEAD " + sharedfiles.BlobsPath + "/" + missing,
	"GET " + sharedfiles.BlobsPath + "/" + missing,
	"POST " + sharedfiles.BlobsPath,
}

// blobServer is web's server keeping its blobs in blobs, with a hub for
// workers when withHub, and the credential of laptop, a worker that
// joined it.
func blobServer(t *testing.T, withHub bool, blobs string) (*httptest.Server, string) {
	t.Helper()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	code, invite := workers.Invite(httpapi.You, time.Now())
	credential, join := workers.Join("laptop", code, workers.Machine{Host: "laptop.lan"})
	for _, p := range []events.Proposal{invite, join} {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatal(rejection, err)
		}
	}
	config := web.Config{UIToken: token, Blobs: sharedfiles.Disk{Dir: blobs}}
	if withHub {
		fleet := machines.NewFleet(&machines.Local{Label: workers.Local})
		config.Fleet, config.Workers = fleet, workerlink.NewServer(s, fleet)
	}
	server := httptest.NewServer(web.NewServer(s, state, config, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	return server, credential
}

func asWorker(t *testing.T, server *httptest.Server, route, credential, body string) (*http.Response, string) {
	t.Helper()
	method, path, _ := strings.Cut(route, " ")
	request, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
	if credential != "" {
		request.Header.Set("Authorization", "Bearer "+credential)
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	return response, string(content)
}

// A worker sends a file and fetches it back by the hash it was given; a
// hash the server doesn't have is no such file.
func TestAWorkerSendsAndFetchesAFileByItsHash(t *testing.T) {
	server, credential := blobServer(t, true, t.TempDir())
	response, body := asWorker(t, server, "POST "+sharedfiles.BlobsPath, credential, "a,b\n1,2\n")
	hash := strings.TrimSuffix(strings.TrimPrefix(body, `{"hash":"`), "\",\"size\":8}\n")
	if response.StatusCode != 200 || len(hash) != 64 {
		t.Fatalf("sending: %d %q", response.StatusCode, body)
	}
	if response, body := asWorker(t, server, "HEAD "+sharedfiles.BlobsPath+"/"+hash, credential, ""); response.StatusCode != 200 || body != "" {
		t.Errorf("HEAD: %d %q", response.StatusCode, body)
	}
	response, body = asWorker(t, server, "GET "+sharedfiles.BlobsPath+"/"+hash, credential, "")
	if response.StatusCode != 200 || body != "a,b\n1,2\n" || response.Header.Get("Content-Type") != "application/octet-stream" {
		t.Errorf("GET: %d %q %s", response.StatusCode, body, response.Header.Get("Content-Type"))
	}
	if response, _ := asWorker(t, server, "HEAD "+sharedfiles.BlobsPath+"/"+missing, credential, ""); response.StatusCode != http.StatusNotFound {
		t.Errorf("HEAD a missing hash: %d, want 404", response.StatusCode)
	}
	if response, body := asWorker(t, server, "GET "+sharedfiles.BlobsPath+"/"+missing, credential, ""); response.StatusCode != http.StatusNotFound || body != "no such file\n" {
		t.Errorf("GET a missing hash: %d %q, want 404 no such file", response.StatusCode, body)
	}
}

// A file the server can't keep is refused with why.
func TestAFileTheServerCantKeepIsRefused(t *testing.T) {
	notAFolder := filepath.Join(t.TempDir(), "files")
	os.WriteFile(notAFolder, nil, 0o600)
	server, credential := blobServer(t, true, notAFolder)
	response, body := asWorker(t, server, "POST "+sharedfiles.BlobsPath, credential, "x")
	if response.StatusCode != http.StatusBadRequest || !strings.Contains(body, "not a directory") {
		t.Errorf("sending: %d %q, want 400", response.StatusCode, body)
	}
}

// The blob routes take a worker's credential and nothing else: not none,
// not the UI token.
func TestTheBlobRoutesTakeOnlyAWorkersCredential(t *testing.T) {
	server, _ := blobServer(t, true, t.TempDir())
	for _, route := range blobRoutes {
		for _, given := range []string{"", token, "not-a-credential"} {
			response, body := asWorker(t, server, route, given, "x")
			if response.StatusCode != http.StatusUnauthorized || (!strings.HasPrefix(route, "HEAD ") && !strings.Contains(body, "this worker isn't joined")) {
				t.Errorf("%s with %q: %d %s, want 401", route, given, response.StatusCode, body)
			}
		}
	}
}

// A server with no hub for workers has no blob routes: the page's GET /
// answers a GET or HEAD 404, and a POST is 405, as no route takes it.
func TestWithNoHubTheBlobRoutesAreNotThere(t *testing.T) {
	server, credential := blobServer(t, false, t.TempDir())
	for _, route := range blobRoutes {
		want := http.StatusNotFound
		if strings.HasPrefix(route, "POST ") {
			want = http.StatusMethodNotAllowed
		}
		if response, body := asWorker(t, server, route, credential, "x"); response.StatusCode != want {
			t.Errorf("%s with no hub: %d %s, want %d", route, response.StatusCode, body, want)
		}
	}
}
