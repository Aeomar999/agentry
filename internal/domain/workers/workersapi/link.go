package workersapi

import (
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// A worker's own link to the server, over the hub (package workerlink):
// connecting, the streams it opens for a turn, updating itself and leaving.

func (a API) connect(w http.ResponseWriter, r *http.Request) {
	a.Connect(w, r, extensions.WorkerOf(r))
}

func (a API) stream(w http.ResponseWriter, r *http.Request) {
	a.Stream(w, r, extensions.WorkerOf(r), r.PathValue("id"))
}

// update sends agentry for a worker updating itself: it sends the hash of
// the agentry it has as If-None-Match, and gets 304 if it is current.
func (a API) update(w http.ResponseWriter, r *http.Request) {
	a.ServeBinary(w, r)
}

// leave takes the worker off at its own request, as on the page.
func (a API) leave(w http.ResponseWriter, r *http.Request) {
	worker := extensions.WorkerOf(r)
	httpapi.Propose(w, a.Log, workers.Left(worker.Name))
	a.Disconnect(worker.Name) // a no-op if it was refused: the credential still works
}
