package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// withEvents is a test server whose task-1 has many events, task-2's between
// them and a refusal among them, and task-1's whole record as a page reads
// it: the stream itself, through entryOf and JSON.
func withEvents(t *testing.T, labels int) (*httptest.Server, []eventEntry) {
	t.Helper()
	server, s := newTestServer(t)
	call(t, server, "POST", "/api/tasks", `{"title":"Many events"}`)
	call(t, server, "POST", "/api/tasks", `{"title":"Others between"}`)
	for i := range labels {
		label := func(task string) {
			s.Propose(events.Proposal{Stream: task, Actor: you, Type: events.TaskLabeled, Data: map[string]any{"labels": []string{fmt.Sprint("l", i)}}})
		}
		label("task-1")
		label("task-2")
		if i == labels/2 {
			refuseOn(t, s, "task-1")
		}
	}
	log, err := s.Log("task-1")
	if err != nil {
		t.Fatal(err)
	}
	var entries []eventEntry
	for _, e := range log {
		entries = append(entries, entryOf(e))
	}
	encoded, err := json.Marshal(entries)
	if err != nil {
		t.Fatal(err)
	}
	var record []eventEntry
	if err := json.Unmarshal(encoded, &record); err != nil {
		t.Fatal(err)
	}
	return server, record
}

func refuseOn(t *testing.T, s *store.Store, task string) {
	t.Helper()
	if _, rejection, _ := s.Propose(events.Proposal{Stream: task, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "again", "repo": "/r", "coordinator": "tech-lead"}}); rejection == nil {
		t.Fatal("a second task.created should be refused")
	}
}

