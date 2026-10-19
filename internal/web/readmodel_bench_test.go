package web

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/rules"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/store"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// TestReadModelBench measures, on a copy of a real agentry.db, how long the
// reads that replay the log take and where the time goes, for
// docs/design/read-model.md. It is skipped unless AGENTRY_BENCH_DB names the
// copy, which it copies again so the run never writes to it:
//
//	AGENTRY_BENCH_DB=$PWD/.agentry/files/bench/agentry.db \
//	  go test ./internal/web -run TestReadModelBench -v -count=1
func TestReadModelBench(t *testing.T) {
	source := os.Getenv("AGENTRY_BENCH_DB")
	if source == "" {
		t.Skip("AGENTRY_BENCH_DB is not set")
	}
	path := filepath.Join(t.TempDir(), "agentry.db")
	copyFile(t, source, path)
	s, state := storetest.Open(t, path)
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	defer raw.Close()

	ids, err := s.TaskIDs()
	if err != nil {
		t.Fatal(err)
	}
	biggest := biggestTask(t, raw)

	t.Log("whole requests (median of 5):")
	server := NewServer(s, Config{UIToken: token, Blobs: sharedfiles.Disk{Dir: t.TempDir()}}, http.NotFoundHandler(), nil).Handler()
	for _, url := range []string{"/api/tasks", "/api/questions", "/api/agents", "/api/tasks/" + biggest} {
		var size int
		took := median(func() { size = request(t, server, url) })
		t.Logf("  %-22s %8s  %8d bytes", url, took, size)
	}

	t.Log("where /api/tasks goes, every task stream (median of 5):")
	streams := append(slices.Clone(ids), events.AgentsStream)
	var rows, bytes int
	scan := median(func() { rows, bytes = scanStreams(t, raw, ids) })
	t.Logf("  SQLite scan, no decoding:    %8s  %d rows, %d bytes of data", scan, rows, bytes)
	read := median(func() { readStreams(t, s, ids) })
	t.Logf("  store.Stream (scan + JSON):  %8s", read)
	decoded := readStreams(t, s, ids)
	replay := median(func() {
		for _, stream := range decoded {
			tasks.FromEvents(stream)
		}
	})
	t.Logf("  tasks.FromEvents:            %8s", replay)
	built := map[string]*tasks.Task{}
	for id, stream := range decoded {
		built[id] = tasks.FromEvents(stream)
	}
	rosterEvents, _ := s.Stream(events.AgentsStream)
	roster := agents.FromEvents(rosterEvents)
	var summaries []taskSummary
	for _, task := range built {
		summaries = append(summaries, summaryOf(task, roster))
	}
	var encodedSize int
	encode := median(func() { encodedSize = encodedLen(t, summaries) })
	t.Logf("  summaries to JSON:           %8s  %d bytes", encode, encodedSize)

	t.Logf("the biggest task, %s:", biggest)
	stream := decoded[biggest]
	t.Logf("  store.Stream:                %8s  %d events", median(func() { s.Stream(biggest) }), len(stream))
	t.Logf("  tasks.FromEvents:            %8s", median(func() { tasks.FromEvents(stream) }))

	t.Log("the agents stream:")
	agentEvents, _ := s.Stream(events.AgentsStream)
	_, agentBytes := scanStreams(t, raw, []string{events.AgentsStream})
	t.Logf("  SQLite scan, no decoding:    %8s  %d bytes of data", median(func() { scanStreams(t, raw, []string{events.AgentsStream}) }), agentBytes)
	t.Logf("  store.Stream:                %8s  %d events", median(func() { s.Stream(events.AgentsStream) }), len(agentEvents))
	t.Logf("  agents.FromEvents:           %8s", median(func() { agents.FromEvents(agentEvents) }))

	t.Log("rules.Check, which every Propose runs under its lock:")
	proposal := events.Proposal{Stream: biggest, Actor: events.Actor{Kind: events.Agent, ID: "bench"}, Type: events.ActionResult,
		Correlation: "no-such-action", Data: map[string]any{}}
	t.Logf("  on %-25s %8s", biggest+":", median(func() { rules.Check(proposal, state) }))

	t.Log("what an in-memory projection would cost at startup:")
	var logged int
	all := median(func() { logged = readWholeLog(t, s) })
	t.Logf("  store.After over the log:    %8s  %d events", all, logged)
	t.Logf("  replay every stream:         %8s", median(func() {
		for _, stream := range decoded {
			tasks.FromEvents(stream)
		}
		agents.FromEvents(agentEvents)
	}))
	t.Logf("  heap held by every task and the roster: %d KB (%d streams)", heldBy(func() any {
		held := map[string]*tasks.Task{}
		for id, stream := range decoded {
			held[id] = tasks.FromEvents(stream)
		}
		return []any{held, agents.FromEvents(agentEvents)}
	})/1024, len(streams))
	runtime.KeepAlive(decoded)

	// The page refetches everything it shows on every event, and every
	// read and Propose shares the store's one connection. Each event here
	// also brings what the server does for one: the coordinator loop loads
	// the task and the roster, a hook call reads the task, and the watchdog
	// replays every task each second.
	t.Log("the tasks page while agents work: an event every 600 ms (the log's busiest 10 minutes), each refetching the page:")
	for _, background := range []bool{false, true} {
		page := []string{"/api/tasks", "/api/tasks", "/api/questions", "/api/agents", "/api/agents", "/api/config"}
		var mu sync.Mutex
		var waits []time.Duration
		var running sync.WaitGroup
		stop := make(chan struct{})
		if background {
			running.Go(func() { watchdogLoad(t, s, stop) })
		}
		for range 20 {
			running.Go(func() {
				s.Propose(proposal)
				if background {
					s.Stream(biggest)             // the hook reads the task
					s.Stream(biggest)             // the loop loads it,
					s.Stream(events.AgentsStream) // and the roster
				}
			})
			for _, url := range page {
				running.Go(func() {
					start := time.Now()
					request(t, server, url)
					if url == "/api/tasks" {
						mu.Lock()
						waits = append(waits, time.Since(start))
						mu.Unlock()
					}
				})
			}
			time.Sleep(600 * time.Millisecond)
		}
		close(stop)
		running.Wait()
		slices.Sort(waits)
		label := "page and Propose only"
		if background {
			label = "with loop, hook and watchdog reads"
		}
		t.Logf("  %-36s /api/tasks median %s, slowest %s over %d requests", label+":",
			waits[len(waits)/2].Round(time.Millisecond), waits[len(waits)-1].Round(time.Millisecond), len(waits))
	}
}

