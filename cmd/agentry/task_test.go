package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/runtime/desk"
)

// The desk's commands send what it wrote to the API with its token.
func TestTaskCommandsTalkToTheServer(t *testing.T) {
	var got []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer desk-secret" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		var body map[string]any
		json.NewDecoder(r.Body).Decode(&body)
		text := func(key string) string { value, _ := body[key].(string); return value }
		labels := ""
		if list, ok := body["labels"].([]any); ok && len(list) > 0 {
			labels = fmt.Sprint(list)
		}
		got = append(got, r.Method+" "+r.URL.Path+" "+text("title")+"|"+text("description")+"|"+text("repo")+labels+text("answer"))
		switch r.URL.Path {
		case "/api/tasks":
			if r.Method == "POST" {
				w.Write([]byte(`{"stream":"task-7"}`))
				return
			}
			w.Write([]byte(`[{"id":"task-7","title":"Add a retry","status":"needs you","needsYou":"Which queue?"}]`))
		default:
			w.Write([]byte(`{}`))
		}
	}))
	defer server.Close()
	t.Setenv(desk.EnvServer, server.URL)
	t.Setenv(desk.EnvToken, "desk-secret")

	var out, errs bytes.Buffer
	description := strings.NewReader("Retry once on a 503.\nDone when the test passes.\n")
	if code := runTask([]string{"create", "--title", "Add a retry", "--repo", "/src/app", "--label", "webhooks", "--label", "bug"}, description, &out, &errs); code != 0 {
		t.Fatalf("create: %d %s", code, errs.String())
	}
	if !strings.Contains(out.String(), "Created task-7") {
		t.Errorf("create printed %q", out.String())
	}
	out.Reset()
	if code := runTask([]string{"list"}, nil, &out, &errs); code != 0 || !strings.Contains(out.String(), "waiting for you: Which queue?") {
		t.Errorf("list: %d %q", code, out.String())
	}
	if code := runTask([]string{"answer", "task-7", "the", "billing", "queue"}, nil, &out, &errs); code != 0 {
		t.Errorf("answer: %d %s", code, errs.String())
	}

	want := []string{
		"POST /api/tasks Add a retry|Retry once on a 503.\nDone when the test passes.|/src/app[webhooks bug]",
		"GET /api/tasks ||",
		"POST /api/tasks/task-7/answer ||the billing queue",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("requests:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}

	t.Setenv(desk.EnvToken, "")
	if code := runTask([]string{"list"}, nil, &out, &errs); code != 2 {
		t.Errorf("without the token: %d, want 2", code)
	}
}
