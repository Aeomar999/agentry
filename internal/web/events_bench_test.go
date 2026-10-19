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
	"slices"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
)

// TestTaskEventsBench measures, on a copy of a real agentry.db, how much of
// a task's detail is its events, and which events and which of their fields
// make it big, for the step that moves the events out of the detail
// (docs/design/read-model.md section 5 step 7). It is skipped unless
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
	for _, id := range ids {
		var body []byte
		took := median(func() { body = detailBody(t, server, "/api/tasks/"+id) })
		var detail struct {
			Events []json.RawMessage `json:"events"`
		}
		if err := json.Unmarshal(body, &detail); err != nil {
			t.Fatal(err)
		}
		var entries []eventEntry
		eventBytes := 0
		for _, raw := range detail.Events {
			var e eventEntry
			if err := json.Unmarshal(raw, &e); err != nil {
				t.Fatal(err)
			}
			entries = append(entries, e)
			eventBytes += len(raw)
		}
		t.Logf("%s: detail %d bytes in %s (median of 5); %d events, %d bytes of them",
			id, len(body), took, len(entries), eventBytes)
		var parts map[string]json.RawMessage
		if err := json.Unmarshal(body, &parts); err != nil {
			t.Fatal(err)
		}
		partSizes := map[string]int{}
		for name, raw := range parts {
			partSizes[name] = len(raw)
		}
		var largest []string
		for _, name := range largestFirst(partSizes)[:min(5, len(partSizes))] {
			largest = append(largest, fmt.Sprintf("%s %d", name, partSizes[name]))
		}
		t.Logf("  the detail's largest parts: %v", largest)
		for _, query := range []string{"?after=0", "?before=" + fmt.Sprint(s.LastPosition()+1)} {
			url := "/api/tasks/" + id + "/events" + query
			var size int
			took := median(func() { size = len(detailBody(t, server, url)) })
			t.Logf("  a page of %d events, %-18s %8s  %8d bytes", eventPage, query+":", took, size)
		}

		byType := map[string]int{}
		countOf := map[string]int{}
		for i, e := range entries {
			byType[string(e.Type)] += len(detail.Events[i])
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
