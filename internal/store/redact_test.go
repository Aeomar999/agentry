package store

import (
	"database/sql"
	"encoding/json"
	"maps"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// The schema's tests write the redaction and run the updates themselves,
// as erase_test.go does for the erasures. Redact's own follow them; the
// ones that need the rules and the state, and the search of the files'
// bytes, are in storetest's redact_test.go.

// redactedAt is an event.redacted naming positions of stream.
func redactedAt(stream string, positions ...int64) events.Proposal {
	fields := [][]string{}
	for range positions {
		fields = append(fields, []string{"$.title"})
	}
	return events.Proposal{Stream: stream, Actor: redactor, Type: events.EventRedacted, Data: map[string]any{
		"positions": positions, "fields": fields, "removed": 6, "who": "human:you", "why": "A token pasted by mistake."}}
}

// updateIn sets the data of the row at position, or another column when
// column isn't data, and says how many rows changed, or the error.
func updateIn(tx *sql.Tx, column string, position int64) (int64, error) {
	result, err := tx.Exec(`UPDATE events SET `+column+` = '{"title":"[redacted]"}' WHERE position = ?`, position)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// A redaction allows updating exactly the data of the rows it names, in
// its own transaction before it commits: no other column, no row it
// doesn't name, no row of its stream after it, nor a row another stream's
// redaction names. A stray update is still refused after it.
func TestARedactionAllowsUpdatingOnlyTheDataOfTheRowsItNames(t *testing.T) {
	s := openTestStore(t)
	acceptAll(t, s, newTask("task-1"), newTask("task-2"), renamed("task-1"), renamed("task-2"))
	// task-1 is at 1 and 3, task-2 at 2 and 4.
	inTransaction(t, s, func(tx *sql.Tx) {
		if _, err := updateIn(tx, "data", 1); !refusedAsAppendOnly(err) {
			t.Errorf("before the redaction: %v, want refused", err)
		}
		appendIn(t, tx, redactedAt("task-1", 1, 6)) // at 5
		appendIn(t, tx, renamed("task-1"))          // at 6, after the redaction that names it
		appendIn(t, tx, redactedAt("task-2", 3))    // at 7, naming task-1's row
		if updated, err := updateIn(tx, "data", 1); err != nil || updated != 1 {
			t.Errorf("the data of a row the redaction names: %d updated, %v", updated, err)
		}
		for _, column := range []string{"at", "stream", "actor_kind", "actor_id", "type", "correlation"} {
			if _, err := updateIn(tx, column, 1); !refusedAsAppendOnly(err) {
				t.Errorf("updating %s of a row the redaction names: %v, want refused", column, err)
			}
		}
		if _, err := tx.Exec(`UPDATE events SET position = 99 WHERE position = 1`); !refusedAsAppendOnly(err) {
			t.Errorf("updating the position of a row the redaction names: %v, want refused", err)
		}
		for _, stray := range []struct {
			name     string
			position int64
		}{
			{"a row of its stream it doesn't name", 3},
			{"a row of another stream", 2},
			{"a row it names after it", 6},
		} {
			if _, err := updateIn(tx, "data", stray.position); !refusedAsAppendOnly(err) {
				t.Errorf("%s: %v, want refused", stray.name, err)
			}
		}
	})
	if _, err := s.db.Exec(`UPDATE events SET data = '{}'`); !refusedAsAppendOnly(err) {
		t.Errorf("a stray update after the redaction: %v, want refused", err)
	}
}

// A log at version 2, with erasing's trigger, migrates to redacting: every
// row as it was, the update trigger gated, and the delete trigger's cases
// kept. Opening it again runs nothing.
func TestAVersionTwoLogMigratesToRedacting(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	execOn(t, path, versionZero+migrations[0]+";"+migrations[1]+`; PRAGMA user_version = 2;`)
	before := rowsAt(t, path)
	s, _ := openAt(t, path)
	acceptAll(t, s, renamed("task-2"))
	inTransaction(t, s, func(tx *sql.Tx) {
		appendIn(t, tx, redactedAt("task-1", 2))
		if updated, err := updateIn(tx, "data", 2); err != nil || updated != 1 {
			t.Errorf("a row the redaction names: %d updated, %v", updated, err)
		}
		if _, err := updateIn(tx, "data", 1); !refusedAsAppendOnly(err) {
			t.Errorf("a row it doesn't name: %v, want refused", err)
		}
	})
	s.Close()
	if after := rowsAt(t, path)[:len(before)]; !reflect.DeepEqual(after[0], before[0]) || !reflect.DeepEqual(after[2], before[2]) {
		t.Errorf("the rows before the migration changed:\n%v\n%v", before, after)
	}
	version, _, deleting := schemaOf(t, path)
	if version != len(migrations) || !strings.Contains(deleting, "task.archived") || !strings.Contains(deleting, "task.erased") {
		t.Errorf("version %d, delete trigger %s; want %d and both cases", version, deleting, len(migrations))
	}
	if updating := updateTrigger(t, path); !strings.Contains(updating, "event.redacted") {
		t.Errorf("update trigger %s; want the redaction's case", updating)
	}

	// Had the migration run again, it would have put back the gated trigger.
	sentinel := `CREATE TRIGGER events_no_update BEFORE UPDATE ON events
	BEGIN SELECT RAISE(ABORT, 'events are append-only: sentinel'); END`
	execOn(t, path, `DROP TRIGGER events_no_update; `+sentinel)
	again, _ := openAt(t, path)
	again.Close()
	if updating := updateTrigger(t, path); updating != sentinel {
		t.Errorf("after a second open: update trigger %s; the migration ran again", updating)
	}
}

// updateTrigger is the update trigger's SQL in the file at path.
func updateTrigger(t *testing.T, path string) string {
	t.Helper()
	db, err := sql.Open(Driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var trigger string
	if err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'trigger' AND name = 'events_no_update'`).Scan(&trigger); err != nil {
		t.Fatal(err)
	}
	return trigger
}

// rowsAt is every row of the file at path, every column, in order.
func rowsAt(t *testing.T, path string) []row {
	t.Helper()
	db, err := sql.Open(Driver, path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	found, err := db.Query(`SELECT ` + columns + ` FROM events ORDER BY position`)
	if err != nil {
		t.Fatal(err)
	}
	defer found.Close()
	var rows []row
	for found.Next() {
		var r row
		if err := found.Scan(&r.Position, &r.At, &r.Stream, &r.ActorKind, &r.ActorID, &r.Type, &r.Correlation, &r.Data); err != nil {
			t.Fatal(err)
		}
		rows = append(rows, r)
	}
	if err := found.Err(); err != nil {
		t.Fatal(err)
	}
	return rows
}

// secret is the text the tests redact.
const secret = "sk-9d2f41"

// openWithSecret opens the log at path with model and a check that
// refuses a second task.created quoting its title, as a rejection may
// quote what it refused, and fills it: task-1 created with the secret in
// its title and a label, and numbers JSON writes in more than one way, at
// 1; a rejection quoting the secret at 2; task-1 renamed at 3, and task-2
// holding the secret too at 4.
func openWithSecret(t *testing.T, path string, model Model) *Store {
	t.Helper()
	made := map[string]bool{}
	quoting := func(p events.Proposal) *events.Rejection {
		if p.Type == events.TaskCreated && made[p.Stream] {
			return &events.Rejection{Check: "order", Reason: "already exists: " + p.Text("title")}
		}
		made[p.Stream] = p.Type == events.TaskCreated
		return nil
	}
	s, err := Open(path, withNotes, model, quoting)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	titled := func(p events.Proposal, title string) events.Proposal {
		p.Data = map[string]any{"title": title, "repo": "/src/app", "coordinator": "tech-lead",
			"labels": []string{"keep", "x " + secret}, "budget": 1e21, "small": 1e-7, "whole": int64(1) << 60}
		return p
	}
	acceptAll(t, s, titled(newTask("task-1"), "Use "+secret+" and "+secret+" <again>"))
	if _, rejection, err := s.Propose(titled(newTask("task-1"), "Again "+secret)); rejection == nil || err != nil {
		t.Fatal("a second task.created should be refused:", rejection, err)
	}
	acceptAll(t, s, renamed("task-1"), titled(newTask("task-2"), secret))
	return s
}

// Redact rewrites only the rows of the stream that hold the secret: every
// column but data as it was, and data byte for byte as it was but for each
// copy of the secret, now [redacted]. Its redaction names those rows and
// fields and how much went, never the secret.
func TestRedactRewritesOnlyTheSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s := openWithSecret(t, path, newCreated())
	before := rowsAt(t, path)

	redacted, rejection, err := s.Redact("task-1", secret, you, "A token pasted by mistake.")
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	after := rowsAt(t, path)
	for i, was := range before {
		want := was
		if was.Position <= 2 {
			want.Data = strings.ReplaceAll(was.Data, secret, Redacted)
		}
		if after[i] != want {
			t.Errorf("the row at %d:\n got %+v\nwant %+v", was.Position, after[i], want)
		}
	}
	if positions := redacted.Data["positions"]; !reflect.DeepEqual(positions, []any{1.0, 2.0}) {
		t.Errorf("the redaction names %v, want 1 and 2", positions)
	}
	wantFields := []any{[]any{"$.labels[1]", "$.title"}, []any{"$.reason"}}
	if fields := redacted.Data["fields"]; !reflect.DeepEqual(fields, wantFields) {
		t.Errorf("the redaction's fields %v, want %v", fields, wantFields)
	}
	if removed := redacted.Number("removed"); removed != 4*len(secret) {
		t.Errorf("the redaction removed %d bytes, want %d", removed, 4*len(secret))
	}
	if redacted.Actor != redactor || redacted.Text("who") != "human:you" || redacted.Text("why") != "A token pasted by mistake." {
		t.Errorf("the redaction: %+v", redacted)
	}
	encoded, _ := json.Marshal(redacted)
	if strings.Contains(string(encoded), secret) || strings.Contains(string(encoded), events.Hash(secret)) {
		t.Errorf("the redaction holds the secret or its hash: %s", encoded)
	}
	if s.LastPosition() != redacted.Position {
		t.Errorf("the store is at %d, want the redaction's %d", s.LastPosition(), redacted.Position)
	}
}

// A secret with what JSON escapes in it, quotes, a backslash, a newline
// and text beyond ASCII, is found as it was written, and every row it was
// in is still valid JSON with the same keys.
func TestASecretWithJSONsSpecialCharactersIsRedacted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	const special = "p\"a\\ss\nwörd ✓ <&>"
	acceptAll(t, s, events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": "Log in with " + special + ".", "repo": "/src/app", "coordinator": "tech-lead", "steps": []any{map[string]any{"text": special}}}})
	if count, rejection, err := s.Preview("task-1", special); count != 1 || rejection != nil || err != nil {
		t.Fatalf("the rows holding it: %d, %v, want 1", count, err)
	}
	if _, rejection, err := s.Redact("task-1", special, you, ""); rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	rows := rowsAt(t, path)
	var data map[string]any
	if err := json.Unmarshal([]byte(rows[0].Data), &data); err != nil {
		t.Fatalf("the rewritten row isn't valid JSON: %v\n%s", err, rows[0].Data)
	}
	want := map[string]any{"title": "Log in with [redacted].", "repo": "/src/app", "coordinator": "tech-lead", "steps": []any{map[string]any{"text": Redacted}}}
	if !reflect.DeepEqual(data, want) {
		t.Errorf("the rewritten data: %v, want %v", data, want)
	}
	if fields := rows[1].Data; !strings.Contains(fields, `"$.steps[0].text","$.title"`) {
		t.Errorf("the redaction's fields: %s", fields)
	}
	if count, _, err := s.Preview("task-1", special); count != 0 || err != nil {
		t.Errorf("the rows holding it after: %d, %v, want none", count, err)
	}
}

// Redact refuses, and writes nothing, when no row holds the secret, the
// reason given holds it, the secret is empty or part of [redacted], or the
// stream took an event since its rows were read. Preview gives the same
// rejection, but for the reason, which it isn't given.
func TestARefusedRedactionWritesNothing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s := openWithSecret(t, path, newCreated())
	for _, c := range []struct {
		name, stream, secret, why, want string
	}{
		{"no row holds it", "task-1", "not-there", "", "no row of task-1 holds the secret"},
		{"the reason holds it", "task-1", secret, "Remove " + secret + ".", "the reason holds the secret, which the log would then keep"},
		{"an empty secret", "task-1", "", "", "a secret to redact is some text, and not part of [redacted]"},
		{"part of [redacted]", "task-1", "dact", "", "a secret to redact is some text, and not part of [redacted]"},
	} {
		if _, rejection, err := s.Redact(c.stream, c.secret, you, c.why); err != nil || rejection == nil || rejection.Reason != c.want {
			t.Errorf("%s: %v, %v, want %q", c.name, rejection, err, c.want)
		}
		if c.why != "" {
			continue
		}
		if count, rejection, err := s.Preview(c.stream, c.secret); count != 0 || err != nil || rejection == nil || rejection.Reason != c.want {
			t.Errorf("%s: Preview %d, %v, %v, want %q", c.name, count, rejection, err, c.want)
		}
	}

	found, rejection, err := s.find("task-1", secret, you, "")
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	acceptAll(t, s, renamed("task-1"))
	before := rowsAt(t, path)
	if _, rejection, err := s.rewrite(found); err != nil || rejection == nil || !strings.Contains(rejection.Reason, "took an event since its rows were read") {
		t.Errorf("the stream moved: %v, %v, want refused", rejection, err)
	}
	if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
		t.Errorf("a refused redaction changed the log:\n%v\n%v", before, after)
	}
}

// Preview says what Redact would do, and writes nothing: the number of rows
// it would rewrite, or the rules' rejection and none.
func TestPreviewSaysWhatRedactWould(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s := openWithSecret(t, path, newCreated())
	quoting := s.check
	s.check = refusing(new(true))
	before, last := rowsAt(t, path), s.LastPosition()
	if count, rejection, err := s.Preview("task-1", secret); count != 0 || err != nil || rejection == nil || rejection.Reason != "no redaction now" {
		t.Errorf("Preview with the rules refusing: %d, %v, %v, want refused", count, rejection, err)
	}
	s.check = quoting
	count, rejection, err := s.Preview("task-1", secret)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if after := rowsAt(t, path); !reflect.DeepEqual(after, before) || s.LastPosition() != last {
		t.Errorf("Preview changed the log:\n%v\n%v", before, after)
	}
	redacted, rejection, err := s.Redact("task-1", secret, you, "")
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if positions := redacted.Numbers("positions"); count != len(positions) {
		t.Errorf("Preview counted %d rows, Redact rewrote %v", count, positions)
	}
}

// A secret that overlaps an end of [redacted] would still show next to it,
// as "]a" in "x ]aa y" becomes "x [redacted]a y": Redact refuses it, and
// the log and the state are as they were.
func TestASecretLeftNextToRedactedIsRefused(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, model := openAt(t, path)
	acceptAll(t, s, newTask("task-1"), events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "x ]aa y"}})
	before, applied := rowsAt(t, path), model.applied
	_, rejection, err := s.Redact("task-1", "]a", you, "")
	if err != nil || rejection == nil || rejection.Reason != "the row at 2 would still show the secret next to [redacted]" {
		t.Errorf("redacting ]a: %v, %v, want refused", rejection, err)
	}
	if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
		t.Errorf("a refused redaction changed the log:\n%v\n%v", before, after)
	}
	if model.applied != applied || s.LastPosition() != 2 {
		t.Errorf("a refused redaction reached the state: %d events given, want %d, at %d", model.applied, applied, s.LastPosition())
	}
}

// A row whose string holding the secret isn't escaped as appendEvent
// escapes, a < not escaped or a / escaped, would be rewritten with more
// changed than the secret: Redact refuses it, naming its position, and
// writes nothing.
func TestARowNotWrittenAsTheLogWritesIsNotRedacted(t *testing.T) {
	for name, data := range map[string]string{
		"a < not escaped": `{"coordinator":"tech-lead","repo":"/src/app","title":"<Use ` + secret + `>"}`,
		"a / escaped":     `{"title":"Use ` + secret + ` in \/src","repo":"/src/app","coordinator":"tech-lead"}`,
	} {
		path := filepath.Join(t.TempDir(), "agentry.db")
		s, _ := openAt(t, path)
		s.Close()
		execOn(t, path, `INSERT INTO events (at, stream, actor_kind, actor_id, type, data) VALUES
			('2026-10-19T09:00:00Z', 'task-1', 'human', 'you', 'task.created', '`+data+`')`)
		s, _ = openAt(t, path)
		before := rowsAt(t, path)
		_, rejection, err := s.Redact("task-1", secret, you, "")
		if err != nil || rejection == nil || !strings.HasPrefix(rejection.Reason, "the row at 1 isn't written as the log writes data") {
			t.Errorf("%s: %v, %v, want refused", name, rejection, err)
		}
		if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
			t.Errorf("%s: a refused redaction changed the log:\n%v\n%v", name, before, after)
		}
	}
}

// A row with a key that holds the secret is refused, with its data as it
// was and no fields, whether or not a value holds it too: redacting
// doesn't rewrite keys, so it would leave the secret in the row and in the
// redaction's paths.
func TestAKeyHoldingTheSecretIsRefused(t *testing.T) {
	for name, data := range map[string]string{
		"and a value":  `{"a":{"` + secret + `X":"x ` + secret + `"}}`,
		"only the key": `{"a":{"` + secret + `X":"x"}}`,
	} {
		rw, err := redactedData(data, secret)
		if err != nil || rw.refused != "holds the secret in a key, which redacting doesn't rewrite" || !rw.inKey || !rw.holds() {
			t.Errorf("%s: refused %q, %v, want refused", name, rw.refused, err)
		}
		if rw.data != data || len(rw.fields) != 0 {
			t.Errorf("%s: data %s fields %v, want the data as it was and no fields", name, rw.data, rw.fields)
		}
	}
}

// Redact refuses a stream with a row whose key holds the secret, naming its
// position, and writes nothing, also when no value holds it; Preview
// gives the same rejection.
func TestRedactRefusesARowWithTheSecretInAKey(t *testing.T) {
	for name, data := range map[string]string{
		"and a value":  `{"coordinator":"tech-lead","repo":{"` + secret + `":"/src/` + secret + `"},"title":"Use it"}`,
		"only the key": `{"coordinator":"tech-lead","repo":{"` + secret + `":"/src/app"},"title":"Use it"}`,
	} {
		path := filepath.Join(t.TempDir(), "agentry.db")
		s, _ := openAt(t, path)
		s.Close()
		execOn(t, path, `INSERT INTO events (at, stream, actor_kind, actor_id, type, data) VALUES
			('2026-10-19T09:00:00Z', 'task-1', 'human', 'you', 'task.created', '`+data+`')`)
		s, _ = openAt(t, path)
		want := "the row at 1 holds the secret in a key, which redacting doesn't rewrite"
		if count, rejection, err := s.Preview("task-1", secret); count != 0 || err != nil || rejection == nil || rejection.Reason != want {
			t.Errorf("%s: Preview %d, %v, %v, want %q", name, count, rejection, err, want)
		}
		before := rowsAt(t, path)
		_, rejection, err := s.Redact("task-1", secret, you, "")
		if err != nil || rejection == nil || rejection.Reason != "the row at 1 holds the secret in a key, which redacting doesn't rewrite" {
			t.Errorf("%s: %v, %v, want refused", name, rejection, err)
		}
		if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
			t.Errorf("%s: a refused redaction changed the log:\n%v\n%v", name, before, after)
		}
	}
}

// An archived task and an erased one are refused for that before anything
// their live rows hold: a row after the marker, which only a crash or a
// store with no rules leaves, holding the secret in a key or a value, or
// no row holding it. Preview gives the same rejection, and nothing is
// written.
func TestAnArchivedOrErasedStreamIsRefusedBeforeItsRows(t *testing.T) {
	inAKey := events.Proposal{Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Use it", "extra": map[string]any{secret: "x"}}}
	inAValue := events.Proposal{Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Use " + secret}}
	for _, c := range []struct {
		name   string
		end    func(s *Store, blobs sharedfiles.Disk) (*events.Rejection, error)
		leaves *events.Proposal
		want   string
	}{
		{"archived, a key holds it", archiving, &inAKey, "task-1 is archived: unarchive it, redact it, then archive it again"},
		{"archived, a value holds it", archiving, &inAValue, "task-1 is archived: unarchive it, redact it, then archive it again"},
		{"archived, no row holds it", archiving, nil, "task-1 is archived: unarchive it, redact it, then archive it again"},
		{"erased, a key holds it", erasing, &inAKey, "task-1 is erased: there is nothing left to redact"},
		{"erased, a value holds it", erasing, &inAValue, "task-1 is erased: there is nothing left to redact"},
		{"erased, no row holds it", erasing, nil, "task-1 is erased: there is nothing left to redact"},
	} {
		path := filepath.Join(t.TempDir(), "agentry.db")
		s, _ := openAt(t, path)
		acceptAll(t, s, newTask("task-1"))
		if rejection, err := c.end(s, sharedfiles.Disk{Dir: t.TempDir()}); rejection != nil || err != nil {
			t.Fatal(c.name, rejection, err)
		}
		if c.leaves != nil {
			left := *c.leaves
			left.Stream = "task-1"
			acceptAll(t, s, left)
		}
		before := rowsAt(t, path)
		if count, rejection, err := s.Preview("task-1", secret); count != 0 || err != nil || rejection == nil || rejection.Reason != c.want {
			t.Errorf("%s: Preview %d, %v, %v, want %q", c.name, count, rejection, err, c.want)
		}
		if _, rejection, err := s.Redact("task-1", secret, you, ""); err != nil || rejection == nil || rejection.Reason != c.want {
			t.Errorf("%s: %v, %v, want %q", c.name, rejection, err, c.want)
		}
		if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
			t.Errorf("%s: a refused redaction changed the log:\n%v\n%v", c.name, before, after)
		}
	}
}

func archiving(s *Store, blobs sharedfiles.Disk) (*events.Rejection, error) {
	_, rejection, err := s.Archive("task-1", summary, blobs)
	return rejection, err
}

func erasing(s *Store, blobs sharedfiles.Disk) (*events.Rejection, error) {
	_, rejection, err := s.Erase("task-1", you, "", blobs)
	return rejection, err
}

// A string holding the secret that json.Marshal would write otherwise, a
// < not escaped, a / or an A escaped, or a raw U+2028, is refused.
func TestAStringEscapedOtherwiseIsRefused(t *testing.T) {
	for name, raw := range map[string]string{
		"a < not escaped": `"<` + secret + `"`,
		"a / escaped":     `"\/` + secret + `"`,
		"an A escaped":    `"\u0041` + secret + `"`,
		"a raw U+2028":    "\"\u2028" + secret + `"`,
	} {
		rw, err := redactedData(`{"title":`+raw+`}`, secret)
		if err != nil || !strings.HasPrefix(rw.refused, "isn't written as the log writes data") {
			t.Errorf("%s: refused %q, %v", name, rw.refused, err)
		}
	}
}

// pin is a secret of digits alone, which a row may also hold as a number.
const pin = "90210417"

// A row that would still hold the secret once its strings are rewritten,
// in a number beside a string holding it or in a number alone, is refused,
// by Preview too, and nothing is written: the secret would stay in the
// log while the redaction said it had gone.
func TestASecretLeftInANumberIsRefused(t *testing.T) {
	for name, data := range map[string]map[string]any{
		"beside a string": {"title": "PIN " + pin, "extra": 90210417},
		"alone":           {"title": "PIN", "extra": 90210417},
	} {
		path := filepath.Join(t.TempDir(), "agentry.db")
		s, _ := openAt(t, path)
		created := newTask("task-1")
		maps.Copy(created.Data, data)
		acceptAll(t, s, created)
		before := rowsAt(t, path)
		want := "the row at 1 holds the secret outside its text, as in a number, which redacting can't rewrite: erase the task, or redact a longer piece of the text"
		if count, rejection, err := s.Preview("task-1", pin); count != 0 || err != nil || rejection == nil || rejection.Reason != want {
			t.Errorf("%s: Preview %d, %v, %v, want %q", name, count, rejection, err, want)
		}
		if _, rejection, err := s.Redact("task-1", pin, you, ""); err != nil || rejection == nil || rejection.Reason != want {
			t.Errorf("%s: %v, %v, want %q", name, rejection, err, want)
		}
		if after := rowsAt(t, path); !reflect.DeepEqual(after, before) {
			t.Errorf("%s: a refused redaction changed the log:\n%v\n%v", name, before, after)
		}
	}
}

// Propose refuses event.redacted from anyone: only Redact writes it, with
// its rows.
func TestOnlyRedactWritesARedaction(t *testing.T) {
	s := openTestStore(t)
	acceptAll(t, s, newTask("task-1"))
	for _, actor := range []events.Actor{you, redactor} {
		p := redactedAt("task-1", 1)
		p.Actor = actor
		if _, rejection, err := s.Propose(p); err != nil || rejection == nil || rejection.Check != "who" {
			t.Errorf("event.redacted by %s: %v, %v, want refused by the who check", actor, rejection, err)
		}
	}
}

// The model refolds the stream from the rows as Redact rewrote them, never
// as they were.
func TestTheModelRefoldsTheRewrittenRows(t *testing.T) {
	model := &refolds{}
	s := openWithSecret(t, filepath.Join(t.TempDir(), "agentry.db"), model)
	redacted, rejection, err := s.Redact("task-1", secret, you, "")
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if model.stream != "task-1" || model.e.Position != redacted.Position || len(model.before) != 2 {
		t.Fatalf("refolded %s with %d events before %d", model.stream, len(model.before), model.e.Position)
	}
	if title := model.before[0].Text("title"); title != "Use [redacted] and [redacted] <again>" {
		t.Errorf("the model was given the title %q", title)
	}
}

// refolds is a stand-in model that keeps the last refold it was given.
type refolds struct {
	stream string
	before []events.Event
	e      events.Event
}

func (r *refolds) Since() int64                { return 0 }
func (r *refolds) Load(string, []events.Event) {}
func (r *refolds) Loaded(int64)                {}
func (r *refolds) Apply(events.Event)          {}
func (r *refolds) Refold(stream string, before []events.Event, e events.Event) {
	r.stream, r.before, r.e = stream, before, e
}

// Data written from a struct has its keys in the struct's order, not
// sorted, as a task.archived marker's summary does: Redact rewrites the
// string that holds the secret and leaves every other byte as it was. The
// marker stays in the stream once it is unarchived, so this is how a
// secret in an archived task's title goes.
func TestAnArchivedMarkersSummaryIsRedacted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	fill(t, s)
	summarise := func() any {
		return tasks.Summary{ID: "task-1", Title: "Use " + secret + " <now>", Repo: "/src/app", Status: tasks.Done, Labels: []string{"keep"}, Rank: 1.5}
	}
	blobs := sharedfiles.Disk{Dir: t.TempDir()}
	marker, rejection, err := s.Archive("task-1", summarise, blobs)
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	unarchive(t, s, "task-1", blobs)
	was := rowAt(t, path, marker.Position)
	if !strings.Contains(was.Data, `"summary":{"id":"task-1","title":"Use `+secret) {
		t.Fatalf("the marker's summary should be in the struct's order: %s", was.Data)
	}

	redacted, rejection, err := s.Redact("task-1", secret, you, "")
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	want := was
	want.Data = strings.Replace(was.Data, secret, Redacted, 1)
	if now := rowAt(t, path, marker.Position); now != want || !json.Valid([]byte(now.Data)) {
		t.Errorf("the marker:\n got %+v\nwant %+v", now, want)
	}
	if fields := redacted.Data["fields"]; !reflect.DeepEqual(fields, []any{[]any{"$.summary.title"}}) {
		t.Errorf("the redaction's fields %v, want the summary's title", fields)
	}
}

// A template.saved row holds its agents as structs, prompts and all: one
// whose prompt holds the secret is redacted the same way.
func TestASavedTemplatesPromptIsRedacted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	saved := agents.UserTemplate{ID: "mine", Team: "Web", Agents: []agents.Agent{
		{Name: "planner", Prompt: "Plan."}, {Name: "engineer", Prompt: "Deploy with " + secret + " & <care>.", Engine: "claude"}}}
	acceptAll(t, s, saved.Save(you))
	was := rowsAt(t, path)[0]

	redacted, rejection, err := s.Redact(events.TemplatesStream, secret, you, "")
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	want := was
	want.Data = strings.Replace(was.Data, secret, Redacted, 1)
	if now := rowsAt(t, path)[0]; now != want {
		t.Errorf("the template:\n got %+v\nwant %+v", now, want)
	}
	if fields := redacted.Data["fields"]; !reflect.DeepEqual(fields, []any{[]any{"$.agents[1].prompt"}}) {
		t.Errorf("the redaction's fields %v, want the engineer's prompt", fields)
	}
}

