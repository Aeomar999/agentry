package store

import (
	"cmp"
	"errors"
	"io"
	"io/fs"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
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
// PreviewArchived gives the same rejection, but for the reason, which it
// isn't given.
func TestARefusedArchivedRedactionWritesNothing(t *testing.T) {
	for _, c := range []struct {
		name, stream string
		kind         events.Type
		data, why    string
		want         string
		secret       string // the package's secret when empty
	}{
		{"no row holds it", "task-1", events.TaskRenamed, `{"title":"Plain"}`, "", "no row of task-1 holds the secret", ""},
		{"the reason holds it", "task-1", events.TaskRenamed, holdingSecret, "Remove " + secret + ".", "the reason holds the secret, which the log would then keep", ""},
		{"a knowledge version", "task-1", events.AgentLearned, `{"name":"engineer","knowledge":"Use ` + secret + `."}`, "",
			"the row at 2 is a knowledge version, which is redacted by writing it again in full", ""},
		{"a key", "task-1", events.TaskRenamed, `{"title":"x","extra":{"` + secret + `":"x"}}`, "",
			"the row at 2 holds the secret in a key, which redacting doesn't rewrite", ""},
		{"escaped otherwise", "task-1", events.TaskRenamed, `{"title":"<Use ` + secret + `>"}`, "",
			"the row at 2 isn't written as the log writes data: a string holding the secret is escaped otherwise, so rewriting it would change more than the secret", ""},
		{"not archived", "task-2", events.TaskRenamed, holdingSecret, "", "task-2 isn't archived: it is redacted as it is", ""},
		{"left in a number", "task-1", events.TaskRenamed, `{"title":"PIN ` + pin + `","extra":` + pin + `}`, "",
			"the row at 2 holds the secret outside its text, as in a number, which redacting can't rewrite: erase the task, or redact a longer piece of the text", pin},
	} {
		path := filepath.Join(t.TempDir(), "agentry.db")
		s, blobs, marker := archivedWithRow(t, path, acceptEvery, c.kind, c.data)
		before := rowsAt(t, path)
		if c.why == "" {
			if count, rejection, err := s.PreviewArchived(c.stream, cmp.Or(c.secret, secret), blobs); count != 0 || err != nil || rejection == nil || rejection.Reason != c.want {
				t.Errorf("%s: PreviewArchived %d, %v, %v, want %q", c.name, count, rejection, err, c.want)
			}
		}
		if _, rejection, err := s.RedactArchived(c.stream, cmp.Or(c.secret, secret), you, c.why, summary, blobs); err != nil || rejection == nil || rejection.Reason != c.want {
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
// new blob, and keeps the old one, which holds the secret still. Once the
// rules allow it, the redaction names both blobs, in order, and deletes
// them.
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
	if count, rejection, err := s.PreviewArchived("task-1", secret, blobs); count != 1 || rejection != nil || err != nil {
		t.Errorf("task-1's rows to redact: %d, %v, %v, want the one still holding the secret", count, rejection, err)
	}

	refuse = false
	redaction, rejection, err = s.RedactArchived("task-1", secret, you, "", summary, blobs)
	if rejection != nil || err != nil {
		t.Fatalf("redacting once allowed: %v, %v", rejection, err)
	}
	both := []string{old.Text("archive"), marker.Text("archive")}
	if names := redaction.List("archives"); !reflect.DeepEqual(names, both) {
		t.Errorf("the redaction names %v, want the original blob and the refused run's %v", names, both)
	}
	latest := stillArchived(t, s, "task-1")
	for _, hash := range both {
		if blobs.Has(hash) {
			t.Errorf("the blob %s is still there", hash)
		}
		if hash == latest.Text("archive") {
			t.Errorf("the new marker names the old blob %s", hash)
		}
	}
}

// A redaction refused when there is no room to archive the task again
// comes back with an error saying the task is left unarchived, and keeps
// both blobs: nothing was redacted.
func TestARefusedRedactionWithNoRoomToArchiveAgainLeavesTheTaskUnarchived(t *testing.T) {
	refuse := true
	s, blobs, old := archivedWithRow(t, filepath.Join(t.TempDir(), "agentry.db"), refusing(&refuse), events.TaskRenamed, holdingSecret)
	_, rejection, err := s.RedactArchived("task-1", secret, you, "", summary, failingPut{blobs})
	if rejection == nil || err == nil || !strings.Contains(err.Error(), "task-1 is left unarchived") || !strings.Contains(err.Error(), "the disk is full") {
		t.Errorf("redacting with no room to archive again: %v, %v, want refused and left unarchived", rejection, err)
	}
	if live, _ := s.Log("task-1"); len(live) < 2 {
		t.Errorf("task-1 has %d live rows, want its rows back", len(live))
	}
	if !blobs.Has(old.Text("archive")) {
		t.Error("the old blob is gone, though nothing was redacted")
	}
}

// refusingArchives is a check that refuses every event.redacted while
// redactions holds, and every task.archived after the first allowed, as
// the rules refuse archiving a task that is archived.
func refusingArchives(redactions *bool, allowed int) OrderCheck {
	archived := 0
	return func(p events.Proposal) *events.Rejection {
		switch {
		case p.Type == events.EventRedacted && *redactions:
			return &events.Rejection{Check: "test", Reason: "no redaction now"}
		case p.Type == events.TaskArchived:
			if archived++; archived > allowed {
				return &events.Rejection{Check: "test", Reason: "no archiving now"}
			}
		}
		return nil
	}
}

// blobsIn are the hashes of the blobs on disk.
func blobsIn(t *testing.T, blobs sharedfiles.Disk) []string {
	t.Helper()
	var hashes []string
	err := filepath.WalkDir(blobs.Dir, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			hashes = append(hashes, entry.Name())
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return hashes
}

// holdsSecret is whether a blob's rows hold the secret.
func holdsSecret(t *testing.T, blobs sharedfiles.Disk, hash string) bool {
	t.Helper()
	blob, err := blobs.Open(hash)
	if err != nil {
		t.Fatal(err)
	}
	defer blob.Close()
	rows, err := decodeArchive(blob)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		if strings.Contains(r.Data, secret) {
			return true
		}
	}
	return false
}

// Archive puts its blob before the rules refuse the marker: when archiving
// again is refused, that blob is removed, so every blob left is one a
// marker names, whether the redaction was refused (the blob held the
// secret) or committed (only the old blob did, and it is deleted).
func TestARefusedArchiveAgainLeavesNoBlobNoMarkerNames(t *testing.T) {
	for _, refuseRedaction := range []bool{true, false} {
		s, blobs, old := archivedWithRow(t, filepath.Join(t.TempDir(), "agentry.db"), refusingArchives(&refuseRedaction, 1), events.TaskRenamed, holdingSecret)
		_, _, err := s.RedactArchived("task-1", secret, you, "", summary, blobs)
		if err == nil || !strings.Contains(err.Error(), "task-1 is left unarchived: archiving it again was refused: no archiving now") {
			t.Errorf("redaction refused %v: %v, want left unarchived", refuseRedaction, err)
		}
		var want []string
		if refuseRedaction {
			want = []string{old.Text("archive")} // the live marker row names it
		}
		if got := blobsIn(t, blobs); !reflect.DeepEqual(got, want) {
			t.Errorf("redaction refused %v: the blobs on disk are %v, want %v", refuseRedaction, got, want)
		}
		for _, hash := range blobsIn(t, blobs) {
			if hash != old.Text("archive") && holdsSecret(t, blobs, hash) {
				t.Errorf("redaction refused %v: the blob %s, which no marker names, holds the secret", refuseRedaction, hash)
			}
		}
	}
}

// When another writer archived the task between the unarchiving and
// archiving it again, archiving again is refused, and the error says the
// task was archived by another writer, not that it is left unarchived.
// The blob it put is removed, and that writer's is kept.
func TestArchivingAgainAfterAnotherWriterSaysSo(t *testing.T) {
	redactions := false
	s, blobs, old := archivedWithRow(t, filepath.Join(t.TempDir(), "agentry.db"), refusingArchives(&redactions, 2), events.TaskRenamed, holdingSecret)
	if _, rejection, err := s.Unarchive("task-1", blobs); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	theirs, rejection, err := s.Archive("task-1", summary, blobs) // the other writer
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}

	err = s.archiveAgain("task-1", summary, blobs)
	if err == nil || !strings.Contains(err.Error(), "task-1 was archived again by another writer") || strings.Contains(err.Error(), "left unarchived") {
		t.Errorf("archiving again: %v, want it to say another writer archived task-1", err)
	}
	want := []string{old.Text("archive"), theirs.Text("archive")}
	slices.Sort(want)
	if got := blobsIn(t, blobs); !reflect.DeepEqual(got, want) {
		t.Errorf("the blobs on disk are %v, want the first and the other writer's %v", got, want)
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
	if count, _, err := s.Preview("task-1", secret); count != 0 || err != nil {
		t.Errorf("task-1's rows holding the secret: %d, %v, want none", count, err)
	}
}

// racingPut is blobs where, once the first blob is put, another writer
// archives the stream into the same blobs before the putter takes the
// lock.
type racingPut struct {
	sharedfiles.Disk
	s      *Store
	stream string
	theirs events.Event
	raced  bool
}

func (r *racingPut) Put(blob io.Reader) (string, int64, error) {
	hash, size, err := r.Disk.Put(blob)
	if err == nil && !r.raced {
		r.raced = true
		var rejection *events.Rejection
		r.theirs, rejection, err = r.s.Archive(r.stream, summary, r.Disk)
		if rejection != nil {
			err = errors.New(rejection.Reason)
		}
	}
	return hash, size, err
}

// When another writer archives the same rows while RedactArchived archives
// the task again, both put the same blob, which that writer's marker
// names: it is kept, the old blob is deleted, and the redaction comes back
// with no error, since the task is redacted and archived.
func TestARaceToArchiveAgainKeepsTheBlobTheWinnersMarkerNames(t *testing.T) {
	s, disk, old := archivedWithRow(t, filepath.Join(t.TempDir(), "agentry.db"), acceptEvery, events.TaskRenamed, holdingSecret)
	blobs := &racingPut{Disk: disk, s: s, stream: "task-1"}
	redaction, rejection, err := s.RedactArchived("task-1", secret, you, "", summary, blobs)
	if redaction.Type != events.EventRedacted || rejection != nil || err != nil {
		t.Fatalf("redacting: %+v, %v, %v, want redacted with no error", redaction, rejection, err)
	}
	if blobs.theirs.Position == 0 {
		t.Fatal("the other writer didn't archive task-1")
	}
	if marker := stillArchived(t, s, "task-1"); marker.Position != blobs.theirs.Position {
		t.Errorf("task-1's marker %+v, want the other writer's %+v", marker, blobs.theirs)
	}
	if got, want := blobsIn(t, disk), []string{blobs.theirs.Text("archive")}; !reflect.DeepEqual(got, want) || got[0] == old.Text("archive") {
		t.Errorf("the blobs on disk are %v, want the other writer's %v alone", got, want)
	}
	if disk.Has(old.Text("archive")) {
		t.Error("the old blob is still there")
	}
}

// An erased task is refused as erased, not as a task that isn't archived,
// which would send the user to Redact, which refuses it too.
func TestAnErasedTaskIsRefusedAsErased(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, blobs, _ := archivedWithRow(t, path, acceptEvery, events.TaskRenamed, holdingSecret)
	if _, rejection, err := s.Erase("task-1", you, "", blobs); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	before := rowsAt(t, path)
	_, rejection, err := s.RedactArchived("task-1", secret, you, "", summary, blobs)
	if want := "task-1 is erased: there is nothing left to redact"; err != nil || rejection == nil || rejection.Reason != want {
		t.Errorf("redacting an erased task: %v, %v, want %q", rejection, err, want)
	}
	if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
		t.Errorf("a refused redaction changed the log:\n%v\n%v", before, after)
	}
}

// refusingRemove is blobs that won't delete a blob refuse says to keep.
type refusingRemove struct {
	sharedfiles.Disk
	refuse func(hash string) bool
}

func (r refusingRemove) Remove(hash string) error {
	if r.refuse(hash) {
		return errors.New("the disk is read-only")
	}
	return r.Disk.Remove(hash)
}

// When the blob a refused archive put won't delete, the error names it,
// and the rest is as when it deletes: the task redacted, its old blob
// deleted, and the task left unarchived.
func TestABlobArchivingAgainPutThatWontDeleteIsNamed(t *testing.T) {
	redactions := false
	s, disk, old := archivedWithRow(t, filepath.Join(t.TempDir(), "agentry.db"), refusingArchives(&redactions, 1), events.TaskRenamed, holdingSecret)
	blobs := refusingRemove{disk, func(hash string) bool { return hash != old.Text("archive") }}
	redaction, _, err := s.RedactArchived("task-1", secret, you, "", summary, blobs)
	left := blobsIn(t, disk)
	if len(left) != 1 || left[0] == old.Text("archive") {
		t.Fatalf("the blobs on disk are %v, want only the one archiving again put", left)
	}
	if want := "removing the blob " + left[0] + " archiving task-1 again put: the disk is read-only"; err == nil || !strings.Contains(err.Error(), want) {
		t.Errorf("redacting: %v, want it to say %q", err, want)
	}
	if redaction.Type != events.EventRedacted {
		t.Errorf("redacting: %+v, want the redaction", redaction)
	}
	live, err := s.Log("task-1")
	if _, archived := MarkerOf(live); archived || err != nil {
		t.Errorf("task-1 is archived: %v", err)
	}
	if count, _, err := s.Preview("task-1", secret); count != 0 || err != nil {
		t.Errorf("task-1's rows holding the secret: %d, %v, want none", count, err)
	}
}

// An old blob that won't delete once the redaction commits still holds
// the secret, which no row does any more: the next RedactArchived of the
// task deletes it, though it finds nothing to redact, and keeps the blob
// the task's marker names. PreviewArchived, which writes nothing, leaves
// it.
func TestAnOldBlobThatWontDeleteGoesNextTime(t *testing.T) {
	s, disk, old := archivedWithRow(t, filepath.Join(t.TempDir(), "agentry.db"), acceptEvery, events.TaskRenamed, holdingSecret)
	stuck := refusingRemove{disk, func(hash string) bool { return hash == old.Text("archive") }}
	_, rejection, err := s.RedactArchived("task-1", secret, you, "", summary, stuck)
	if want := "removing the blob " + old.Text("archive") + ": the disk is read-only"; rejection != nil || err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("redacting: %v, %v, want it to say %q", rejection, err, want)
	}
	marker := stillArchived(t, s, "task-1")
	if !disk.Has(old.Text("archive")) {
		t.Fatal("the old blob is gone")
	}

	if count, rejection, err := s.PreviewArchived("task-1", secret, disk); count != 0 || err != nil || rejection == nil || rejection.Reason != "no row of task-1 holds the secret" {
		t.Errorf("PreviewArchived: %d, %v, %v, want no row holding it", count, rejection, err)
	}
	if !disk.Has(old.Text("archive")) {
		t.Fatal("PreviewArchived deleted the old blob")
	}
	_, rejection, err = s.RedactArchived("task-1", secret, you, "", summary, disk)
	if want := "no row of task-1 holds the secret"; err != nil || rejection == nil || rejection.Reason != want {
		t.Errorf("redacting again: %v, %v, want %q", rejection, err, want)
	}
	if got, want := blobsIn(t, disk), []string{marker.Text("archive")}; !reflect.DeepEqual(got, want) {
		t.Errorf("the blobs on disk are %v, want the marker's %v alone", got, want)
	}
}

// PreviewArchived counts the rows RedactArchived would rewrite, read
// through the marker's blob, and writes nothing: the same rows and blobs.
func TestPreviewArchivedCountsTheRowsInTheBlob(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, blobs, _ := archivedWithRow(t, path, acceptEvery, events.TaskRenamed, holdingSecret)
	before, kept := rowsAt(t, path), blobsIn(t, blobs)
	if count, rejection, err := s.Preview("task-1", secret); count != 0 || err != nil || rejection == nil || rejection.Reason != ArchivedRedactionRefusal("task-1") {
		t.Errorf("Preview of an archived task: %d, %v, %v, want refused as archived", count, rejection, err)
	}
	count, rejection, err := s.PreviewArchived("task-1", secret, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if after := rowsAt(t, path); !reflect.DeepEqual(after, before) || !reflect.DeepEqual(blobsIn(t, blobs), kept) {
		t.Errorf("PreviewArchived changed the log or the blobs:\n%v\n%v", before, after)
	}
	redaction, rejection, err := s.RedactArchived("task-1", secret, you, "", summary, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if positions := redaction.Numbers("positions"); count != 1 || len(positions) != count {
		t.Errorf("PreviewArchived counted %d rows, RedactArchived rewrote %v; want the one row", count, positions)
	}
}

// The task.unarchived that RedactArchived adds, and then redacts, holds
// the marker's position and blob, so PreviewArchived counts it and both
// refuse it before the unarchiving: a secret in the position is left in a
// number, and nothing is written.
func TestPreviewArchivedSeesTheRowTheUnarchivingAdds(t *testing.T) {
	t.Run("a secret in the marker's position", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "agentry.db")
		s, blobs, marker := archivedWithRow(t, path, acceptEvery, events.TaskRenamed, `{"title":"Use code 93"}`)
		inPosition := strconv.FormatInt(marker.Position, 10)
		before := rowsAt(t, path)
		want := "the row at 5 holds the secret outside its text, as in a number, which redacting can't rewrite: erase the task, or redact a longer piece of the text"
		if count, rejection, err := s.PreviewArchived("task-1", inPosition, blobs); count != 0 || err != nil || rejection == nil || rejection.Reason != want {
			t.Errorf("PreviewArchived: %d, %v, %v, want %q", count, rejection, err, want)
		}
		if _, rejection, err := s.RedactArchived("task-1", inPosition, you, "", summary, blobs); err != nil || rejection == nil || rejection.Reason != want {
			t.Errorf("RedactArchived: %v, %v, want %q", rejection, err, want)
		}
		if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
			t.Errorf("a refused redaction changed the log:\n%v\n%v", before, after)
		}
	})
	t.Run("a secret in the marker's blob", func(t *testing.T) {
		s, blobs, marker := archivedWithRow(t, filepath.Join(t.TempDir(), "agentry.db"), acceptEvery, events.TaskRenamed, `{"title":"Plain"}`)
		inBlob := marker.Text("archive")[:12]
		count, rejection, err := s.PreviewArchived("task-1", inBlob, blobs)
		if rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
		redaction, rejection, err := s.RedactArchived("task-1", inBlob, you, "", summary, blobs)
		if rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
		if positions := redaction.Numbers("positions"); count != 2 || len(positions) != count {
			t.Errorf("PreviewArchived counted %d rows, RedactArchived rewrote %v; want the marker and the task.unarchived", count, positions)
		}
	})
}
