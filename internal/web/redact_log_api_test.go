package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// Redacting a secret pasted widely, across the whole log
// (docs/design/log-growth.md section 5, "Removing a secret"). Each
// stream's redaction is the per-stream routes' own, tested in
// redact_api_test.go; these see what the log-wide routes add: which
// streams they read, the snapshot written once, and a stream refused
// leaving the rest redacted.

// widely has secretHere in task-4 (two rows, withSecret), task-5 (one),
// the archived task-6 (one, in its blob), the engineer's knowledge and
// idea-1, and is the old blob of task-6.
func widely(t *testing.T) (f *eraseFixture, oldBlob string) {
	t.Helper()
	f = withSecret(t)
	f.propose(t,
		events.Proposal{Stream: "task-5", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Rotate " + secretHere, "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-6", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Old", "description": "It was " + secretHere + ".",
			"repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-6", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "Done.", "moved": true}},
	)
	storetest.ArchiveIn(t, f.log, f.state, "task-6", f.blobs)
	f.teach(t, "# Notes\n\nThe key is "+secretHere+".")
	if _, rejection, err := f.log.Create(ideas.Prefix, events.Proposal{Actor: loop, Type: ideas.IdeaSuggested, Data: map[string]any{"title": "Vault",
		"problem": "The key " + secretHere + " is pasted around.", "proposal": "Keep keys in a vault.", "size": "S", "task": "task-1", "repo": "/r"}}); rejection != nil || err != nil {
		t.Fatalf("suggesting idea-1: %v, %v", rejection, err)
	}
	return f, markerOf(t, f, "task-6")
}

// previewAcross is the preview across the log of text, through the
// route at server.
func previewAcross(t *testing.T, server *httptest.Server, text string) logPreview {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text})
	code, answer := call(t, server, "POST", "/api/redact/preview", string(body))
	if code != http.StatusOK {
		t.Fatalf("previewing the log: %d %s", code, answer)
	}
	var preview logPreview
	if err := json.Unmarshal([]byte(answer), &preview); err != nil {
		t.Fatal(err)
	}
	return preview
}

// redactAcross redacts text across the log through the route at server,
// and is its answer.
func redactAcross(t *testing.T, server *httptest.Server, text string) logRedacted {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"text": text, "why": "Pasted everywhere."})
	code, answer := call(t, server, "POST", "/api/redact", string(body))
	if code != http.StatusOK {
		t.Fatalf("redacting the log: %d %s", code, answer)
	}
	var got logRedacted
	if err := json.Unmarshal([]byte(answer), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

// redactCounted asks the route at server to redact text across the log
// of the rows counted in each stream, and is its answer.
func redactCounted(t *testing.T, server *httptest.Server, text string, counts map[string]int) (int, string) {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"text": text, "why": "Pasted everywhere.", "counts": counts})
	return call(t, server, "POST", "/api/redact", string(body))
}

// countsOf is the rows the preview counted in each stream it listed.
func countsOf(preview logPreview) map[string]int {
	counts := map[string]int{}
	for _, listed := range preview.Streams {
		counts[listed.Stream] = listed.Rows
	}
	return counts
}

// matches fails the test unless the redaction rewrote the streams the
// preview listed, as many rows in each, just now.
func matches(t *testing.T, answer logRedacted, preview logPreview) {
	t.Helper()
	var listed, rewrote []string
	for _, p := range preview.Streams {
		listed = append(listed, fmt.Sprintf("%s: %d, archived %t", p.Stream, p.Rows, p.Archived))
	}
	for _, r := range answer.Streams {
		rewrote = append(rewrote, fmt.Sprintf("%s: %d, archived %t", r.Stream, r.Rows, r.Archived))
		if time.Since(r.At) > time.Minute {
			t.Errorf("%s was redacted at %s", r.Stream, r.At)
		}
	}
	if answer.Rows != preview.Rows || !slices.Equal(rewrote, listed) {
		t.Errorf("the redaction rewrote %d rows, %q; the preview listed %d, %q", answer.Rows, rewrote, preview.Rows, listed)
	}
}

