package store

import (
	"path/filepath"
	"reflect"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// A redaction the user confirmed holds to the rows they were shown: one
// written since the count, holding the text too, refuses it, and nothing
// is rewritten. Counted again, it commits.

// renamedTo is task stream renamed to title.
func renamedTo(stream, title string) events.Proposal {
	p := renamed(stream)
	p.Data = map[string]any{"title": title}
	return p
}

// A live task's redaction with the count Preview gave commits; one with
// the count from before a row holding the secret was written is refused,
// leaving every row as it was, and so is one whose stream took the row
// between the rows read and the commit, as the count is checked under the
// lock with the rows the commit would rewrite.
func TestARedactionHoldsToTheCountTheUserSaw(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s := openWithSecret(t, path, newCreated())
	counted, rejection, err := s.Preview("task-1", secret)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	acceptAll(t, s, renamedTo("task-1", "Again "+secret))
	before := rowsAt(t, path)
	want := "3 rows hold this text now, not 2: count them again"
	if _, rejection, err := s.RedactCounted("task-1", secret, counted, you, ""); counted != 2 || err != nil || rejection == nil || rejection.Reason != want {
		t.Errorf("counted %d, then: %v, %v, want %q", counted, rejection, err, want)
	}
	found, rejection, err := s.find("task-1", secret, nil, you, "")
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	found.counted = counted
	if _, rejection, err := s.rewrite(found); err != nil || rejection == nil || rejection.Reason != want {
		t.Errorf("rewrite with the stale count: %v, %v, want %q", rejection, err, want)
	}
	if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
		t.Errorf("a redaction refused for its count changed the log:\n%v\n%v", before, after)
	}

	redaction, rejection, err := s.RedactCounted("task-1", secret, counted+1, you, "")
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if positions := redaction.Numbers("positions"); len(positions) != counted+1 {
		t.Errorf("the redaction rewrote %v, want %d rows", positions, counted+1)
	}
}

// An archived task's redaction with a stale count is refused before the
// task is unarchived: the rows and the blob are as they were. With the
// count PreviewArchived gives now, it commits.
func TestAnArchivedRedactionHoldsToTheCountTheUserSaw(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, blobs, marker := archivedWithRow(t, path, acceptEvery, events.TaskRenamed, holdingSecret)
	counted, rejection, err := s.PreviewArchived("task-1", secret, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	before := rowsAt(t, path)
	want := "1 row holds this text now, not 2: count them again"
	if _, rejection, err := s.RedactArchivedCounted("task-1", secret, counted+1, you, "", summary, blobs); err != nil || rejection == nil || rejection.Reason != want {
		t.Errorf("RedactArchivedCounted with a stale count: %v, %v, want %q", rejection, err, want)
	}
	if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
		t.Errorf("a redaction refused for its count changed the log:\n%v\n%v", before, after)
	}
	if !blobs.Has(marker.Text("archive")) {
		t.Error("the task's blob is gone")
	}

	redaction, rejection, err := s.RedactArchivedCounted("task-1", secret, counted, you, "", summary, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if positions := redaction.Numbers("positions"); counted != 1 || len(positions) != counted {
		t.Errorf("counted %d, the redaction rewrote %v", counted, positions)
	}
}

// The agents' redaction with the count from before an agent learned the
// secret again is refused, every version as it was.
func TestTheAgentsRedactionHoldsToTheCountTheUserSaw(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	acceptAll(t, s, agentCreated("engineer"))
	learn(t, s, "engineer", engineersVersions[:3]...)
	counted, rejection, err := s.Preview(events.AgentsStream, secret)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	learn(t, s, "engineer", engineersVersions[3])
	before := rowsAt(t, path)
	want := "2 rows hold this text now, not 1: count them again"
	if _, rejection, err := s.RedactCounted(events.AgentsStream, secret, counted, you, ""); counted != 1 || err != nil || rejection == nil || rejection.Reason != want {
		t.Errorf("counted %d, then: %v, %v, want %q", counted, rejection, err, want)
	}
	if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
		t.Errorf("a redaction refused for its count changed the log:\n%v\n%v", before, after)
	}
}
