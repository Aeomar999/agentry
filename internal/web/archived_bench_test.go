package web

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// Target 7 of docs/design/log-growth.md: an archived task's history opens
// in at most coldRead for the largest task, and in at most warmRead from
// the cache of the last few opened.
const (
	coldRead = 100 * time.Millisecond
	warmRead = 10 * time.Millisecond
)

// TestArchivedTaskOpensBench measures Target 7 on a copy of a real
// agentry.db: it archives the copy's largest task by rows (task-8 on the
// real log), then times GET /api/tasks/{id} cold, gunzipping its blob and
// folding its events, and warm, from the cache. It is skipped unless
// AGENTRY_BENCH_DB names the copy, which it copies again so the run never
// writes to it:
//
//	AGENTRY_BENCH_DB=$PWD/.agentry/files/bench/agentry.db \
//	  go test ./internal/web -run TestArchivedTaskOpensBench -v -count=1
func TestArchivedTaskOpensBench(t *testing.T) {
	source := os.Getenv("AGENTRY_BENCH_DB")
	if source == "" {
		t.Skip("AGENTRY_BENCH_DB is not set")
	}
	path := filepath.Join(t.TempDir(), "agentry.db")
	copyFile(t, source, path)
	raw, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		t.Fatal(err)
	}
	id := biggestTask(t, raw, nil)
	var rows int
	if err := raw.QueryRow(`SELECT count(*) FROM events WHERE stream = ?`, id).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	s, state := storetest.OpenWith(t, path, storetest.WithExtensions())
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	app := NewServer(s, state, Config{UIToken: token, Blobs: blobs}, http.NotFoundHandler(), nil)
	storetest.ArchiveIn(t, s, state, id, blobs)
	cold, warm := openArchived(t, app, id)
	t.Logf("%s, %d rows archived: cold %s (%s), warm %s (%s)", id, rows, cold, verdict(cold, coldRead), warm, verdict(warm, warmRead))
}

// A stand-in for Target 7 that always runs: a task of about 4,000 events,
// half as many as the real log's largest (task-8, 7,726), archived, opens
// within the target cold and warm. Half, so the check has room on a busy
// machine: at task-8's size this one takes some 35 ms cold.
func TestAnArchivedTaskOpensInTime(t *testing.T) {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	app := NewServer(s, state, Config{UIToken: token, DefaultRepo: "/r", Blobs: blobs}, http.NotFoundHandler(), nil)
	server := httptest.NewServer(app.Handler())
	defer server.Close()
	if code, body := call(t, server, "POST", "/api/tasks", `{"title":"A long one"}`); code != 200 {
		t.Fatalf("creating the task: %d %s", code, body)
	}
	made := longTask(t, s, "task-1", 125, 10)
	storetest.ArchiveIn(t, s, state, "task-1", blobs)

	cold, warm := openArchived(t, app, "task-1")
	t.Logf("task-1, %d events: cold %s, warm %s", made, cold, warm)
	if limit := coldRead * raceSlowdown; cold > limit {
		t.Errorf("a cold read took %s, want at most %s", cold, limit)
	}
	if limit := warmRead * raceSlowdown; warm > limit {
		t.Errorf("a warm read took %s, want at most %s", warm, limit)
	}
}

// longTask gives a task turns of the agent at work, each with actions
// requested, approved and run, and a report, as a long task's log has
// them. It says how many events it wrote.
func longTask(t *testing.T, s *store.Store, id string, turns, actions int) int {
	t.Helper()
	worker := events.Actor{Kind: events.Worker, ID: workers.Local}
	policy := events.Actor{Kind: events.Policy, ID: "policy"}
	made := 0
	propose := func(p events.Proposal) {
		t.Helper()
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatalf("%s on %s: %v %v", p.Type, p.Stream, err, rejection)
		}
		made++
	}
	propose(events.Proposal{Stream: id, Actor: worker, Type: events.WorktreeReady, Data: map[string]any{"path": "/w/" + id, "branch": "agentry/" + id, "port": 7901}})
	for turn := range turns {
		correlation := fmt.Sprintf("turn-%d", turn+1)
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.TurnStarted, Correlation: correlation, Data: map[string]any{"agent": "tech-lead", "hash": "h"}})
		for action := range actions {
			call := fmt.Sprintf("call-%d-%d", turn, action)
			command := fmt.Sprintf("cd /w/%s && go test ./internal/web -run Test%d -count=1 %s", id, action, strings.Repeat("-v ", 60))
			propose(events.Proposal{Stream: id, Actor: techLead, Type: events.ActionRequested, Correlation: call, Data: map[string]any{"tool": "Bash", "command": command}})
			propose(events.Proposal{Stream: id, Actor: policy, Type: events.ActionApproved, Correlation: call, Data: map[string]any{"reason": "a test run"}})
			propose(events.Proposal{Stream: id, Actor: worker, Type: events.ActionResult, Correlation: call})
		}
		report := strings.Repeat("Changed the handler and its test; both pass. ", 40)
		propose(events.Proposal{Stream: id, Actor: worker, Type: events.TurnEnded, Correlation: correlation, Data: map[string]any{"agent": "tech-lead", "report": report, "tokens": 1200}})
	}
	return made
}

// openArchived times GET /api/tasks/{id} for an archived task: cold, with
// the cache of opened tasks emptied before each read, and warm, from it.
// Each is the median of 5.
func openArchived(t *testing.T, app *Server, id string) (cold, warm time.Duration) {
	t.Helper()
	server := app.Handler()
	url := "/api/tasks/" + id
	cold = median(func() {
		app.archives.mu.Lock()
		app.archives.recent = nil
		app.archives.mu.Unlock()
		if body := detailBody(t, server, url); !strings.Contains(string(body), `"archived":true`) {
			t.Fatalf("%s isn't read as archived: %.200s", id, body)
		}
	})
	warm = median(func() { detailBody(t, server, url) })
	return cold, warm
}

func verdict(took, limit time.Duration) string {
	if took <= limit {
		return fmt.Sprintf("PASS, at most %s", limit)
	}
	return fmt.Sprintf("FAIL, over %s", limit)
}
