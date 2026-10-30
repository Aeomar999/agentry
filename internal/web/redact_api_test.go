package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/platform/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// Redacting a secret through the page (docs/design/log-growth.md section
// 5, "Removing a secret"). The store's side is in store's redact tests;
// these see what the routes add: the count first, the archive cache, the
// snapshot, and who may ask.

// secretHere is a secret pasted into task-4, which nothing else holds.
const secretHere = "sk-live-0c9e44d1"

// withSecret adds task-4 to an erase fixture, holding secretHere in its
// description and in the name of a file it shares: two rows.
func withSecret(t *testing.T) *eraseFixture {
	t.Helper()
	f := newEraseFixture(t)
	f.propose(t,
		events.Proposal{Stream: "task-4", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Deploy",
			"description": "Use the key " + secretHere + " to deploy.", "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-4", Actor: you, Type: events.FileAdded, Data: map[string]any{"name": secretHere + ".md", "hash": f.both, "size": 21, "by": "you"}},
	)
	return f
}

// preview counts the rows of path that hold text, through the route.
func (f *eraseFixture) preview(t *testing.T, path, text string) int {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text})
	code, answer := call(t, f.server, "POST", path+"/preview", string(body))
	if code != http.StatusOK {
		t.Fatalf("previewing %s: %d %s", path, code, answer)
	}
	var count struct{ Rows int }
	if err := json.Unmarshal([]byte(answer), &count); err != nil {
		t.Fatal(err)
	}
	return count.Rows
}

// redactAt redacts text through the route at path, and is its answer.
func (f *eraseFixture) redactAt(t *testing.T, path, text string) redacted {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text, "why": "Pasted by mistake."})
	code, answer := call(t, f.server, "POST", path, string(body))
	if code != http.StatusOK {
		t.Fatalf("redacting %s: %d %s", path, code, answer)
	}
	var got redacted
	if err := json.Unmarshal([]byte(answer), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

// holdsSecret fails the test unless the files in want, with the snapshot
// written now, hold the secret: the guard that the test searches the
// right bytes.
func (f *eraseFixture) holdsSecret(t *testing.T, want ...string) {
	t.Helper()
	if err := f.snapshots.WriteNow(); err != nil {
		t.Fatal(err)
	}
	holding := storetest.HoldingAny(t, f.dir, secretHere)
	for _, file := range want {
		if !slices.Contains(holding, file) {
			t.Fatalf("before the redaction %s should hold the secret; these do: %v", file, holding)
		}
	}
}

// redactedFromDisk fails the test unless the answer counts the rows the
// preview did, warns of nothing but want, and no file in the folder holds
// the secret: no row, WAL, snapshot or blob.
func (f *eraseFixture) redactedFromDisk(t *testing.T, answer redacted, stream string, previewed int, want ...string) {
	t.Helper()
	if answer.Stream != stream || answer.Rows != previewed || previewed == 0 || time.Since(answer.At) > time.Minute {
		t.Errorf("the answer %+v, want %s with the %d rows previewed", answer, stream, previewed)
	}
	if !slices.Equal(answer.Warnings, want) {
		t.Errorf("the redaction warned %q, want %q", answer.Warnings, want)
	}
	if holding := storetest.HoldingAny(t, f.dir, secretHere); len(holding) != 0 {
		t.Errorf("after the redaction these files still hold the secret: %v", holding)
	}
}

// The step's proof for a live task: the preview counts the rows that hold
// the secret, the redaction rewrites as many, and then no file holds it.
// The rest of the task is kept.
func TestRedactingALiveTaskLeavesNoTraceOnDisk(t *testing.T) {
	f := withSecret(t)
	f.holdsSecret(t, "state.snapshot")
	rows := f.preview(t, "/api/tasks/task-4/redact", secretHere)
	f.redactedFromDisk(t, f.redactAt(t, "/api/tasks/task-4/redact", secretHere), "task-4", rows)
	task, err := f.state.Task("task-4")
	if err != nil || task == nil {
		t.Fatalf("task-4 after the redaction: %v, %v", task, err)
	}
	if task.Description != "Use the key [redacted] to deploy." {
		t.Errorf("task-4's description: %q", task.Description)
	}
}

// The same for an archived task, opened from the page first, whose rows
// are in its blob, not the snapshot: it is archived again, its old blob
// is gone, and the cache of opened archives, whose entry held the secret,
// drops it.
func TestRedactingAnArchivedTaskLeavesNoTraceOnDisk(t *testing.T) {
	f := withSecret(t)
	f.propose(t, events.Proposal{Stream: "task-4", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "Deployed.", "moved": true}})
	storetest.ArchiveIn(t, f.log, f.state, "task-4", f.blobs)
	old := markerOf(t, f, "task-4")
	if code, body := call(t, f.server, "GET", "/api/tasks/task-4", ""); code != http.StatusOK || !slices.Equal(f.opened(), []string{"task-4"}) {
		t.Fatalf("opening task-4: %d %s, opened %q", code, body, f.opened())
	}
	f.holdsSecret(t, old)
	rows := f.preview(t, "/api/tasks/task-4/redact", secretHere)
	f.redactedFromDisk(t, f.redactAt(t, "/api/tasks/task-4/redact", secretHere), "task-4", rows)
	if f.blobs.Has(old) {
		t.Errorf("the old archive blob %s is still there", old)
	}
	if _, err := f.state.Task("task-4"); !errors.Is(err, current.ErrArchived) {
		t.Errorf("task-4, redacted, should be archived again: %v", err)
	}
	if got := f.opened(); slices.Contains(got, "task-4") {
		t.Errorf("opened archives after the redaction %q, want no task-4", got)
	}
}

