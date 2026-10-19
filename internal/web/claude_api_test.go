package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

const claudeToken = "workspace-claude-token-for-tests"

func TestClaudeSendsWhatYouAskedAndNothingElse(t *testing.T) {
	s, _ := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(NewServer(s, Config{UIToken: token, HelperToken: claudeToken, DefaultRepo: "/src/app"}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	call(t, server, "POST", "/api/tasks", `{"title":"Add a cache"}`)
	asClaude := func(method, path, body string) int {
		request, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+claudeToken)
		response, err := http.DefaultClient.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		io.Copy(io.Discard, response.Body)
		response.Body.Close()
		return response.StatusCode
	}
	taskNow := func() *tasks.Task {
		stream, _ := s.Stream("task-1")
		return tasks.FromEvents(stream)
	}

	if code := asClaude("POST", "/api/tasks/task-1/claude/message", `{"text":"Use Redis for the cache."}`); code != 200 {
		t.Fatalf("sending: %d", code)
	}
	task := taskNow()
	if last := task.Trail[len(task.Trail)-1]; last.From != tasks.You || last.Text != "Use Redis for the cache." {
		t.Errorf("it goes to the coordinator as yours: %+v", last)
	}
	stream, _ := s.Stream("task-1")
	if e := stream[len(stream)-1]; e.Type != events.TaskDirected || e.Text("via") != "claude" {
		t.Errorf("marked as sent through Claude: %+v", e)
	}

	// Its token does nothing else, and the page's token doesn't send as Claude.
	for _, path := range []string{"/api/tasks/task-1/message", "/api/tasks/task-1/answer", "/api/tasks"} {
		if code := asClaude("POST", path, `{"text":"go","answer":"go","title":"x"}`); code != http.StatusUnauthorized {
			t.Errorf("%s with Claude's token: %d", path, code)
		}
	}
	if code, _ := call(t, server, "POST", "/api/tasks/task-1/claude/message", `{"text":"go"}`); code != http.StatusUnauthorized {
		t.Errorf("the page's token on Claude's route: %d", code)
	}
}