// watchdogLoad reads what watchdog.Check reads, once a second, until stop.
func watchdogLoad(t *testing.T, s *store.Store, stop chan struct{}) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case <-ticker.C:
			ids, _ := s.TaskIDs()
			for _, stream := range readStreams(t, s, ids) {
				tasks.FromEvents(stream)
			}
		}
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	content, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, content, 0o600); err != nil {
		t.Fatal(err)
	}
}

func biggestTask(t *testing.T, raw *sql.DB) string {
	t.Helper()
	var id string
	err := raw.QueryRow(`SELECT stream FROM events WHERE stream LIKE 'task-%'
		GROUP BY stream ORDER BY count(*) DESC LIMIT 1`).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func median(run func()) time.Duration {
	var took []time.Duration
	for range 5 {
		start := time.Now()
		run()
		took = append(took, time.Since(start))
	}
	slices.Sort(took)
	return took[len(took)/2].Round(100 * time.Microsecond)
}

func request(t *testing.T, server http.Handler, url string) int {
	t.Helper()
	r := httptest.NewRequest(http.MethodGet, url, nil)
	r.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("%s: %d %s", url, w.Code, w.Body.String())
	}
	n, _ := io.Copy(io.Discard, w.Body)
	return int(n)
}

// scanStreams reads the rows Store.Stream reads, keeping the data as text.
func scanStreams(t *testing.T, raw *sql.DB, ids []string) (rows, bytes int) {
	t.Helper()
	for _, id := range ids {
		found, err := raw.Query(`SELECT position, at, stream, actor_kind, actor_id, type, correlation, data
			FROM events WHERE stream = ? AND type != ? ORDER BY position`, id, events.ProposalRejected)
		if err != nil {
			t.Fatal(err)
		}
		for found.Next() {
			var position int64
			var at, stream, kind, actor, kindOf, correlation, data string
			if err := found.Scan(&position, &at, &stream, &kind, &actor, &kindOf, &correlation, &data); err != nil {
				t.Fatal(err)
			}
			rows, bytes = rows+1, bytes+len(data)
		}
		if err := found.Err(); err != nil {
			t.Fatal(err)
		}
		found.Close()
	}
	return rows, bytes
}

func readStreams(t *testing.T, s *store.Store, ids []string) map[string][]events.Event {
	t.Helper()
	read := map[string][]events.Event{}
	for _, id := range ids {
		stream, err := s.Stream(id)
		if err != nil {
			t.Fatal(err)
		}
		read[id] = stream
	}
	return read
}

func readWholeLog(t *testing.T, s *store.Store) int {
	t.Helper()
	var position int64
	count := 0
	for {
		found, err := s.After(position, 500)
		if err != nil {
			t.Fatal(err)
		}
		if len(found) == 0 {
			return count
		}
		count, position = count+len(found), found[len(found)-1].Position
	}
}

func encodedLen(t *testing.T, value any) int {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return len(encoded)
}

// heldBy is how much heap what build returns keeps alive: the heap with it,
// less the heap once it is dropped.
func heldBy(build func() any) uint64 {
	var with, without runtime.MemStats
	kept := build()
	runtime.GC()
	runtime.ReadMemStats(&with)
	runtime.KeepAlive(kept)
	kept = nil
	runtime.GC()
	runtime.ReadMemStats(&without)
	return with.HeapAlloc - without.HeapAlloc
}