// teach has the engineer know each version of knowledge in turn, through
// the page.
func (f *eraseFixture) teach(t *testing.T, knowledge ...string) {
	t.Helper()
	for _, text := range knowledge {
		body, _ := json.Marshal(map[string]string{"knowledge": text})
		if code, answer := call(t, f.server, "PUT", "/api/agents/engineer/knowledge", string(body)); code != http.StatusOK {
			t.Fatalf("teaching the engineer: %d %s", code, answer)
		}
	}
}

// A secret in an agent's knowledge is redacted from the agents' stream,
// every version that holds it. With no turn running, nothing can write it
// back, so the answer warns of nothing.
func TestRedactingTheAgentsKnowledgeLeavesNoTraceOnDisk(t *testing.T) {
	f := newEraseFixture(t)
	f.teach(t, "# Notes\n\nThe key is "+secretHere+".", "# Notes\n\nThe key is "+secretHere+".\n\nDeploy on Fridays.")
	f.holdsSecret(t, "state.snapshot")
	rows := f.preview(t, "/api/agents/redact", secretHere)
	f.redactedFromDisk(t, f.redactAt(t, "/api/agents/redact", secretHere), events.AgentsStream, rows)
	engineer, _ := f.state.Roster().Get("engineer")
	if want := "# Notes\n\nThe key is [redacted].\n\nDeploy on Fridays."; engineer.Knowledge != want {
		t.Errorf("the engineer's knowledge: %q, want %q", engineer.Knowledge, want)
	}
}

// The engineer's turn starts, with its knowledge file holding the secret,
// and you redact the secret while it runs: the answer says the turn's end
// will redact it again, and it does. The turn ends with the file as it
// started plus a line the engineer learned; the newest version keeps the
// line, and no file holds the secret, nor does the server remember it.
// storetest's cleanup checks the agents' stream replays to the state.
func TestATurnRunningAsTheAgentsAreRedactedDoesNotWriteTheSecretBack(t *testing.T) {
	f := newEraseFixture(t)
	written := "# Notes\n\nThe key is " + secretHere + "."
	f.teach(t, written)
	f.startTurn(t)
	rows := f.preview(t, "/api/agents/redact", secretHere)
	answer := f.redactAt(t, "/api/agents/redact", secretHere)
	if !slices.Equal(answer.Warnings, []string{stillRunning}) {
		t.Errorf("the redaction warned %q, want only that a running turn's end redacts it again", answer.Warnings)
	}

	f.endTurn(t, written+"\n\nDeploy on Fridays.\n")

	engineer, _ := f.state.Roster().Get("engineer")
	if want := "# Notes\n\nThe key is [redacted].\n\nDeploy on Fridays."; engineer.Knowledge != want {
		t.Errorf("the engineer's knowledge after the turn: %q, want %q", engineer.Knowledge, want)
	}
	if err := f.snapshots.WriteNow(); err != nil {
		t.Fatal(err)
	}
	f.redactedFromDisk(t, answer, events.AgentsStream, rows, stillRunning)
	if f.hooks.Redactions().Holds(secretHere) {
		t.Error("the secret is still held after the only turn running as it was redacted ended")
	}
}

