package mcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/client"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/schema"
)

// votes is a feature with one tool, a line in the instructions and one
// thing waiting for you.
func votes() features.Feature {
	return features.Feature{Name: "votes",
		Tools: func(agentry features.API, repo string) []features.Tool {
			return []features.Tool{{Name: "cast_vote", Description: "Vote.", Input: schema.Object(schema.Props{}),
				Run: func(context.Context, json.RawMessage) (string, error) { return "voted in " + repo, nil }}}
		},
		Instructions: "- Votes: cast them with cast_vote.",
		Waiting: func(agentry features.API, repo string) []features.Waiting {
			return []features.Waiting{{Key: "votes/1", Kind: "votes", Line: "1 vote to count (cast_vote)"}}
		},
	}
}

// votesAgentry is an Agentry whose votes feature is hidden.
func votesAgentry(t *testing.T) client.Client {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/features":
			io.WriteString(w, `{"features":[{"name":"votes","enabled":false,"visible":false}],"unknown":[]}`)
		case "/api/tasks", "/api/questions":
			io.WriteString(w, `[]`)
		default:
			io.WriteString(w, `{}`)
		}
	}))
	t.Cleanup(server.Close)
	return client.Client{Where: client.Where{URL: server.URL}}
}

// Every reply ends with what each feature says waits for you, and so does
// what waits for you outside a session.
func TestRepliesEndWithWhatFeaturesSayWaits(t *testing.T) {
	agentry := votesAgentry(t)
	c := startWith(t, agentry, []features.Feature{votes()}, false, nil)
	if text := c.tool("list_tasks", map[string]any{}); !strings.HasSuffix(text, "Waiting for the user (1):\n- 1 vote to count (cast_vote)") {
		t.Errorf("reply: %s", text)
	}
	waiting, err := WaitingFor(agentry, []features.Feature{votes()})
	if err != nil || len(waiting) != 1 || waiting[0].Kind != "votes" {
		t.Errorf("waiting: %+v, %v", waiting, err)
	}
}
