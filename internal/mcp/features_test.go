package mcp

import (
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

	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/schema"
)

// votes is a feature with one tool and a line in the instructions. The
// server says one vote waits for you (votesAgentry).
func votes() features.Feature {
	return features.Feature{Name: "votes",
		Tools: func(agentry features.API, repo string) []features.Tool {
			return []features.Tool{{Name: "cast_vote", Description: "Vote.", Input: schema.Object(schema.Props{}),
				Run: func(context.Context, json.RawMessage) (string, error) { return "voted in " + repo, nil }}}
		},
		Instructions: "- Votes: cast them with cast_vote.",
	}
}

// votesAgentry is an Agentry whose votes feature is hidden until show is
// called. Its log is at position 1 until then, and 2 after: a wait for a
// change past 1 answers once show is called. One vote waits for you all
// along, in the repo asked about, as GET /api/waiting says.
func votesAgentry(t *testing.T) (agentry client.Client, show func()) {
	var visible atomic.Bool
	shown, done := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/features":
			fmt.Fprintf(w, `{"features":[{"name":"votes","enabled":false,"visible":%v}],"unknown":[]}`, visible.Load())
		case "/api/changes":
			switch r.URL.Query().Get("after") {
			case "0":
			case "1":
				select {
				case <-shown:
				case <-done:
				}
			default:
				<-done
			}
			fmt.Fprintf(w, `{"position":%d}`, map[bool]int{false: 1, true: 2}[visible.Load()])
		case "/api/tasks", "/api/questions":
			io.WriteString(w, `[]`)
		case "/api/waiting":
			fmt.Fprintf(w, `[{"key":"votes/1","kind":"votes","line":"1 vote to count in [%s] (cast_vote)"}]`, r.URL.Query().Get("repo"))
		default:
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(server.Close)
	t.Cleanup(func() { close(done) }) // before Close, which waits for the waits
	return client.Client{Where: client.Where{URL: server.URL}}, func() { visible.Store(true); close(shown) }
}

func toolNames(c *claude) []string {
	var listed struct{ Tools []struct{ Name string } }
	json.Unmarshal(c.call("tools/list", nil), &listed)
	var names []string
	for _, tool := range listed.Tools {
		names = append(names, tool.Name)
	}
	return names
}

// A hidden feature's tools aren't listed and can't be called, and its line
// isn't in the instructions. When the log moves and the feature shows,
// Claude Code is told the tools changed, and they are there.
func TestAFeatureBringsItsToolsWhenItShows(t *testing.T) {
	agentry, show := votesAgentry(t)
	c := startWith(t, agentry, []features.Feature{votes()}, false, nil)
	var hello struct {
		Capabilities struct{ Tools struct{ ListChanged bool } }
		Instructions string
	}
	json.Unmarshal(c.hello, &hello)
	if !hello.Capabilities.Tools.ListChanged {
		t.Errorf("initialize should declare tools.listChanged: %s", c.hello)
	}
	if strings.Contains(hello.Instructions, "Votes:") || !strings.HasSuffix(hello.Instructions, "\n- Every reply ends with what is waiting for the user. When something is, tell them in a line and offer to deal with it.") {
		t.Errorf("a hidden feature's instructions:\n%s", hello.Instructions)
	}
	if names := toolNames(c); slices.Contains(names, "cast_vote") || !slices.Contains(names, "status") {
		t.Errorf("hidden, tools: %v", names)
	}
	if text := c.tool("cast_vote", map[string]any{}); text != "error: no tool cast_vote" {
		t.Errorf("a hidden feature's tool: %s", text)
	}

	show()
	c.waitFor("notifications/tools/list_changed")
	if names := toolNames(c); !slices.Contains(names, "cast_vote") || !slices.Contains(names, "status") {
		t.Errorf("shown, tools: %v", names)
	}
	if text := c.tool("cast_vote", map[string]any{}); !strings.HasPrefix(text, "voted in /repo") {
		t.Errorf("a shown feature's tool: %s", text)
	}
	if len(c.notified) != 1 {
		t.Errorf("notified %v, want one list_changed", c.notified)
	}

	again := startWith(t, agentry, []features.Feature{votes()}, false, nil)
	if err := json.Unmarshal(again.hello, &hello); err != nil || !strings.Contains(hello.Instructions, "Votes: cast them with cast_vote.\n- Every reply ends") {
		t.Errorf("a shown feature's instructions:\n%s", hello.Instructions)
	}
}

// Every reply ends with what GET /api/waiting says waits for a session in
// its repo, and what waits for you outside a session is what it says for
// no repo.
func TestRepliesEndWithWhatTheServerSaysWaits(t *testing.T) {
	agentry, _ := votesAgentry(t)
	c := startWith(t, agentry, []features.Feature{votes()}, false, nil)
	if text := c.tool("list_tasks", map[string]any{}); !strings.HasSuffix(text, "Waiting for the user (1):\n- 1 vote to count in [/repo] (cast_vote)") {
		t.Errorf("reply: %s", text)
	}
	waiting, err := WaitingFor(agentry, "")
	if err != nil || len(waiting) != 1 || waiting[0].Kind != "votes" || waiting[0].Line != "1 vote to count in [] (cast_vote)" {
		t.Errorf("waiting: %+v, %v", waiting, err)
	}
}
