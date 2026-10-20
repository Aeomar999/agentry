package store

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
)

// RedactRows and RedactArchivedRows: Redact and RedactArchived of only the
// rows named, as an erased story's words are redacted in the rows of its
// epic that copy them and nowhere else in the epic.

// RedactRows rewrites only the rows named that hold the secret: another
// row of the stream holding it is byte for byte as it was, and so is a row
// named that doesn't hold it. The redaction names only the row rewritten.
func TestRedactRowsRewritesOnlyTheRowsNamed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s := openWithSecret(t, path, newCreated())
	before := rowsAt(t, path)

	redacted, rejection, err := s.RedactRows("task-1", secret, []int64{2, 3}, you, "")
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	after := rowsAt(t, path)
	for i, was := range before {
		want := was
		if was.Position == 2 {
			want.Data = strings.ReplaceAll(was.Data, secret, Redacted)
		}
		if after[i] != want {
			t.Errorf("the row at %d:\n got %+v\nwant %+v", was.Position, after[i], want)
		}
	}
	if !strings.Contains(after[0].Data, secret) {
		t.Fatal("the row at 1, not named, should still hold the secret")
	}
	if positions := redacted.Data["positions"]; !reflect.DeepEqual(positions, []any{2.0}) {
		t.Errorf("the redaction names %v, want only 2", positions)
	}
}

// RedactRows refuses, and writes nothing, when no row named holds the
// secret, though another row of the stream does: a row of it that doesn't,
// a row of another stream, or none named at all. Redact's own refusals
// stand for the rows named: the reason holding the secret, and a row named
// holding it in a number; a row not named holding it in a number doesn't
// stop the rows named.
func TestRedactRowsRefusesWhatRedactWouldOfTheRowsNamed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s := openWithSecret(t, path, newCreated())
	for _, c := range []struct {
		name      string
		positions []int64
		why, want string
	}{
		{"a row named that doesn't hold it", []int64{3}, "", "no row of task-1 holds the secret"},
		{"another stream's row", []int64{4}, "", "no row of task-1 holds the secret"},
		{"no row named", nil, "", "no row of task-1 holds the secret"},
		{"the reason holds it", []int64{2}, "Remove " + secret + ".", "the reason holds the secret, which the log would then keep"},
	} {
		before := rowsAt(t, path)
		if _, rejection, err := s.RedactRows("task-1", secret, c.positions, you, c.why); err != nil || rejection == nil || rejection.Reason != c.want {
			t.Errorf("%s: %v, %v, want %q", c.name, rejection, err, c.want)
		}
		if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
			t.Errorf("%s: a refused redaction changed the log:\n%v\n%v", c.name, before, after)
		}
	}

	path = filepath.Join(t.TempDir(), "agentry.db")
	s, _ = openAt(t, path)
	created := newTask("task-1")
	created.Data["extra"] = 90210417
	acceptAll(t, s, created, events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "PIN " + pin}})
	want := "the row at 1 holds the secret outside its text, as in a number, which redacting can't rewrite: erase the task, or redact a longer piece of the text"
	if _, rejection, err := s.RedactRows("task-1", pin, []int64{1, 2}, you, ""); err != nil || rejection == nil || rejection.Reason != want {
		t.Errorf("a row named with the secret in a number: %v, %v, want %q", rejection, err, want)
	}
	before := rowsAt(t, path)
	if _, rejection, err := s.RedactRows("task-1", pin, []int64{2}, you, ""); rejection != nil || err != nil {
		t.Fatalf("a row not named with the secret in a number: %v, %v", rejection, err)
	}
	if after := rowsAt(t, path); after[0] != before[0] || strings.Contains(after[1].Data, pin) {
		t.Errorf("the rows after:\n%v\nwant the first as it was and the second redacted", after)
	}
}

// RedactArchivedRows names the rows by the positions read through the
// blob, which are theirs again once unarchived: it rewrites only those,
// and the task, archived again, keeps its other rows holding the secret
// byte for byte. When no row named holds it, nothing is written: the same
// rows and blobs.
func TestRedactArchivedRowsRewritesOnlyTheRowsNamed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	created := newTask("task-1")
	created.Data["title"] = "Use " + secret
	acceptAll(t, s, created, events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Use " + secret + " again"}})
	wasCreated := rowsAt(t, path)[0]
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	if _, rejection, err := s.Archive("task-1", summary, blobs); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}

	before, kept := rowsAt(t, path), blobsIn(t, blobs)
	if _, rejection, err := s.RedactArchivedRows("task-1", secret, []int64{3}, you, "", summary, blobs); err != nil || rejection == nil || rejection.Reason != "no row of task-1 holds the secret" {
		t.Errorf("no row named holds it: %v, %v, want refused", rejection, err)
	}
	if after := rowsAt(t, path); !reflect.DeepEqual(after, before) || !reflect.DeepEqual(blobsIn(t, blobs), kept) {
		t.Errorf("a refused redaction changed the log or the blobs:\n%v\n%v", before, after)
	}

	redacted, rejection, err := s.RedactArchivedRows("task-1", secret, []int64{2}, you, "", summary, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if positions := redacted.Data["positions"]; !reflect.DeepEqual(positions, []any{2.0}) {
		t.Errorf("the redaction names %v, want only 2", positions)
	}
	rows, err := keptBy(stillArchived(t, s, "task-1"), blobs)
	if err != nil {
		t.Fatal(err)
	}
	if rows[0] != wasCreated {
		t.Errorf("the row at 1, not named:\n got %+v\nwant %+v", rows[0], wasCreated)
	}
	if rows[1].Position != 2 || rows[1].Data != `{"title":"Use [redacted] again"}` {
		t.Errorf("the row at 2, named: %+v", rows[1])
	}
}
