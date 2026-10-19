package web

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/httpapi"
)

// claudeMessage is a message to the coordinator that you asked Claude, in a
// task's Workspace, to send with `agentry send`. It goes as yours, the same
// as one typed in the chat, marked as sent through Claude. Claude Code asks
// you before it runs the command, so it never sends one on its own.
func (s *Server) claudeMessage(w http.ResponseWriter, r *http.Request) {
	var request struct{ Text string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	s.say(w, r.PathValue("task"), request.Text, "claude")
}

// requireClaude lets in only the token of the Claude sessions in tasks'
// Workspaces, which sends what you ask it to, and does nothing else.
func (s *Server) requireClaude(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		given := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if s.config.HelperToken == "" || subtle.ConstantTimeCompare([]byte(given), []byte(s.config.HelperToken)) != 1 {
			http.Error(w, "only Claude in a task's Workspace sends this", http.StatusUnauthorized)
			return
		}
		next(w, r)
	})
}
