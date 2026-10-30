package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// answering is a log that gives back what it was set to.
type answering struct {
	event     events.Event
	rejection *events.Rejection
	err       error
}

func (a answering) Propose(events.Proposal) (events.Event, *events.Rejection, error) {
	return a.event, a.rejection, a.err
}

func (a answering) Create(_ string, p events.Proposal) (events.Event, *events.Rejection, error) {
	return a.Propose(p)
}

func TestProposeAnswersAsTheLogDid(t *testing.T) {
	for _, c := range []struct {
		name   string
		log    answering
		status int
		body   string
		ok     bool
	}{
		{"written", answering{event: events.Event{Position: 7, Stream: "task-1"}}, http.StatusOK, `"position":7`, true},
		{"refused", answering{rejection: &events.Rejection{Check: "rules", Reason: "the task is done"}}, http.StatusConflict, "the task is done", false},
		{"the log failed", answering{err: errors.New("disk full")}, http.StatusInternalServerError, "disk full", false},
	} {
		w := httptest.NewRecorder()
		Propose(w, c.log, events.Proposal{Stream: "task-1", Actor: You})
		if w.Code != c.status || !strings.Contains(w.Body.String(), c.body) {
			t.Errorf("Propose, %s: %d %q, want %d with %q", c.name, w.Code, w.Body, c.status, c.body)
		}
		w = httptest.NewRecorder()
		if ok := ProposeOrRefuse(w, c.log, events.Proposal{Stream: "task-1", Actor: You}); ok != c.ok || (!ok && w.Code != c.status) {
			t.Errorf("ProposeOrRefuse, %s: %v with %d, want %v with %d", c.name, ok, w.Code, c.ok, c.status)
		}
		if c.ok && w.Body.Len() != 0 {
			t.Errorf("ProposeOrRefuse, %s, answered for the request: %q", c.name, w.Body)
		}
	}
}

func TestDecodeRefusesABadBody(t *testing.T) {
	var into struct{ Note string }
	w := httptest.NewRecorder()
	if Decode(w, httptest.NewRequest("POST", "/", strings.NewReader("{")), &into) || w.Code != http.StatusBadRequest {
		t.Errorf("a broken body: %d %q, want 400", w.Code, w.Body)
	}
	w = httptest.NewRecorder()
	if !Decode(w, httptest.NewRequest("POST", "/", strings.NewReader(`{"note":"later"}`)), &into) || into.Note != "later" {
		t.Errorf("a good body: %+v, %d %q", into, w.Code, w.Body)
	}
}

func TestLimitOf(t *testing.T) {
	for _, c := range []struct{ asked, byDefault, want int }{{0, 500, 500}, {NoLimit, 500, 0}, {200, 500, 200}, {-5, 500, -5}} {
		if got := LimitOf(c.asked, c.byDefault); got != c.want {
			t.Errorf("LimitOf(%d, %d) = %d, want %d", c.asked, c.byDefault, got, c.want)
		}
	}
}
