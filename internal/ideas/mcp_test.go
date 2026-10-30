package ideas_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/mcp"
	"gitlab.com/dev.chempa/agentry/internal/platform/client"
)

// In agentry mcp, Ideas brings its three tools and its instructions only
// while GET /api/features shows it. A session that started with it hidden
// is told when the Visionaries team is added (the log moves and Ideas is
// enabled), and lists them from then on.
func TestAgentryMCPListsTheIdeasToolsOnlyWhileIdeasShows(t *testing.T) {
	var enabled atomic.Bool
	added, done := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/features":
			fmt.Fprintf(w, `{"features":[{"name":"ideas","enabled":%v,"visible":%[1]v}]}`, enabled.Load())
		case "/api/changes":
			switch r.URL.Query().Get("after") { // the log is at 1, and at 2 once the team is added
			case "0":
			case "1":
				select {
				case <-added:
				case <-done:
				}
			default:
				<-done
			}
			fmt.Fprintf(w, `{"position":%d}`, map[bool]int{false: 1, true: 2}[enabled.Load()])
		default:
			io.WriteString(w, `[]`)
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(done) })
	agentry := client.Client{Where: client.Where{URL: server.URL}}

	session := func() (call func(method string) json.RawMessage, next func() (method string)) {
		s := mcp.New(agentry, t.TempDir(), "test", []extensions.Extension{ideas.Extension()})
		toServer, fromClaude := io.Pipe()
		fromServer, toClaude := io.Pipe()
		go s.Serve(context.Background(), toServer, toClaude)
		t.Cleanup(func() { fromClaude.Close(); toClaude.Close() })
		out := bufio.NewScanner(fromServer)
		id := 0
		read := func() (m struct {
			ID     *int
			Method string
			Result json.RawMessage
		}) {
			if !out.Scan() {
				t.Fatal("the server stopped")
			}
			json.Unmarshal(out.Bytes(), &m)
			return m
		}
		call = func(method string) json.RawMessage {
			id++
			fmt.Fprintf(fromClaude, `{"jsonrpc":"2.0","id":%d,"method":%q,"params":{}}`+"\n", id, method)
			for m := read(); ; m = read() {
				if m.ID != nil && *m.ID == id {
					return m.Result
				}
			}
		}
		next = func() string { // what the server sends next, within a few seconds
			t.Helper()
			scanned := make(chan bool, 1)
			go func() { scanned <- out.Scan() }()
			select {
			case ok := <-scanned:
				if !ok {
					t.Fatal("the server stopped")
				}
			case <-time.After(5 * time.Second):
				t.Fatal("the server sent nothing within 5 s")
			}
			var m struct{ Method string }
			json.Unmarshal(out.Bytes(), &m)
			return m.Method
		}
		return call, next
	}
	tools := func(call func(string) json.RawMessage) []string {
		var listed struct{ Tools []struct{ Name string } }
		json.Unmarshal(call("tools/list"), &listed)
		var names []string
		for _, tool := range listed.Tools {
			names = append(names, tool.Name)
		}
		return names
	}
	ideasTools := []string{"list_ideas", "accept_idea", "dismiss_idea"}

	call, next := session()
	if hello := string(call("initialize")); strings.Contains(hello, "- Ideas:") {
		t.Errorf("hidden, the instructions carry Ideas: %s", hello)
	}
	for _, name := range ideasTools {
		if slices.Contains(tools(call), name) {
			t.Errorf("hidden, yet %s is listed", name)
		}
	}

	enabled.Store(true)
	close(added)
	if method := next(); method != "notifications/tools/list_changed" {
		t.Fatalf("the server sent %q, want notifications/tools/list_changed", method)
	}
	for _, name := range ideasTools {
		if !slices.Contains(tools(call), name) {
			t.Errorf("shown, %s isn't listed", name)
		}
	}

	call, _ = session()
	if hello := string(call("initialize")); !strings.Contains(hello, "- Ideas: the Visionaries suggest features as ideas (list_ideas).") {
		t.Errorf("shown, the instructions don't carry Ideas: %s", hello)
	}
}
