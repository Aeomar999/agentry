// Package httpapi is how every handler of Agentry's HTTP API answers, the
// core's in web and an extension's in its own package: JSON replies, request
// bodies, writes to the log turned into status codes, and the reply for a
// task there is none of. Like extensions, it is a leaf: it imports only
// events, extensions and the tasks domain.
package httpapi

import (
	"encoding/json"
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// You is who every write from the page or the desk is made as.
var You = events.Actor{Kind: events.Human, ID: "you"}

// System is who a signed route's writes are made as: a webhook speaks for
// the service that signed it, not for you.
var System = events.Actor{Kind: events.System, ID: "webhook"}

// MaxBody is the most a request's body may hold.
const MaxBody = 1 << 20

// Decode reads a request's JSON body into into, and answers 400 when it
// can't. A body over MaxBody is refused.
func Decode(w http.ResponseWriter, r *http.Request, into any) bool {
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, MaxBody)).Decode(into); err != nil {
		http.Error(w, "bad request: "+err.Error(), http.StatusBadRequest)
		return false
	}
	return true
}

// Reply answers with value as JSON.
func Reply(w http.ResponseWriter, value any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(value)
}

// Failed answers 500 with err.
func Failed(w http.ResponseWriter, err error) {
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// Propose writes a proposal and replies with the event, or with the
// validator's reason as a 409 when it was refused.
func Propose(w http.ResponseWriter, log extensions.Proposer, p events.Proposal) {
	if event, ok := write(w, log, p); ok {
		Reply(w, event)
	}
}

// ProposeOrRefuse writes a proposal that comes before another, and answers
// for the request only if it fails.
func ProposeOrRefuse(w http.ResponseWriter, log extensions.Proposer, p events.Proposal) bool {
	_, ok := write(w, log, p)
	return ok
}

// write proposes p, and answers 500 when the log fails or 409 with the
// reason when it was refused.
func write(w http.ResponseWriter, log extensions.Proposer, p events.Proposal) (events.Event, bool) {
	event, rejection, err := log.Propose(p)
	switch {
	case err != nil:
		Failed(w, err)
		return events.Event{}, false
	case rejection != nil:
		http.Error(w, rejection.Reason, http.StatusConflict)
		return events.Event{}, false
	}
	return event, true
}

// Tasks is what the replies about a task read: the task, and an erased
// one's erasure.
type Tasks interface {
	Task(id string) (*tasks.Task, error)
	Erased(id string) (tasks.Erasure, bool)
}

// IsNone is whether there is no task id, never made or erased.
func IsNone(state Tasks, id string) bool {
	task, err := state.Task(id)
	_, erased := state.Erased(id)
	return erased || err == nil && task == nil
}

// NoTask answers that there is no task id to act on: that an erased one
// is gone for good, and when it went, 410, rather than that there was
// never such a task, 404. It names id, as a route may ask about more than
// one task.
func NoTask(w http.ResponseWriter, state Tasks, id string) {
	if erasure, erased := state.Erased(id); erased {
		http.Error(w, id+" was erased on "+erasure.At.Format("2 January 2006"), http.StatusGone)
		return
	}
	http.Error(w, "no such task "+id, http.StatusNotFound)
}

// NoLimit is what a request sends for no limit. At creation 0 takes the
// server's default, so a task made without one is unlimited only when asked.
const NoLimit = -1

// LimitOf is a budget or limit of turns as the task keeps it, 0 for no
// limit, from a request's: 0 takes byDefault (the server's for a new task, no
// limit for a change), and NoLimit is none.
func LimitOf(asked, byDefault int) int {
	switch asked {
	case 0:
		return byDefault
	case NoLimit:
		return 0
	}
	return asked // below -1, the rules refuse it
}
