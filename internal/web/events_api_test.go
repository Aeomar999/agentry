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
// them and a refusal among them, and task-1's events as its detail has them.
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
	_, body := call(t, server, "GET", "/api/tasks/task-1", "")
	var detail taskDetail
	if err := json.Unmarshal([]byte(body), &detail); err != nil {
		t.Fatal(err)
	}
	return server, detail.Events
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
// exactly the events the detail has, in its order, in pages of the default
// size and of a small one.
func TestTheEventPagesAddUpToTheDetailsEvents(t *testing.T) {
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
			t.Errorf("limit %q: %d pages give %d events, the detail has %d; first positions %v, want %v",
				limit, pages, len(walked), len(all), positionsOf(walked)[:min(5, len(walked))], positionsOf(all)[:5])
		}
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
		{fmt.Sprintf("?before=%d&limit=3", p[n-1]), p[n-4 : n-1], true},
		{fmt.Sprintf("?before=%d", p[2]), p[:2], false},
		{fmt.Sprintf("?before=%d", p[0]), []int64{}, false},
		{fmt.Sprintf("?after=%d&before=%d", p[0], p[3]), p[1:3], false},
		{fmt.Sprintf("?after=%d&before=%d&limit=1", p[0], p[3]), p[1:2], true},
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
		t.Error("the refusal on the task should be among its events, as in its detail")
	}
}

func TestAnEventPageOfNoTaskOrWithBadBoundsIsRefused(t *testing.T) {
	server, _ := withEvents(t, 1)
	if code, _ := call(t, server, "GET", "/api/tasks/task-9/events", ""); code != http.StatusNotFound {
		t.Errorf("no such task: %d", code)
	}
	for _, query := range []string{"after=-1", "after=x", "after=", "before=0", "before=1.5", "limit=0", "limit=-3",
		fmt.Sprint("limit=", eventPageMax+1), "limit=many"} {
		if code, body := call(t, server, "GET", "/api/tasks/task-1/events?"+query, ""); code != http.StatusBadRequest || !strings.Contains(body, "bad request") {
			t.Errorf("?%s: %d %s", query, code, body)
		}
	}
	if code, _ := call(t, server, "GET", fmt.Sprint("/api/tasks/task-1/events?limit=", eventPageMax), ""); code != http.StatusOK {
		t.Errorf("the largest limit: %d", code)
	}
}