// redactedStreams is every stream an event.redacted is in, in log order.
func redactedStreams(t *testing.T, s *store.Store) []string {
	t.Helper()
	all, err := s.After(0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	var streams []string
	for _, e := range all {
		if e.Type == events.EventRedacted {
			streams = append(streams, e.Stream)
		}
	}
	return streams
}

// The step's proof: a secret pasted into two live tasks, an archived one,
// an agent's knowledge and an idea is previewed in each, in task order
// and then the rest, and redacted from each, one event.redacted per
// stream, as many rows as the preview counted. Then no file holds it: no
// row, WAL, snapshot or blob. The archived task is archived again, its
// old blob gone.
func TestRedactingASecretAcrossTheLogLeavesNoTraceOnDisk(t *testing.T) {
	f, old := widely(t)
	f.holdsSecret(t, "state.snapshot", old)
	preview := previewAcross(t, f.server, secretHere)
	want := []streamPreview{{Stream: "task-4", Rows: 2}, {Stream: "task-5", Rows: 1}, {Stream: "task-6", Rows: 1, Archived: true},
		{Stream: events.AgentsStream, Rows: 1}, {Stream: "idea-1", Rows: 1}}
	if !slices.Equal(preview.Streams, want) || preview.Rows != 6 {
		t.Fatalf("the preview: %+v, want 6 rows in %+v", preview, want)
	}
	before := redactedStreams(t, f.log)

	answer := redactAcross(t, f.server, secretHere)
	matches(t, answer, preview)
	if len(answer.Warnings) != 0 {
		t.Errorf("the redaction warned %q", answer.Warnings)
	}
	if holding := storetest.HoldingAny(t, f.dir, secretHere); len(holding) != 0 {
		t.Errorf("after the redaction these files still hold the secret: %v", holding)
	}
	// task-6's event.redacted is archived with it, in its new blob.
	if got := redactedStreams(t, f.log)[len(before):]; !slices.Equal(got, []string{"task-4", "task-5", events.AgentsStream, "idea-1"}) {
		t.Errorf("the redaction wrote an event.redacted in %q", got)
	}
	for id, want := range map[string]string{"task-4": "Use the key [redacted] to deploy.", "task-5": "Rotate [redacted]"} {
		task, err := f.state.Task(id)
		if err != nil || task == nil || (task.Description != want && task.Title != want) {
			t.Errorf("%s after the redaction: %+v, %v; want %q", id, task, err, want)
		}
	}
	if _, err := f.state.Task("task-6"); !errors.Is(err, current.ErrArchived) {
		t.Errorf("task-6, redacted, should be archived again: %v", err)
	}
	if f.blobs.Has(old) {
		t.Errorf("task-6's old archive blob %s is still there", old)
	}
}

// What doesn't hold the secret is left as it is: an erased task isn't
// read, though its preview would be refused rather than hold none, and a
// stream without the secret gets no event.redacted. The snapshot, written
// once at the end, no longer holds the secret.
func TestARedactionAcrossTheLogLeavesWhatDoesNotHoldTheSecret(t *testing.T) {
	f := withSecret(t)
	f.erase(t, "task-2")
	f.holdsSecret(t, "state.snapshot")
	before := redactedStreams(t, f.log)
	preview := previewAcross(t, f.server, secretHere)
	if want := []streamPreview{{Stream: "task-4", Rows: 2}}; !slices.Equal(preview.Streams, want) {
		t.Fatalf("the preview listed %+v, want only %+v", preview.Streams, want)
	}
	matches(t, redactAcross(t, f.server, secretHere), preview)
	if got := redactedStreams(t, f.log)[len(before):]; !slices.Equal(got, []string{"task-4"}) {
		t.Errorf("the redaction wrote an event.redacted in %q, want only task-4", got)
	}
	if holding := storetest.HoldingAny(t, f.dir, secretHere); len(holding) != 0 {
		t.Errorf("after the redaction these files still hold the secret: %v", holding)
	}
}

// A stream refused, in its preview or once it is asked to redact, is
// named among the failures, and the others are redacted still. When every
// stream that holds the secret is refused, the redaction is refused 409
// with their reasons, as one stream's is.
func TestAStreamRefusedLeavesTheOthersRedacted(t *testing.T) {
	// The rules let task-1's redaction be previewed and refuse it once it
	// is asked for, with a reason, and refuse task-3's both times.
	server, s := newTestServerChecking(t, func(p events.Proposal) *events.Rejection {
		if p.Type != events.EventRedacted {
			return nil
		}
		if p.Stream == "task-3" || p.Stream == "task-1" && p.Data["why"] != "" {
			return &events.Rejection{Check: "order", Reason: p.Stream + " is kept as it is"}
		}
		return nil
	})
	for _, id := range []string{"task-1", "task-2", "task-3"} {
		if _, rejection, err := s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Use " + secretHere,
			"repo": "/r", "coordinator": "tech-lead"}}); rejection != nil || err != nil {
			t.Fatalf("creating %s: %v, %v", id, rejection, err)
		}
	}
	preview := previewAcross(t, server, secretHere)
	want := []streamPreview{{Stream: "task-1", Rows: 1}, {Stream: "task-2", Rows: 1}, {Stream: "task-3", Error: "task-3 is kept as it is"}}
	if !slices.Equal(preview.Streams, want) || preview.Rows != 2 {
		t.Fatalf("the preview: %+v, want 2 rows in %+v", preview, want)
	}
	answer := redactAcross(t, server, secretHere)
	if len(answer.Streams) != 1 || answer.Streams[0].Stream != "task-2" || answer.Rows != 1 {
		t.Errorf("the redaction rewrote %+v, want only task-2's row", answer)
	}
	failures := []streamFailed{{Stream: "task-1", Reason: "task-1 is kept as it is"}, {Stream: "task-3", Reason: "task-3 is kept as it is"}}
	if !slices.Equal(answer.Failures, failures) {
		t.Errorf("the redaction failed %+v, want %+v", answer.Failures, failures)
	}
	if len(answer.Warnings) != 0 {
		t.Errorf("the redaction warned %q, want its failures named only among the failures", answer.Warnings)
	}
	if got := redactedStreams(t, s); !slices.Equal(got, []string{"task-2"}) {
		t.Errorf("event.redacted in %q, want only task-2", got)
	}

	for _, id := range []string{"task-1", "task-3"} {
		if _, rejection, err := s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Use " + plantedHere}}); rejection != nil || err != nil {
			t.Fatalf("renaming %s: %v, %v", id, rejection, err)
		}
	}
	last := s.LastPosition()
	if preview := previewAcross(t, server, plantedHere); preview.Rows != 1 {
		t.Fatalf("the preview of a text task-1 and task-3 hold: %+v", preview)
	}
	code, body := call(t, server, "POST", "/api/redact", `{"text":"`+plantedHere+`","why":"Pasted."}`)
	if want := "task-1: task-1 is kept as it is; task-3: task-3 is kept as it is"; code != http.StatusConflict || strings.TrimSpace(body) != want {
		t.Errorf("redacting a text only refused streams hold: %d %q, want 409 %q", code, body, want)
	}
	if s.LastPosition() != last {
		t.Errorf("the refused redaction wrote up to %d from %d", s.LastPosition(), last)
	}
}

