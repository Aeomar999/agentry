package httpapi

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
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

// board is a state with task-1, and task-2 erased, as current's State reads
// them: an erased task's Task is an error.
type board struct{ erasedAt time.Time }

func (b board) Task(id string) (*tasks.Task, error) {
	switch id {
	case "task-1":
		return &tasks.Task{ID: id}, nil
	case "task-2":
		return nil, errors.New("erased")
	}
	return nil, nil
}

func (b board) Erased(id string) (tasks.Erasure, bool) {
	if id == "task-2" {
		return tasks.Erasure{ID: id, At: b.erasedAt}, true
	}
	return tasks.Erasure{}, false
}

// A task there never was is 404 and an erased one 410 with its day, both
// as plain text naming it; a task that is there is not none.
func TestNoTaskSaysWhetherItWasErased(t *testing.T) {
	state := board{erasedAt: time.Date(2026, 11, 3, 9, 0, 0, 0, time.UTC)}
	for _, c := range []struct {
		id   string
		none bool
		code int
		body string
	}{
		{"task-1", false, 0, ""},
		{"task-2", true, http.StatusGone, "task-2 was erased on 3 November 2026\n"},
		{"task-9", true, http.StatusNotFound, "no such task task-9\n"},
	} {
		if IsNone(state, c.id) != c.none {
			t.Errorf("IsNone(%s) = %v, want %v", c.id, !c.none, c.none)
		}
		if !c.none {
			continue
		}
		w := httptest.NewRecorder()
		NoTask(w, state, c.id)
		if w.Code != c.code || w.Body.String() != c.body || w.Header().Get("Content-Type") != "text/plain; charset=utf-8" {
			t.Errorf("NoTask(%s): %d %q %q, want %d %q as plain text", c.id, w.Code, w.Body, w.Header().Get("Content-Type"), c.code, c.body)
		}
	}
}