// startTurn starts the engineer's turn-1 in task-1, whose knowledge file
// is written as it starts.
func (f *eraseFixture) startTurn(t *testing.T) {
	t.Helper()
	f.propose(t,
		events.Proposal{Stream: "task-1", Actor: machine, Type: events.WorktreeReady, Data: map[string]any{"path": t.TempDir(), "branch": "b", "port": 7901}},
		events.Proposal{Stream: "task-1", Actor: machine, Type: events.TurnStarted, Correlation: "turn-1", Data: map[string]any{"agent": "engineer", "hash": "h"}},
	)
}

// endTurn ends turn-1 as its hook's call does, its knowledge file holding
// learned, and fails the test unless it ended.
func (f *eraseFixture) endTurn(t *testing.T, learned string) {
	t.Helper()
	f.endTurnOf(t, "turn-1", learned)
}

// endTurnOf is endTurn of the engineer's turn in task-1.
func (f *eraseFixture) endTurnOf(t *testing.T, turn, learned string) {
	t.Helper()
	f.hooks.Handle(context.Background(), hookcall.Call{Task: "task-1", Turn: turn, Agent: "engineer", Engine: "claude", Event: "Stop",
		Payload: json.RawMessage(`{"hook_event_name":"Stop","last_assistant_message":"Done."}`), Ending: &hookcall.Ending{Tokens: 1, Knowledge: &learned}})
	if task, _ := f.state.Task("task-1"); task.OpenTurn() != nil {
		t.Errorf("%s should have ended: %+v", turn, task.OpenTurn())
	}
}

// A redaction or a preview that can't be is answered as such, and writes
// nothing: a blank text 400, an erased task 410 and one there never was
// 404 before any row is read, a text no row holds 409 with the store's
// reason, and the desk 401 on every route.
func TestARefusedRedactionWritesNothing(t *testing.T) {
	f := withSecret(t)
	erasure := f.erase(t, "task-2")
	last := f.log.LastPosition()
	routes := []string{"/api/tasks/task-4/redact", "/api/tasks/task-4/redact/preview", "/api/agents/redact", "/api/agents/redact/preview"}
	for _, path := range routes {
		for _, blank := range []string{`{}`, `{"text":" \n "}`} {
			if code, body := call(t, f.server, "POST", path, blank); code != http.StatusBadRequest {
				t.Errorf("%s %s: %d %s, want 400", path, blank, code, body)
			}
		}
		if code, body := call(t, f.server, "POST", path, `{"text":"nowhere-to-be-found"}`); code != http.StatusConflict || !strings.Contains(body, "no row") {
			t.Errorf("%s a text no row holds: %d %s, want 409", path, code, body)
		}
		if code, body := callAs(t, f.server, "desk-secret", "POST", path); code != http.StatusUnauthorized {
			t.Errorf("the desk on %s: %d %s, want 401", path, code, body)
		}
	}
	gone := "task-2 was erased on " + erasure.At.Format("2 January 2006")
	for _, path := range []string{"/api/tasks/{task}/redact", "/api/tasks/{task}/redact/preview"} {
		erased := strings.ReplaceAll(path, "{task}", "task-2")
		if code, body := call(t, f.server, "POST", erased, `{"text":"`+plantedHere+`"}`); code != http.StatusGone || strings.TrimSpace(body) != gone {
			t.Errorf("%s: %d %q, want 410 %q", erased, code, body, gone)
		}
		unknown := strings.ReplaceAll(path, "{task}", "task-9")
		if code, body := call(t, f.server, "POST", unknown, `{"text":"`+secretHere+`"}`); code != http.StatusNotFound || strings.TrimSpace(body) != "no such task task-9" {
			t.Errorf("%s: %d %q, want 404 %q", unknown, code, body, "no such task task-9")
		}
	}
	if f.log.LastPosition() != last {
		t.Errorf("the refusals wrote up to %d from %d", f.log.LastPosition(), last)
	}
}