func pageAt(t *testing.T, server *httptest.Server, query string) eventsPage {
	t.Helper()
	code, body := call(t, server, "GET", "/api/tasks/task-1/events"+query, "")
	if code != http.StatusOK {
		t.Fatalf("%s: %d %s", query, code, body)
	}
	var page eventsPage
	if err := json.Unmarshal([]byte(body), &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func positionsOf(entries []eventEntry) []int64 {
	positions := []int64{}
	for _, e := range entries {
		positions = append(positions, e.Position)
	}
	return positions
}

// Walking the pages from the start, each after the last one's end, gives
// exactly the task's record, in its order, in pages of the default
// size and of a small one.
func TestTheEventPagesAddUpToTheTasksRecord(t *testing.T) {
	server, all := withEvents(t, 600)
	if len(all) <= eventPage {
		t.Fatalf("the task has %d events, too few to need a second page", len(all))
	}
	for _, limit := range []string{"", "&limit=7"} {
		var walked []eventEntry
		pages := 0
		for after, more := int64(0), true; more; pages++ {
			page := pageAt(t, server, fmt.Sprintf("?after=%d%s", after, limit))
			walked = append(walked, page.Events...)
			more = page.More
			if len(page.Events) > 0 {
				after = page.Events[len(page.Events)-1].Position
			}
			if pages > len(all) {
				t.Fatal("the pages never end")
			}
		}
		if !reflect.DeepEqual(walked, all) {
			t.Errorf("limit %q: %d pages give %d events, the record has %d; first positions %v, want %v",
				limit, pages, len(walked), len(all), positionsOf(walked)[:min(5, len(walked))], positionsOf(all)[:5])
		}
	}
}

// Walking the pages of some types, from the start or from the end, gives
// exactly the record's events of those types, in its order: the limit and
// "more" count only those types, and a type no event has matches nothing.
func TestTheEventPagesOfSomeTypesAreTheRecordsEventsOfThoseTypes(t *testing.T) {
	server, all := withEvents(t, 600)
	var want []eventEntry
	for _, e := range all {
		if e.Type == events.ProposalRejected || e.Type == events.TaskCreated {
			want = append(want, e)
		}
	}
	if len(want) != 2 {
		t.Fatalf("task-1 should have one task.created and one refusal: %v", positionsOf(want))
	}
	types := "&type=proposal.rejected&type=no.such_type&type=task.created"
	for _, limit := range []string{"", "&limit=1"} {
		var walked []eventEntry
		for after, more := int64(0), true; more; {
			page := pageAt(t, server, fmt.Sprintf("?after=%d%s%s", after, types, limit))
			walked = append(walked, page.Events...)
			more = page.More
			if len(page.Events) > 0 {
				after = page.Events[len(page.Events)-1].Position
			}
			if len(walked) > len(all) {
				t.Fatal("the pages never end")
			}
		}
		if !reflect.DeepEqual(walked, want) {
			t.Errorf("limit %q, from the start: positions %v, want %v", limit, positionsOf(walked), positionsOf(want))
		}
		walked = nil
		for before, more := all[len(all)-1].Position+1, true; more; {
			page := pageAt(t, server, fmt.Sprintf("?before=%d%s%s", before, types, limit))
			walked = append(page.Events, walked...)
			more = page.More
			if len(page.Events) > 0 {
				before = page.Events[0].Position
			}
			if len(walked) > len(all) {
				t.Fatal("the pages never end")
			}
		}
		if !reflect.DeepEqual(walked, want) {
			t.Errorf("limit %q, from the end: positions %v, want %v", limit, positionsOf(walked), positionsOf(want))
		}
	}
	if page := pageAt(t, server, "?type=no.such_type"); len(page.Events) != 0 || page.More {
		t.Errorf("a type no event has: %+v", page)
	}
}

// A page is the first events after ?after=, or the last before ?before= when
// that is all it is given, in order either way, and says whether more lie
// beyond it on the side it was read from.
func TestAnEventPageIsBoundedAndSaysWhetherThereIsMore(t *testing.T) {
	server, all := withEvents(t, 5)
	p := positionsOf(all)
	n := len(p)
	if n != 7 {
		t.Fatalf("task-1 should have 7 events, its creation, 5 labels and a refusal: %v", p)
	}
	cases := []struct {
		query string
		want  []int64
		more  bool
	}{
		{"", p, false},
		{"?limit=3", p[:3], true},
		{fmt.Sprintf("?after=%d&limit=3", p[0]), p[1:4], true},
		{fmt.Sprintf("?after=%d", p[1]), p[2:], false},
		{fmt.Sprintf("?after=%d&limit=%d", p[1], n-2), p[2:], false},
		{fmt.Sprintf("?after=%d", p[n-1]), []int64{}, false},
		{fmt.Sprintf("?before=%d&limit=3", p[n-1]+1), p[n-3:], true},
		{fmt.Sprintf("?before=%d&limit=%d", p[n-1]+1, n), p, false},
		{fmt.Sprintf("?before=%d&limit=3", p[n-1]), p[n-4 : n-1], true},
		{fmt.Sprintf("?before=%d", p[2]), p[:2], false},
		{fmt.Sprintf("?before=%d", p[0]), []int64{}, false},
		{fmt.Sprintf("?after=%d&before=%d", p[0], p[3]), p[1:3], false},
		{fmt.Sprintf("?after=%d&before=%d&limit=1", p[0], p[3]), p[1:2], true},
		{fmt.Sprintf("?after=%d&before=%d", p[3], p[3]), []int64{}, false},
		{fmt.Sprintf("?after=%d&before=%d", p[4], p[2]), []int64{}, false},
	}
	for _, c := range cases {
		page := pageAt(t, server, c.query)
		if got := positionsOf(page.Events); !reflect.DeepEqual(got, c.want) || page.More != c.more {
			t.Errorf("%q: positions %v more %v, want %v more %v", c.query, got, page.More, c.want, c.more)
		}
	}
	refused := false
	for _, e := range pageAt(t, server, "").Events {
		refused = refused || e.Type == events.ProposalRejected
	}
	if !refused {
		t.Error("the refusal on the task should be among its events, as in its record")
	}
}

func TestAnEventPageOfNoTaskOrWithBadBoundsIsRefused(t *testing.T) {
	server, _ := withEvents(t, 1)
	if code, _ := call(t, server, "GET", "/api/tasks/task-9/events", ""); code != http.StatusNotFound {
		t.Errorf("no such task: %d", code)
	}
	for _, query := range []string{"after=-1", "after=x", "after=", "before=0", "before=1.5", "limit=0", "limit=-3",
		fmt.Sprint("limit=", eventPageMax+1), "limit=many", strings.Repeat("type=task.created&", eventTypesMax+1)} {
		if code, body := call(t, server, "GET", "/api/tasks/task-1/events?"+query, ""); code != http.StatusBadRequest || !strings.Contains(body, "bad request") {
			t.Errorf("?%s: %d %s", query, code, body)
		}
	}
	if code, _ := call(t, server, "GET", fmt.Sprint("/api/tasks/task-1/events?limit=", eventPageMax), ""); code != http.StatusOK {
		t.Errorf("the largest limit: %d", code)
	}
	if code, _ := call(t, server, "GET", "/api/tasks/task-1/events?"+strings.Repeat("type=task.created&", eventTypesMax), ""); code != http.StatusOK {
		t.Errorf("the most types: %d", code)
	}
}
