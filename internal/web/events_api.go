package web

import (
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
)

// A task's events a page at a time, so the page needn't take a long task's
// whole record with its detail.

const (
	eventPage     = 500  // events in a page unless ?limit= says otherwise
	eventPageMax  = 2000 // the most ?limit= may ask for
	eventTypesMax = 100  // the most types ?type= may name, far more than there are
)

type eventsPage struct {
	Events []eventEntry `json:"events"`
	More   bool         `json:"more"` // more events lie beyond the page, on the side it was read from
}

// listEvents is a page of a task's events, rejections included, in order:
// those after ?after= and before ?before=, both positions left out, of the
// types ?type= names (it may be given again for more) or of every type when
// it isn't given. The page is the first ?limit= of them, or the last when
// only ?before= is given, so the newest page is ?before= the position in
// the X-Agentry-Position header of the read it goes with, plus one.
func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	task, archived := s.shownTask(w, r)
	if task == nil {
		return
	}
	after, before, limit, err := pageOf(r)
	if err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	types, err := typesOf(r)
	if err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return
	}
	fromEnd := r.URL.Query().Has("before") && !r.URL.Query().Has("after")
	read := s.store.LogAfter
	switch {
	case archived != nil:
		read = pageIn(archived.history, fromEnd)
	case fromEnd:
		read = s.store.LogBefore
	}
	found, err := read(task.ID, types, after, before, limit+1) // one more, to tell whether there is more
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	page := eventsPage{Events: []eventEntry{}, More: len(found) > limit}
	switch {
	case page.More && fromEnd:
		found = found[1:]
	case page.More:
		found = found[:limit]
	}
	for _, e := range found {
		page.Events = append(page.Events, entryOf(e))
	}
	httpapi.Reply(w, page)
}

// pageOf reads a page's bounds and size from the query: no bound when a
// position is left out, eventPage events when the limit is.
func pageOf(r *http.Request) (after, before int64, limit int, err error) {
	query := r.URL.Query()
	before, limit = math.MaxInt64, eventPage
	if text := query.Get("after"); query.Has("after") {
		if after, err = strconv.ParseInt(text, 10, 64); err != nil || after < 0 {
			return 0, 0, 0, fmt.Errorf("after is a position, 0 or more: %q", text)
		}
	}
	if text := query.Get("before"); query.Has("before") {
		if before, err = strconv.ParseInt(text, 10, 64); err != nil || before < 1 {
			return 0, 0, 0, fmt.Errorf("before is a position, 1 or more: %q", text)
		}
	}
	if text := query.Get("limit"); query.Has("limit") {
		if limit, err = strconv.Atoi(text); err != nil || limit < 1 || limit > eventPageMax {
			return 0, 0, 0, fmt.Errorf("limit is from 1 to %d: %q", eventPageMax, text)
		}
	}
	return after, before, limit, nil
}

// typesOf is the types ?type= names, none for every type. A name no event
// has is no mistake: it just matches nothing.
func typesOf(r *http.Request) ([]events.Type, error) {
	names := r.URL.Query()["type"]
	if len(names) > eventTypesMax {
		return nil, fmt.Errorf("type is given %d times, at most %d", len(names), eventTypesMax)
	}
	var types []events.Type
	for _, name := range names {
		types = append(types, events.Type(name))
	}
	return types, nil
}

// pageIn reads a page from a task's whole record held in memory, as an
// archived task's is, the way the store's LogAfter reads one from the log,
// or LogBefore when fromEnd.
func pageIn(record []events.Event, fromEnd bool) func(string, []events.Type, int64, int64, int) ([]events.Event, error) {
	return func(_ string, types []events.Type, after, before int64, limit int) ([]events.Event, error) {
		var found []events.Event
		for _, e := range record {
			if e.Position > after && e.Position < before && (len(types) == 0 || slices.Contains(types, e.Type)) {
				found = append(found, e)
			}
		}
		if fromEnd {
			return found[max(len(found)-limit, 0):], nil
		}
		return found[:min(limit, len(found))], nil
	}
}
