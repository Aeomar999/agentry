package hooks

import (
	"crypto/subtle"
	"encoding/json"
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/hookcall"
)

// HTTP serves hook calls at hookcall.Path. Only callers with the hook token
// get in; that token can post hook calls and nothing else.
func HTTP(h *Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		given := r.Header.Get(hookcall.TokenHeader)
		if subtle.ConstantTimeCompare([]byte(given), []byte(token)) != 1 {
			http.Error(w, "wrong hook token", http.StatusUnauthorized)
			return
		}
		var call hookcall.Call
		if err := json.NewDecoder(r.Body).Decode(&call); err != nil {
			http.Error(w, "bad hook call: "+err.Error(), http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(h.Handle(r.Context(), call))
	})
}
