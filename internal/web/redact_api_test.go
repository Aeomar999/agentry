package web

import (
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// Redacting a secret through the page, as web wires settingsapi's routes
// (docs/design/log-growth.md section 5, "Removing a secret"). The routes'
// tests are in internal/domain/settings/settingsapi; this one sees web's
// cache of opened archives, which no route shows. Its helpers mirror
// internal/domain/settings/settingsapi/redact_api_test.go's.

// secretHere is a secret pasted into task-4, which nothing else holds.
const secretHere = "sk-live-0c9e44d1"

// redacted is what redacting a secret answers.
type redacted struct {
	Stream   string    `json:"stream"`
	At       time.Time `json:"at"`
	Rows     int       `json:"rows"`
	Warnings []string  `json:"warnings"`
}

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

// settingsapi's TestRedactingALiveTaskLeavesNoTraceOnDisk for an archived
// task, opened from the page first, whose rows are in its blob, not the
// snapshot: it is archived again, its old blob is gone, and the cache of
// opened archives, whose entry held the secret, drops it.
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
