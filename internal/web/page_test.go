package web

import (
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/fstest"
)

// A binary built without `make ui` embeds only notbuilt.html. Every page
// path gets it, with status 200, and the API still answers.
func TestServesTheNotBuiltPageWithoutTheUI(t *testing.T) {
	notBuilt := "<h1>Agentry's page isn't built</h1> run make ui"
	before := static
	static = fstest.MapFS{"notbuilt.html": {Data: []byte(notBuilt)}}
	t.Cleanup(func() { static = before })
	server, _ := newTestServer(t)

	for _, path := range []string{"/", "/tasks", "/tasks/task-1?tab=events", "/assets/index.js", "/sw.js"} {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(response.Body)
		response.Body.Close()
		if response.StatusCode != 200 || string(body) != notBuilt || !strings.HasPrefix(response.Header.Get("Content-Type"), "text/html") {
			t.Errorf("%s: %d %q, not the not-built page", path, response.StatusCode, body)
		}
	}

	if code, _ := call(t, server, "GET", "/api/tasks", ""); code != 200 {
		t.Errorf("/api/tasks: %d", code)
	}
	if code, _ := call(t, server, "GET", "/api/nothing", ""); code != http.StatusNotFound {
		t.Errorf("/api/nothing: %d", code)
	}
}

// The committed notbuilt.html says how to build the page.
func TestTheNotBuiltPageSaysHowToBuildIt(t *testing.T) {
	page, err := embedded.ReadFile("static/notbuilt.html")
	if err != nil {
		t.Fatal(err)
	}
	for _, says := range []string{"isn't built", "make ui", "Node 24"} {
		if !strings.Contains(string(page), says) {
			t.Errorf("notbuilt.html doesn't say %q", says)
		}
	}
}