// An archived task whose blob is lost fails 500 on both its routes, the
// answer naming the task, not the blob's path on the server, and the
// preview not saying it redacts.
func TestAnArchivedTaskWhoseBlobIsLostFails(t *testing.T) {
	f := newEraseFixture(t)
	f.propose(t, events.Proposal{Stream: "task-4", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Old", "description": "It was " + secretHere + ".",
		"repo": "/r", "coordinator": "tech-lead"}})
	storetest.Archive(t, f.log, f.state, "task-4") // its blob in a folder the server doesn't read
	last := f.log.LastPosition()
	for _, path := range []string{"/api/tasks/task-4/redact/preview", "/api/tasks/task-4/redact"} {
		if code, body := call(t, f.server, "POST", path, `{"text":"`+secretHere+`","why":"Pasted."}`); code != http.StatusInternalServerError ||
			strings.TrimSpace(body) != "task-4's archived rows can't be read: its blob is missing" {
			t.Errorf("%s with task-4's blob lost: %d %q, want 500 naming task-4 and no path", path, code, body)
		}
	}
	if f.log.LastPosition() != last {
		t.Errorf("the failed redaction wrote up to %d from %d", f.log.LastPosition(), last)
	}
}

// A redaction holds to the rows the page counted: once a row holding the
// text is written after the count, the count sent is refused 409 with the
// rows there are now, and nothing is written, nor does the server hold
// the secret for running turns. Counted again, it redacts. A count below
// one is refused 400.
func TestARedactionWithAStaleCountIsRefused(t *testing.T) {
	f := withSecret(t)
	f.teach(t, "# Notes\n\nThe key is "+secretHere+".")
	counted := map[string]int{}
	for _, path := range []string{"/api/tasks/task-4/redact", "/api/agents/redact"} {
		counted[path] = f.preview(t, path, secretHere)
	}
	f.propose(t, events.Proposal{Stream: "task-4", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Deploy with " + secretHere}})
	f.teach(t, "# Notes\n\nThe key is "+secretHere+".\n\nDeploy on Fridays.")
	last := f.log.LastPosition()
	for path, rows := range counted {
		body, _ := json.Marshal(map[string]any{"text": secretHere, "why": "Pasted by mistake.", "rows": rows})
		want := "3 rows hold this text now, not 2: count them again"
		if path == "/api/agents/redact" {
			want = "2 rows hold this text now, not 1: count them again"
		}
		if code, answer := call(t, f.server, "POST", path, string(body)); code != http.StatusConflict || strings.TrimSpace(answer) != want {
			t.Errorf("%s with %d rows counted: %d %q, want 409 %q", path, rows, code, answer, want)
		}
		if code, answer := call(t, f.server, "POST", path, `{"text":"`+secretHere+`","rows":0}`); code != http.StatusBadRequest {
			t.Errorf("%s with no rows counted: %d %s, want 400", path, code, answer)
		}
	}
	if f.log.LastPosition() != last {
		t.Errorf("the refusals wrote up to %d from %d", f.log.LastPosition(), last)
	}
	if f.hooks.Redactions().Holds(secretHere) {
		t.Error("the secret is held for running turns after its redaction was refused")
	}

	body, _ := json.Marshal(map[string]any{"text": secretHere, "why": "Pasted by mistake.", "rows": 3})
	if code, answer := call(t, f.server, "POST", "/api/tasks/task-4/redact", string(body)); code != http.StatusOK {
		t.Errorf("counted again: %d %s, want 200", code, answer)
	}
}

// The same for an archived task, whose rows are counted through its blob:
// a count that no longer holds is refused 409 with the rows there are,
// and nothing is written, nor is its blob replaced.
func TestARedactionOfAnArchivedTaskWithAStaleCountIsRefused(t *testing.T) {
	f := withSecret(t)
	f.propose(t, events.Proposal{Stream: "task-4", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "Deployed.", "moved": true}})
	storetest.ArchiveIn(t, f.log, f.state, "task-4", f.blobs)
	old := markerOf(t, f, "task-4")
	rows := f.preview(t, "/api/tasks/task-4/redact", secretHere)
	last := f.log.LastPosition()
	body, _ := json.Marshal(map[string]any{"text": secretHere, "why": "Pasted by mistake.", "rows": rows + 1})
	want := "2 rows hold this text now, not 3: count them again"
	if code, answer := call(t, f.server, "POST", "/api/tasks/task-4/redact", string(body)); code != http.StatusConflict || strings.TrimSpace(answer) != want {
		t.Errorf("redacting archived task-4 with %d rows counted: %d %q, want 409 %q", rows+1, code, answer, want)
	}
	if f.log.LastPosition() != last {
		t.Errorf("the refusal wrote up to %d from %d", f.log.LastPosition(), last)
	}
	if !f.blobs.Has(old) {
		t.Errorf("the refusal took task-4's archive blob %s", old)
	}
}