// A secret with what JSON escapes in it is found in a struct's row, and
// its escaped bytes are what go: a quote, a backslash, a < and a newline.
func TestASecretJSONEscapesGoesFromAStructsRow(t *testing.T) {
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, _ := openAt(t, path)
	const special = "pa\"ss\\w<rd\n"
	saved := agents.UserTemplate{ID: "mine", Team: "Web", Agents: []agents.Agent{{Name: "engineer", Prompt: "Log in with " + special + " here."}}}
	acceptAll(t, s, saved.Save(you))
	was := rowsAt(t, path)[0]
	escaped := strings.Trim(encoded(special), `"`)
	if !strings.Contains(was.Data, `Log in with `+escaped+` here.`) {
		t.Fatalf("the row should hold the secret escaped as %s: %s", escaped, was.Data)
	}

	redacted, rejection, err := s.Redact(events.TemplatesStream, special, you, "")
	if rejection != nil || err != nil {
		t.Fatal(rejection, err)
	}
	if now, want := rowsAt(t, path)[0].Data, strings.Replace(was.Data, escaped, Redacted, 1); now != want {
		t.Errorf("the template:\n got %s\nwant %s", now, want)
	}
	if removed := redacted.Number("removed"); removed != len(special) {
		t.Errorf("the redaction removed %d bytes, want the secret's %d", removed, len(special))
	}
}

// rowAt is the row at position in the file at path.
func rowAt(t *testing.T, path string, position int64) row {
	t.Helper()
	for _, r := range rowsAt(t, path) {
		if r.Position == position {
			return r
		}
	}
	t.Fatalf("no row at %d", position)
	return row{}
}
