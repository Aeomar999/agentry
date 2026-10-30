package storetest

import (
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// Redacting an archived task, as the state and the files on disk see it
// (docs/design/log-growth.md section 5, "Removing a secret", item 5). The
// store's side, its refusals and failures, is in store's
// redact_archived_test.go.

// leaked is the secret in archived task-1, with text JSON escapes.
const leaked = `ghp_"7c1e<a9`

func redactArchived(t testing.TB, s *store.Store, state *current.State, id string, blobs store.ArchiveBlobs) events.Event {
	t.Helper()
	event, rejection, err := s.RedactArchived(id, leaked, you, "A token pasted by mistake.", summarise(state, id), blobs)
	if rejection != nil || err != nil {
		t.Fatalf("redacting archived %s: %v, %v", id, rejection, err)
	}
	return event
}

// withLeaked seeds the roster and gives task-1 the secret in its title, so
// in its marker's summary too, and a direction long enough to need
// overflow pages, and archives it into blobs; task-2's rows sit among
// task-1's, and keep kept.
func withLeaked(t testing.TB, s *store.Store, state *current.State, blobs store.Blobs) {
	t.Helper()
	for _, p := range append(agents.SeedProposals(nil),
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "One " + leaked, "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Two " + kept, "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskDirected, Data: map[string]any{"text": strings.Repeat("Use the token "+leaked+". ", 400)}},
		events.Proposal{Stream: "task-2", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Two, renamed " + kept}},
	) {
		mustPropose(t, s, p)
	}
	ArchiveIn(t, s, state, "task-1", blobs)
}

// archivesOf are the blobs the task.archived events among found name.
func archivesOf(found []events.Event) []string {
	var hashes []string
	for _, e := range found {
		if e.Type == events.TaskArchived {
			hashes = append(hashes, e.Text("archive"))
		}
	}
	return hashes
}

// Test 1: once an archived task's secret is redacted, the task is archived
// again in a new blob, the old one is gone, and the secret is in no byte
// of the log, its WAL or any blob, as it is or escaped. Its history reads
// back as it was, [redacted] in place of the secret, live and after a
// restart.
func TestARedactedArchivedTaskIsInNoByteOnDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentry.db")
	s, state := Open(t, path)
	blobs := sharedfiles.Disk{Dir: filepath.Join(dir, "files")}
	withLeaked(t, s, state, blobs)
	before, err := s.History("task-1", store.FromBlobs(blobs))
	if err != nil {
		t.Fatal(err)
	}
	old := archivesOf(before)
	if len(HoldingAny(t, dir, leaked)) == 0 {
		t.Fatalf("before redacting, %q is in no file", leaked)
	}

	redaction := redactArchived(t, s, state, "task-1", blobs)
	if names := redaction.List("archives"); len(old) != 1 || !reflect.DeepEqual(names, old) {
		t.Errorf("the redaction names %v, want the old blob %v", names, old)
	}
	if blobs.Has(old[0]) {
		t.Error("the old blob is still there")
	}
	marker, archived := store.MarkerOf(mustLog(t, s, "task-1"))
	if !archived || marker.Text("archive") == old[0] || !blobs.Has(marker.Text("archive")) {
		t.Errorf("task-1's marker %+v: want it archived again, in a new blob", marker)
	}
	if task, err := state.Task("task-1"); task != nil || !errors.Is(err, current.ErrArchived) {
		t.Errorf("task-1 in the state: %v, %v, want archived", task, err)
	}
	if summary, _ := state.Summary("task-1"); summary.Title != "One "+store.Redacted {
		t.Errorf("task-1's summary is titled %q, want it redacted", summary.Title)
	}
	if holding := HoldingAny(t, dir, leaked); len(holding) != 0 {
		t.Errorf("after redacting, %q is still in %v", leaked, holding)
	}
	if holding := FilesHolding(t, dir, kept); len(holding) == 0 {
		t.Errorf("task-2's %q is in no file: the search can't see the live rows", kept)
	}

	after, err := s.History("task-1", store.FromBlobs(blobs))
	if err != nil {
		t.Fatal(err)
	}
	checkRedactedHistory(t, before, after)
	CheckEqual(t, s, state)
	_, restarted := Open(t, path)
	again, _ := restarted.Summary("task-1")
	if live, _ := state.Summary("task-1"); !reflect.DeepEqual(again, live) {
		t.Errorf("task-1's summary after a restart: %+v, want %+v", again, live)
	}
}

// checkRedactedHistory fails the test unless each event of before is in
// after, at its position, with leaked as [redacted] and every other byte
// as it was.
func checkRedactedHistory(t *testing.T, before, after []events.Event) {
	t.Helper()
	at := map[int64]events.Event{}
	for _, e := range after {
		at[e.Position] = e
	}
	escaped := writtenAs(leaked)[1]
	for _, e := range before {
		was, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		now, err := json.Marshal(at[e.Position])
		if err != nil {
			t.Fatal(err)
		}
		if want := strings.ReplaceAll(string(was), escaped, store.Redacted); string(now) != want {
			t.Errorf("the event at %d reads back as\n%s\nwant\n%s", e.Position, now, want)
		}
	}
}

// Test 2: a task archived, unarchived and archived again has two old
// blobs, the second holding the first marker: both are deleted, and the
// redaction names both.
func TestARedactionDeletesEveryBlobTheTaskWasArchivedIn(t *testing.T) {
	dir := t.TempDir()
	s, state := Open(t, filepath.Join(dir, "agentry.db"))
	blobs := sharedfiles.Disk{Dir: filepath.Join(dir, "files")}
	withLeaked(t, s, state, blobs)
	unarchive(t, s, "task-1", blobs)
	ArchiveIn(t, s, state, "task-1", blobs)
	history, err := s.History("task-1", store.FromBlobs(blobs))
	if err != nil {
		t.Fatal(err)
	}
	old := archivesOf(history)

	redaction := redactArchived(t, s, state, "task-1", blobs)
	if names := redaction.List("archives"); len(old) != 2 || !reflect.DeepEqual(names, old) {
		t.Errorf("the redaction names %v, want both old blobs %v", names, old)
	}
	for _, hash := range old {
		if blobs.Has(hash) {
			t.Errorf("the old blob %s is still there", hash)
		}
	}
	if holding := HoldingAny(t, dir, leaked); len(holding) != 0 {
		t.Errorf("after redacting, %q is still in %v", leaked, holding)
	}
}

// Test 3: a task redacted while archived is erased as any archived task
// is: the blobs its earlier markers name are gone already, and its new
// one goes.
func TestARedactedArchivedTaskIsErased(t *testing.T) {
	s, state := Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	withLeaked(t, s, state, blobs)
	redactArchived(t, s, state, "task-1", blobs)
	marker, _ := store.MarkerOf(mustLog(t, s, "task-1"))

	erased := erase(t, s, "task-1", blobs)
	if blobs.Has(marker.Text("archive")) {
		t.Error("the blob of task-1's rows is still there")
	}
	if names := erased.List("archives"); len(names) != 2 || names[1] != marker.Text("archive") {
		t.Errorf("the erasure names %v, want the deleted blob and the latest", names)
	}
	if _, erased := state.Erased("task-1"); !erased {
		t.Error("task-1 isn't erased")
	}
}

// failingPut is blobs that keep nothing more.
type failingPut struct{ sharedfiles.Disk }

func (failingPut) Put(io.Reader) (string, int64, error) { return "", 0, errors.New("the disk is full") }

// Test 4: a redaction that commits but can't archive the task again leaves
// it unarchived and redacted, the state as a replay of the log has it,
// live and after a restart, and the secret in no file. Archived later, it
// is redacted while archived as any task is: no row holds the secret.
func TestARedactedTaskLeftUnarchivedIsArchivedAndRedactedLater(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agentry.db")
	state := NewState(events.Core())
	s := openChecked(t, path, state)
	blobs := sharedfiles.Disk{Dir: filepath.Join(dir, "files")}
	withLeaked(t, s, state, blobs)

	redaction, rejection, err := s.RedactArchived("task-1", leaked, you, "A token pasted by mistake.", summarise(state, "task-1"), failingPut{blobs})
	if rejection != nil || err == nil || !strings.Contains(err.Error(), "task-1 is left unarchived") || redaction.Type != events.EventRedacted {
		t.Fatalf("redacting: %+v, %v, %v, want redacted and left unarchived", redaction, rejection, err)
	}
	if task, err := state.Task("task-1"); task == nil || err != nil || task.Title != "One "+store.Redacted {
		t.Errorf("task-1 in the state: %+v, %v, want it live and redacted", task, err)
	}
	CheckEqual(t, s, state)
	s.Close()
	restarted, again := Open(t, path)
	CheckEqual(t, restarted, again)
	if holding := HoldingAny(t, dir, leaked); len(holding) != 0 {
		t.Errorf("after redacting, %q is still in %v", leaked, holding)
	}

	ArchiveIn(t, restarted, again, "task-1", blobs)
	if _, rejection, err := restarted.RedactArchived("task-1", leaked, you, "", summarise(again, "task-1"), blobs); err != nil || rejection == nil ||
		rejection.Reason != "no row of task-1 holds the secret" {
		t.Errorf("redacting it again once archived: %v, %v, want no row holding the secret", rejection, err)
	}
	if _, archived := store.MarkerOf(mustLog(t, restarted, "task-1")); !archived {
		t.Error("task-1 isn't archived")
	}
}
