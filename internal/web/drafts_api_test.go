package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

const helperToken = "helper-token-for-tests"

func TestYourClaudeOnlyDraftsAndYouSend(t *testing.T) {
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), rules.Check)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	server := httptest.NewServer(NewServer(s, Config{UIToken: token, HelperToken: helperToken, DefaultRepo: "/src/app"}, http.NotFoundHandler(), nil).Handler())
	t.Cleanup(server.Close)
	call(t, server, "POST", "/api/tasks", `{"title":"Add a cache"}`)
	asHelper := func(method, path, body string) int {
		request, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+helperToken)
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

	if code := asHelper("POST", "/api/tasks/task-1/drafts", `{"text":"Use Redis for the cache."}`); code != 200 {
		t.Fatalf("drafting: %d", code)
	}
	// It can't send its own draft, or talk to the coordinator, or do anything else.
	for _, path := range []string{"/api/tasks/task-1/drafts/draft-1/send", "/api/tasks/task-1/drafts/draft-1/discard", "/api/tasks/task-1/message"} {
		if code := asHelper("POST", path, `{"text":"go"}`); code != http.StatusUnauthorized {
			t.Errorf("%s with the helper's token: %d", path, code)
		}
	}
	if code := asHelper("GET", "/api/tasks", ""); code != http.StatusUnauthorized {
		t.Errorf("listing tasks with the helper's token: %d", code)
	}
	if steps := len(taskNow().Trail); steps != 1 {
		t.Errorf("a draft reached the coordinator: %d steps", steps)
	}

	// You send it, changed.
	if code, body := call(t, server, "POST", "/api/tasks/task-1/drafts/draft-1/send", `{"text":"Use Redis for the cache, with a 5 minute TTL."}`); code != 200 {
		t.Fatalf("sending: %d %s", code, body)
	}
	task := taskNow()
	if last := task.Trail[len(task.Trail)-1]; last.From != tasks.You || !strings.Contains(last.Text, "5 minute TTL") || !task.Draft("draft-1").Sent {
		t.Errorf("your words, from the draft: %+v, draft %+v", last, task.Draft("draft-1"))
	}
	if code, _ := call(t, server, "POST", "/api/tasks/task-1/drafts/draft-1/send", `{"text":"again"}`); code != http.StatusConflict {
		t.Errorf("sending a draft twice: %d", code)
	}

	// Or you discard one.
	asHelper("POST", "/api/tasks/task-1/drafts", `{"text":"Rewrite it in Rust."}`)
	if code, _ := call(t, server, "POST", "/api/tasks/task-1/drafts/draft-2/discard", ""); code != 200 {
		t.Errorf("discarding: %d", code)
	}
	if code, _ := call(t, server, "POST", "/api/tasks/task-1/drafts/draft-2/send", `{"text":"Rewrite it in Rust."}`); code != http.StatusConflict {
		t.Errorf("sending a discarded draft: %d", code)
	}
}
