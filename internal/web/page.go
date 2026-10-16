package web

import (
	_ "embed"
	"net/http"
)

//go:embed page.html
var pageHTML []byte

// page serves the one page. The page itself keeps the token from ?token=
// and sends it on every call, so signing in doesn't depend on a cookie
// surviving port forwarding or a change between localhost and 127.0.0.1.
// The cookie is still set, for browsers that keep it.
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if token := r.URL.Query().Get("token"); token != "" {
		if token != s.config.UIToken {
			http.Error(w, "wrong token: open the link `agentry serve` printed", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: uiCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(pageHTML)
}