// When no stream that holds the secret is redacted and one of them
// failed, rather than was refused, the redaction fails 500 with every
// stream's reason, as one stream's error does, and writes nothing: here
// task-1 is refused once it is asked to redact, and the archived task-2's
// blob is lost.
func TestARedactionAcrossTheLogWithNoStreamRedactedAndOneFailedFails(t *testing.T) {
	server, s, state := newTestStateChecking(t, func(p events.Proposal) *events.Rejection {
		if p.Type == events.EventRedacted && p.Stream == "task-1" && p.Data["why"] != "" {
			return &events.Rejection{Check: "order", Reason: "task-1 is kept as it is"}
		}
		return nil
	})
	for _, id := range []string{"task-1", "task-2"} {
		if _, rejection, err := s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Use " + secretHere,
			"repo": "/r", "coordinator": "tech-lead"}}); rejection != nil || err != nil {
			t.Fatalf("creating %s: %v, %v", id, rejection, err)
		}
	}
	storetest.Archive(t, s, state, "task-2") // its blob in a folder the server doesn't read
	if preview := previewAcross(t, server, secretHere); preview.Rows != 1 || len(preview.Streams) != 2 || preview.Streams[1].Error == "" {
		t.Fatalf("the preview: %+v, want task-1's row and task-2 failing", preview)
	}
	last := s.LastPosition()
	code, body := call(t, server, "POST", "/api/redact", `{"text":"`+secretHere+`","why":"Pasted."}`)
	if want := "task-1: task-1 is kept as it is; task-2: task-2's archived rows can't be read: its blob is missing"; code != http.StatusInternalServerError || strings.TrimSpace(body) != want {
		t.Errorf("redacting with task-1 refused and task-2 failing: %d %q, want 500 naming both", code, body)
	}
	if s.LastPosition() != last {
		t.Errorf("the failed redaction wrote up to %d from %d", s.LastPosition(), last)
	}
}

