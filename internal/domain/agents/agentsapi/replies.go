package agentsapi

import (
	"encoding/json"
	"net/http"
	"strings"
)

// replyWith answers status with value as JSON.
func replyWith(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

// jsonErrors writes the plain-text errors of http.Error as {"error": ...},
// so a handler built from the shared helpers replies in one shape.
type jsonErrors struct {
	http.ResponseWriter
	failing bool
}

func (w *jsonErrors) WriteHeader(status int) {
	if status >= http.StatusBadRequest && strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain") {
		w.failing = true
		w.Header().Set("Content-Type", "application/json")
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *jsonErrors) Write(text []byte) (int, error) {
	if !w.failing {
		return w.ResponseWriter.Write(text)
	}
	err := json.NewEncoder(w.ResponseWriter).Encode(map[string]string{"error": strings.TrimSpace(string(text))})
	return len(text), err
}
