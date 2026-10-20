package store

import (
	"errors"
	"io"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
)

// Store.RedactArchived's refusals and failures, with the stand-in model and
// check. What it leaves on disk and in the state is in storetest's
// redact_archived_test.go.

// archivedWithRow opens a log at path with check, holding task-1 created at
// 1, a row of kind with data, written by hand, at 2, and task-1's marker at
// 3, its rows in blobs; task-2 stays live at 4.
func archivedWithRow(t *testing.T, path string, check OrderCheck, kind events.Type, data string) (*Store, sharedfiles.Disk, events.Event) {
	t.Helper()
	s, _ := openAt(t, path)
	acceptAll(t, s, newTask("task-1"))
	s.Close()
	execOn(t, path, `INSERT INTO events (at, stream, actor_kind, actor_id, type, data) VALUES
		('2026-10-19T09:00:00Z', 'task-1', 'human', 'you', '`+string(kind)+`', '`+data+`')`)
	s, err := Open(path, withNotes, newCreated(), check)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	marker, rejection, err := s.Archive("task-1", summary, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	acceptAll(t, s, newTask("task-2"))
	return s, blobs, marker
}

func acceptEvery(events.Proposal) *events.Rejection { return nil }

// holdingSecret is a rename whose title holds the secret, as the log
// writes it.
var holdingSecret = `{"title":"Use ` + secret + `"}`

// What Redact would refuse in an archived task's rows is refused before
// the task is unarchived, and nothing is written: the same rows, marker
// and blob. So is a task that isn't archived, which Redact redacts.
func TestARefusedArchivedRedactionWritesNothing(t *testing.T) {
	for _, c := range []struct {
		name, stream string
		kind         events.Type
		data, why    string
		want         string
	}{
		{"no row holds it", "task-1", events.TaskRenamed, `{"title":"Plain"}`, "", "no row of task-1 holds the secret"},
		{"the reason holds it", "task-1", events.TaskRenamed, holdingSecret, "Remove " + secret + ".", "the reason holds the secret, which the log would then keep"},
		{"a knowledge version", "task-1", events.AgentLearned, `{"name":"engineer","knowledge":"Use ` + secret + `."}`, "",
			"the row at 2 is a knowledge version, which is redacted by writing it again in full"},
		{"a key", "task-1", events.TaskRenamed, `{"title":"x","extra":{"` + secret + `":"x"}}`, "",
			"the row at 2 holds the secret in a key, which redacting doesn't rewrite"},
		{"escaped otherwise", "task-1", events.TaskRenamed, `{"title":"<Use ` + secret + `>"}`, "",
			"the row at 2 isn't written as the log writes data: a string holding the secret is escaped otherwise, so rewriting it would change more than the secret"},
		{"not archived", "task-2", events.TaskRenamed, holdingSecret, "", "task-2 isn't archived: it is redacted as it is"},
	} {
		path := filepath.Join(t.TempDir(), "agentry.db")
		s, blobs, marker := archivedWithRow(t, path, acceptEvery, c.kind, c.data)
		before := rowsAt(t, path)
		if _, rejection, err := s.RedactArchived(c.stream, secret, you, c.why, summary, blobs); err != nil || rejection == nil || rejection.Reason != c.want {
			t.Errorf("%s: %v, %v, want %q", c.name, rejection, err, c.want)
		}
		if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
			t.Errorf("%s: a refused redaction changed the log:\n%v\n%v", c.name, before, after)
		}
		if !blobs.Has(marker.Text("archive")) {
			t.Errorf("%s: the task's blob is gone", c.name)
		}
	}
}

// refusing is a check that refuses an event.redacted while refuse holds.
func refusing(refuse *bool) OrderCheck {
	return func(p events.Proposal) *events.Rejection {
		if p.Type == events.EventRedacted && *refuse {
			return &events.Rejection{Check: "test", Reason: "no redaction now"}
		}
		return nil
	}
}

// failingPut is blobs that keep nothing more.
type failingPut struct{ sharedfiles.Disk }