// A turn running as a secret in the agents' knowledge is redacted across
// the log doesn't write it back, as for the agents' own route
// (TestATurnRunningAsTheAgentsAreRedactedDoesNotWriteTheSecretBack): the
// answer says the turn's end redacts it again, and it does.
func TestATurnRunningAsTheLogIsRedactedDoesNotWriteTheSecretBack(t *testing.T) {
	f := newEraseFixture(t)
	written := "# Notes\n\nThe key is " + secretHere + "."
	f.teach(t, written)
	f.startTurn(t)
	answer := redactAcross(t, f.server, secretHere)
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
	if holding := storetest.HoldingAny(t, f.dir, secretHere); len(holding) != 0 {
		t.Errorf("after the turn these files still hold the secret: %v", holding)
	}
}

// The tester's case: a done task whose wrap-up turn runs isn't archived,
// so a redaction across the log holds the turn, and its end through the
// hook doesn't write the secret back. (An archived task with a turn left
// open, from an older log, keeps nothing its end learned:
// hooks.TestATurnOnAnArchivedTaskKeepsNothingItLearned.)
func TestAWrapUpTurnRunningAsTheLogIsRedactedDoesNotWriteTheSecretBack(t *testing.T) {
	f := newEraseFixture(t)
	written := "# Notes\n\nThe key is " + secretHere + "."
	f.teach(t, written)
	f.startTurn(t)
	f.propose(t,
		events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "Moved.", "moved": true}}, // stops turn-1
		events.Proposal{Stream: "task-1", Actor: machine, Type: events.TurnStarted, Correlation: "turn-2", Data: map[string]any{"agent": "engineer", "hash": "h"}},
	)
	task, _ := f.state.Task("task-1")
	summary := tasks.SummaryOf(task, f.state.Roster())
	if _, rejection, err := f.log.Archive("task-1", func() any { return summary }, f.blobs); err != nil || rejection == nil || !strings.Contains(rejection.Reason, "turn running") {
		t.Fatalf("archiving task-1 while its wrap-up turn runs: %v, %v, want refused", rejection, err)
	}
	if answer := redactAcross(t, f.server, secretHere); !slices.Equal(answer.Warnings, []string{stillRunning}) {
		t.Errorf("the redaction warned %q, want only that a running turn's end redacts it again", answer.Warnings)
	}
	f.endTurnOf(t, "turn-2", written+"\n\nDeploy on Fridays.\n")
	engineer, _ := f.state.Roster().Get("engineer")
	if want := "# Notes\n\nThe key is [redacted].\n\nDeploy on Fridays."; engineer.Knowledge != want {
		t.Errorf("the engineer's knowledge after the wrap-up turn: %q, want %q", engineer.Knowledge, want)
	}
	if err := f.snapshots.WriteNow(); err != nil {
		t.Fatal(err)
	}
	if holding := storetest.HoldingAny(t, f.dir, secretHere); len(holding) != 0 {
		t.Errorf("after the wrap-up turn these files still hold the secret: %v", holding)
	}
}

