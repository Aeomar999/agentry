package web

import (
	"cmp"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
)

// TestTaskEventsBench measures, on a copy of a real agentry.db, a task's
// detail and what makes it big, its events read a page at a time, all of
// them and the chat's types alone, and which events and which of their
// fields make them big (docs/design/read-model.md section 5 step 7). It is skipped unless
// AGENTRY_BENCH_DB names the copy, as for TestReadModelBench; AGENTRY_BENCH_TASKS
// names the tasks to measure, task-8 and the biggest other one by default.
func TestTaskEventsBench(t *testing.T) {
	source := os.Getenv("AGENTRY_BENCH_DB")
	if source == "" {
		t.Skip("AGENTRY_BENCH_DB is not set")
	}
	path := filepath.Join(t.TempDir(), "agentry.db")
	copyFile(t, source, path)
	s, state := storetest.OpenWith(t, path, storetest.WithIdeas())
	server := NewServer(s, state, Config{UIToken: token, Blobs: sharedfiles.Disk{Dir: t.TempDir()}}, http.NotFoundHandler(), nil).Handler()

	ids := []string{"task-8"}
	if named := os.Getenv("AGENTRY_BENCH_TASKS"); named != "" {
		ids = nil
		if err := json.Unmarshal([]byte(named), &ids); err != nil {
			t.Fatalf("AGENTRY_BENCH_TASKS is a JSON list of task ids: %v", err)
		}
	} else {
		raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
		if err != nil {
			t.Fatal(err)
		}
		defer raw.Close()
		ids = append(ids, biggestTask(t, raw, ids))
	}
	chat := chatTypes(t)
	for _, id := range ids {
		var body []byte
		took := median(func() { body = detailBody(t, server, "/api/tasks/"+id) })
		var parts map[string]json.RawMessage
		if err := json.Unmarshal(body, &parts); err != nil {
			t.Fatal(err)
		}
		partSizes := map[string]int{}
		for name, raw := range parts {
			partSizes[name] = len(raw)
		}
		var largest []string
		for _, name := range largestFirst(partSizes)[:min(6, len(partSizes))] {
			largest = append(largest, fmt.Sprintf("%s %d", name, partSizes[name]))
		}
		t.Logf("%s: detail %d bytes in %s (median of 5); its largest parts: %v", id, len(body), took, largest)

		var all eventWalk
		took = median(func() { all = walkEvents(t, server, id, "") })
		t.Logf("  every event, from the start: %d events in %d pages, %d bytes, %s", len(all.events), all.pages, all.bytes, took)
		var chatWalk eventWalk
		took = median(func() { chatWalk = walkEvents(t, server, id, chat) })
		t.Logf("  the chat's types, from the start: %d events in %d pages, %d bytes, %s", len(chatWalk.events), chatWalk.pages, chatWalk.bytes, took)
		newest := "/api/tasks/" + id + "/events?before=" + fmt.Sprint(s.LastPosition()+1)
		var size int
		took = median(func() { size = len(detailBody(t, server, newest)) })
		t.Logf("  the newest page of %d events: %d bytes, %s", eventPage, size, took)

		var entries []eventEntry
		for _, raw := range all.events {
			var e eventEntry
			if err := json.Unmarshal(raw, &e); err != nil {
				t.Fatal(err)
			}
			entries = append(entries, e)
		}
		byType := map[string]int{}
		countOf := map[string]int{}
		for i, e := range entries {
			byType[string(e.Type)] += len(all.events[i])
			countOf[string(e.Type)]++
		}
		types := largestFirst(byType)
		for _, kind := range types {
			t.Logf("  %-24s %6d events %10d bytes", kind, countOf[kind], byType[kind])
		}
		for _, kind := range types[:min(3, len(types))] {
			byField := map[string]int{}
			for _, e := range entries {
				if string(e.Type) != kind {
					continue
				}
				for field, value := range e.Data {
					encoded, _ := json.Marshal(value)
					byField[field] += len(encoded)
				}
			}
			var fields []string
			for _, field := range largestFirst(byField) {
				fields = append(fields, fmt.Sprintf("%s %d", field, byField[field]))
			}
			t.Logf("  %s data by field: %v", kind, fields)
		}
	}
}

type eventWalk struct {
	events []json.RawMessage
	bytes  int // of the pages, as sent
	pages  int
}

// walkEvents reads every page of a task's events from the start, as the
// chat does; query keeps them to some types.
func walkEvents(t *testing.T, server http.Handler, id, query string) eventWalk {
	t.Helper()
	var walk eventWalk
	for after, more := int64(0), true; more; walk.pages++ {
		body := detailBody(t, server, fmt.Sprintf("/api/tasks/%s/events?after=%d%s", id, after, query))
		var page struct {
			Events []json.RawMessage `json:"events"`
			More   bool              `json:"more"`
		}
		if err := json.Unmarshal(body, &page); err != nil {
			t.Fatal(err)
		}
		walk.bytes += len(body)
		walk.events = append(walk.events, page.Events...)
		more = page.More
		if len(page.Events) > 0 {
			var last eventEntry
			if err := json.Unmarshal(page.Events[len(page.Events)-1], &last); err != nil {
				t.Fatal(err)
			}
			after = last.Position
		}
	}
	return walk
}

// chatTypes is the ?type= query of the types the chat reads, from the
// page's own list of them (CHAT_TYPES in ui/src/lib/chat.ts).
func chatTypes(t *testing.T) string {
	t.Helper()
	source, err := os.ReadFile(filepath.Join("..", "..", "ui", "src", "lib", "chat.ts"))
	if err != nil {
		t.Fatal(err)
	}
	list := regexp.MustCompile(`(?s)export const CHAT_TYPES = \[(.*?)\]`).FindSubmatch(source)
	if list == nil {
		t.Fatal("no CHAT_TYPES in chat.ts")
	}
	query := ""
	for _, name := range regexp.MustCompile(`"([^"]+)"`).FindAllSubmatch(list[1], -1) {
		query += "&type=" + string(name[1])
	}
	return query
}

func detailBody(t *testing.T, server http.Handler, url string) []byte {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, url, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("%s: %d %s", url, w.Code, w.Body.String())
	}
	return w.Body.Bytes()
}

// largestFirst is a map's keys, the largest value first.
func largestFirst(sizes map[string]int) []string {
	var keys []string
	for key := range sizes {
		keys = append(keys, key)
	}
	slices.SortFunc(keys, func(a, b string) int { return cmp.Or(sizes[b]-sizes[a], cmp.Compare(a, b)) })
	return keys
}
