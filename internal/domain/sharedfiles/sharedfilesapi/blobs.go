package sharedfilesapi

import (
	"io"
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
)

// A task's shared files as the workers fetch and send them, by their hash
// (package sharedfiles).

// blobFor sends a worker a file it needs, by its hash.
func (a API) blobFor(w http.ResponseWriter, r *http.Request) {
	if !a.Blobs.Has(r.PathValue("hash")) {
		http.Error(w, "no such file", http.StatusNotFound)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	body, err := a.Blobs.Open(r.PathValue("hash"))
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, body)
}

// blobFrom keeps a file a worker sends, and says its hash.
func (a API) blobFrom(w http.ResponseWriter, r *http.Request) {
	hash, size, err := a.Blobs.Put(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpapi.Reply(w, map[string]any{"hash": hash, "size": size})
}