// A redaction across the log that can't be is answered as such, and
// writes nothing: a blank text 400, a text no row holds 409, and the desk
// 401 on both routes.
func TestARefusedRedactionAcrossTheLogWritesNothing(t *testing.T) {
	f := withSecret(t)
	last := f.log.LastPosition()
	for _, path := range []string{"/api/redact/preview", "/api/redact"} {
		for _, blank := range []string{`{}`, `{"text":" \n "}`} {
			if code, body := call(t, f.server, "POST", path, blank); code != http.StatusBadRequest {
				t.Errorf("%s %s: %d %s, want 400", path, blank, code, body)
			}
		}
		if code, body := call(t, f.server, "POST", path, `{"text":"nowhere-to-be-found"}`); code != http.StatusConflict || strings.TrimSpace(body) != "no row of the log holds the secret" {
			t.Errorf("%s a text no row holds: %d %q, want 409", path, code, body)
		}
		if code, body := callAs(t, f.server, "desk-secret", "POST", path); code != http.StatusUnauthorized {
			t.Errorf("the desk on %s: %d %s, want 401", path, code, body)
		}
	}
	if f.log.LastPosition() != last {
		t.Errorf("the refusals wrote up to %d from %d", f.log.LastPosition(), last)
	}
}

// A redaction across the log whose only stream that could hold the text
// can't be read, here an archived task whose blob is lost, fails 500 on
// both routes, as one stream's does, rather than being refused, and
// writes nothing. The answer names the task, not the blob's path on the
// server.
func TestARedactionAcrossTheLogThatCantReadTheOnlyStreamFails(t *testing.T) {
	f := newEraseFixture(t)
	f.propose(t, events.Proposal{Stream: "task-4", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Old", "description": "It was " + secretHere + ".",
		"repo": "/r", "coordinator": "tech-lead"}})
	storetest.Archive(t, f.log, f.state, "task-4") // its blob in a folder the server doesn't read
	last := f.log.LastPosition()
	for _, path := range []string{"/api/redact/preview", "/api/redact"} {
		if code, body := call(t, f.server, "POST", path, `{"text":"`+secretHere+`","why":"Pasted."}`); code != http.StatusInternalServerError || strings.TrimSpace(body) != "task-4's archived rows can't be read: its blob is missing" {
			t.Errorf("%s with task-4's blob lost: %d %q, want 500 naming task-4 and no path", path, code, body)
		}
	}
	if f.log.LastPosition() != last {
		t.Errorf("the failed redaction wrote up to %d from %d", f.log.LastPosition(), last)
	}
}

