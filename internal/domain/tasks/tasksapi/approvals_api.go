package tasksapi

import (
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
)

func (a API) approve(w http.ResponseWriter, r *http.Request) {
	a.decide(w, r, events.ActionApproved)
}
func (a API) deny(w http.ResponseWriter, r *http.Request) { a.decide(w, r, events.ActionDenied) }

func (a API) decide(w http.ResponseWriter, r *http.Request, kind events.Type) {
	var request struct{ Reason string }
	if r.ContentLength != 0 && !httpapi.Decode(w, r, &request) {
		return
	}
	if request.Reason == "" {
		request.Reason = "no reason given"
	}
	if a.isNone(r.PathValue("task")) {
		a.noTask(w, r.PathValue("task"))
		return
	}
	httpapi.Propose(w, a.Log, events.Proposal{Stream: r.PathValue("task"), Actor: httpapi.You, Type: kind,
		Correlation: r.PathValue("action"), Data: map[string]any{"reason": request.Reason}})
}
