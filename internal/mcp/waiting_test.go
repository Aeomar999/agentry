package mcp

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/platform/client"
)

// The news a waiter wakes Claude with: only what is new since it started,
// of each kind, with an epic named epic-N.
func TestTheNewsIsWhatIsNew(t *testing.T) {
	var mu sync.Mutex
	board := map[string]any{
		"/api/tasks":        []map[string]any{{"id": "task-1", "title": "Old", "status": "done"}},
		"/api/waiting":      []any{},
		"/api/workers":      map[string]any{"workers": []map[string]any{{"name": "build-box", "online": true, "tasks": []string{"task-2"}}}},
		"/api/tasks/task-2": map[string]any{"summary": "The game runs."},
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/changes" {
			time.Sleep(50 * time.Millisecond)
			json.NewEncoder(w).Encode(map[string]int{"position": 1})
			return
		}
		mu.Lock()
		defer mu.Unlock()
		json.NewEncoder(w).Encode(board[r.URL.Path])
	}))
	defer server.Close()
	agentry := client.Client{Where: client.Where{URL: server.URL}, Token: "x"}

	now, err := News(agentry)
	if err != nil {
		t.Fatal(err)
	}
	result := make(chan []Waiting, 1)
	go func() {
		fresh, _ := WaitForNew(context.Background(), agentry, Seen(now))
		result <- fresh
	}()
	time.Sleep(150 * time.Millisecond)
	mu.Lock()
	board["/api/tasks"] = []map[string]any{
		{"id": "task-1", "title": "Old", "status": "done"},
		{"id": "task-2", "title": "Snake", "status": "done", "kind": "epic", "told": map[string]string{"at": "2026-10-17T21:00:00Z", "from": "tech-lead", "text": "Moved it to the desktop."}},
	}
	board["/api/workers"] = map[string]any{"workers": []map[string]any{{"name": "build-box", "online": false, "since": "t", "tasks": []string{"task-2"}}}}
	mu.Unlock()

	var lines []string
	select {
	case fresh := <-result:
		for _, w := range fresh {
			lines = append(lines, w.Kind+": "+w.Line)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no news")
	}
	got := strings.Join(lines, "\n")
	for _, want := range []string{"done: epic-2 is done: Snake. The game runs.", "said: epic-2 (Snake): tech-lead says: Moved it to the desktop.",
		"offline: the machine build-box went offline; epic-2 waits until it is back"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in:\n%s", want, got)
		}
	}
	if strings.Contains(got, "task-1") {
		t.Errorf("task-1 was done before the waiter started:\n%s", got)
	}
}
