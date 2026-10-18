package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// The UI, built from ui/ by `make ui` into dist/ and embedded, so the
// binary serves it with nothing else to install.
//
//go:embed all:dist
var built embed.FS

var ui = func() fs.FS {
	sub, err := fs.Sub(built, "dist")
	if err != nil {
		panic(err) // "dist" is a valid path, so this can't happen
	}
	return sub
}()

// page serves the UI. A file that exists is served as it is: the bundles
// under assets/ have their hash in their names, so the browser may keep
// them. Every other path is a page of the UI, which routes in the browser,
// so it gets index.html.
//
// The page's files, the manifest, service worker and icons that install it
// as an app among them, need no token: browsers fetch those without one.
// The data behind the page does.
//
// The UI keeps the token from ?token= and sends it on every call, so
// signing in doesn't depend on a cookie surviving port forwarding or a
// change between localhost and 127.0.0.1. The cookie is still set, for
// browsers that keep it.
func (s *Server) page(w http.ResponseWriter, r *http.Request) {
	if strings.HasPrefix(r.URL.Path, "/api/") {
		http.NotFound(w, r)
		return
	}
	file := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
	if info, err := fs.Stat(ui, file); file != "" && err == nil && !info.IsDir() {
		switch {
		case strings.HasPrefix(file, "assets/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		case file == "sw.js" || file == "manifest.webmanifest":
			w.Header().Set("Cache-Control", "no-cache") // so an installed app picks up a new version
		}
		if path.Ext(file) == ".webmanifest" {
			w.Header().Set("Content-Type", "application/manifest+json") // not in every system's MIME table
		}
		http.ServeFileFS(w, r, ui, file)
		return
	}

	if token := r.URL.Query().Get("token"); token != "" {
		if token != s.config.UIToken {
			http.Error(w, "wrong token: open the link `agentry serve` printed", http.StatusUnauthorized)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: uiCookie, Value: token, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	}
	index, err := fs.ReadFile(ui, "index.html")
	if err != nil {
		http.Error(w, "the UI isn't built: run `make ui`", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache") // it names the current bundles
	w.Write(index)
}