func (failingPut) Put(io.Reader) (string, int64, error) { return "", 0, errors.New("the disk is full") }

// stillArchived fails the test unless stream is archived, and returns its
// marker.
func stillArchived(t *testing.T, s *Store, stream string) events.Event {
	t.Helper()
	live, err := s.Log(stream)
	if err != nil {
		t.Fatal(err)
	}
	marker, archived := MarkerOf(live)
	if !archived {
		t.Fatalf("%s isn't archived", stream)
	}
	return marker
}

// A redaction refused once the task is unarchived archives it again, in a
// new blob, and keeps the old one, which holds the secret still; when that
// archive fails too, the error says the task is left unarchived.
func TestARedactionRefusedAfterTheUnarchivingArchivesTheTaskAgain(t *testing.T) {
	refuse := true
	s, blobs, old := archivedWithRow(t, filepath.Join(t.TempDir(), "agentry.db"), refusing(&refuse), events.TaskRenamed, holdingSecret)
	redaction, rejection, err := s.RedactArchived("task-1", secret, you, "", summary, blobs)
	if err != nil || rejection == nil || rejection.Reason != "no redaction now" || redaction.Position != 0 {
		t.Fatalf("redacting: %+v, %v, %v, want refused", redaction, rejection, err)
	}
	marker := stillArchived(t, s, "task-1")
	if marker.Position == old.Position || marker.Text("archive") == old.Text("archive") || !blobs.Has(marker.Text("archive")) {
		t.Errorf("task-1's marker %+v, want a new one, with a new blob, after the old %+v", marker, old)
	}
	if !blobs.Has(old.Text("archive")) {
		t.Error("the old blob is gone, though the redaction was refused")
	}
	if count, err := s.Matches("task-1", secret); count != 0 || err != nil {
		t.Errorf("task-1's live rows holding the secret: %d, %v, want none: it is archived", count, err)
	}

	_, rejection, err = s.RedactArchived("task-1", secret, you, "", summary, failingPut{blobs})
	if rejection == nil || err == nil || !strings.Contains(err.Error(), "task-1 is left unarchived") || !strings.Contains(err.Error(), "the disk is full") {
		t.Errorf("redacting with no room to archive again: %v, %v, want refused and left unarchived", rejection, err)
	}
	if live, _ := s.Log("task-1"); len(live) < 2 {
		t.Errorf("task-1 has %d live rows, want its rows back", len(live))
	}
	for _, hash := range []string{old.Text("archive"), marker.Text("archive")} {
		if !blobs.Has(hash) {
			t.Errorf("the blob %s is gone, though nothing was redacted", hash)
		}
	}
}

// A redaction that commits but can't archive the task again comes back
// with an error that says it is left unarchived: redacted, and its old
// blob deleted, as the redaction says.
func TestARedactedTaskThatCantBeArchivedAgainIsLeftUnarchived(t *testing.T) {
	s, blobs, old := archivedWithRow(t, filepath.Join(t.TempDir(), "agentry.db"), acceptEvery, events.TaskRenamed, holdingSecret)
	redaction, rejection, err := s.RedactArchived("task-1", secret, you, "", summary, failingPut{blobs})
	if rejection != nil || err == nil || !strings.Contains(err.Error(), "task-1 is left unarchived") || redaction.Type != events.EventRedacted {
		t.Fatalf("redacting: %+v, %v, %v, want redacted and left unarchived", redaction, rejection, err)
	}
	if names := redaction.List("archives"); !reflect.DeepEqual(names, []string{old.Text("archive")}) {
		t.Errorf("the redaction names %v, want the old blob", names)
	}
	if blobs.Has(old.Text("archive")) {
		t.Error("the old blob is still there")
	}
	live, err := s.Log("task-1")
	if _, archived := MarkerOf(live); archived || err != nil {
		t.Errorf("task-1 is archived: %v", err)
	}
	if count, err := s.Matches("task-1", secret); count != 0 || err != nil {
		t.Errorf("task-1's rows holding the secret: %d, %v, want none", count, err)
	}
}