// A redaction across the log holds each stream to the rows the page
// counted in it: a stream that took a row holding the text since is
// refused alone, its rows left holding it, and one the page didn't count
// is left too, as you didn't see its rows; each is named, to be counted
// again, and the rest are redacted. Counted again, they are redacted, and
// a stream counted that no longer holds the text is named.
func TestARedactionAcrossTheLogHoldsEachStreamToItsCount(t *testing.T) {
	f := withSecret(t)
	f.propose(t,
		events.Proposal{Stream: "task-5", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Rotate " + secretHere, "repo": "/r", "coordinator": "tech-lead"}},
		events.Proposal{Stream: "task-6", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "Revoke " + secretHere, "repo": "/r", "coordinator": "tech-lead"}},
	)
	first := countsOf(previewAcross(t, f.server, secretHere))
	if want := map[string]int{"task-4": 2, "task-5": 1, "task-6": 1}; !maps.Equal(first, want) {
		t.Fatalf("the preview counted %v, want %v", first, want)
	}
	delete(first, "task-6")
	f.propose(t, events.Proposal{Stream: "task-5", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "Rotate " + secretHere + " now"}})

	code, body := redactCounted(t, f.server, secretHere, first)
	if code != http.StatusOK {
		t.Fatalf("redacting with task-5's count stale: %d %s", code, body)
	}
	var answer logRedacted
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatal(err)
	}
	if len(answer.Streams) != 1 || answer.Streams[0].Stream != "task-4" || answer.Rows != 2 {
		t.Errorf("the redaction rewrote %+v, want only task-4's 2 rows", answer.Streams)
	}
	failures := []streamFailed{
		{Stream: "task-5", Reason: "2 rows hold this text now, not 1: count them again", Recount: true},
		{Stream: "task-6", Reason: "1 row holds this text, which the preview you confirmed didn't count: count them again", Recount: true},
	}
	if !slices.Equal(answer.Failures, failures) {
		t.Errorf("the redaction failed %+v, want %+v", answer.Failures, failures)
	}
	if len(answer.Warnings) != 0 {
		t.Errorf("the redaction warned %q, want its failures named only among the failures", answer.Warnings)
	}

	again := previewAcross(t, f.server, secretHere)
	if want := map[string]int{"task-5": 2, "task-6": 1}; !maps.Equal(countsOf(again), want) {
		t.Fatalf("counted again: %v, want task-5 and task-6 still holding the text, %v", countsOf(again), want)
	}
	counts := countsOf(again)
	counts["task-4"] = 2 // as the first preview counted it
	code, body = redactCounted(t, f.server, secretHere, counts)
	if code != http.StatusOK {
		t.Fatalf("redacting counted again: %d %s", code, body)
	}
	answer = logRedacted{}
	if err := json.Unmarshal([]byte(body), &answer); err != nil {
		t.Fatal(err)
	}
	matches(t, answer, again)
	if want := []streamFailed{{Stream: "task-4", Reason: "no row holds this text now, not 2: count them again", Recount: true}}; !slices.Equal(answer.Failures, want) {
		t.Errorf("counted again, the redaction failed %+v, want %+v", answer.Failures, want)
	}
	if holding := storetest.HoldingAny(t, f.dir, secretHere); len(holding) != 0 {
		t.Errorf("after the redaction these files still hold the secret: %v", holding)
	}
}

// A count below one is refused 400, as a redaction rewrites at least one
// row, and a redaction whose every counted stream is refused is refused
// 409 with their reasons, as one with no counts is; neither writes.
func TestARedactionAcrossTheLogWithABadOrStaleCountIsRefused(t *testing.T) {
	f := withSecret(t)
	last := f.log.LastPosition()
	for _, rows := range []int{0, -1} {
		if code, body := redactCounted(t, f.server, secretHere, map[string]int{"task-4": rows}); code != http.StatusBadRequest {
			t.Errorf("redacting with task-4 counted %d: %d %s, want 400", rows, code, body)
		}
	}
	want := "task-4: 2 rows hold this text now, not 1: count them again"
	if code, body := redactCounted(t, f.server, secretHere, map[string]int{"task-4": 1}); code != http.StatusConflict || strings.TrimSpace(body) != want {
		t.Errorf("redacting with task-4 miscounted: %d %q, want 409 %q", code, body, want)
	}
	if f.log.LastPosition() != last {
		t.Errorf("the refusals wrote up to %d from %d", f.log.LastPosition(), last)
	}
}
