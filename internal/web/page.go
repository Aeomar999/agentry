package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

// The UI, built from ui/ by `make ui` into static/dist/ and embedded, so
// the binary serves it with nothing else to install. The built page isn't
// committed, so static/ also holds notbuilt.html, which keeps the embed
// valid in a binary built without `make ui` and is served in its place.
//
//go:embed all:static
var embedded embed.FS

// static is the embedded folder; a test swaps in a tree of its own.
var static = func() fs.FS {
	sub, err := fs.Sub(embedded, "static")
	if err != nil {
		panic(err) // "static" is a valid path, so this can't happen
	}
	return sub
}()

// built is the page in static/dist, and whether there is one: a binary
// built without `make ui` has no static/dist/index.html.
func built() (fs.FS, bool) {
	ui, _ := fs.Sub(static, "dist") // "dist" is a valid path, so this can't fail
	_, err := fs.Stat(ui, "index.html")
	return ui, err == nil
}

// page serves the UI. A file that exists is served as it is: the bundles
// under assets/ have their hash in their names, so the browser may keep
// them. Every other path is a page of the UI, which routes in the browser,
// so it gets index.html. Without a built page, every page path gets
// notbuilt.html, which says how to build it.
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
	ui, isBuilt := built()
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
		setUICookie(w, r, token)
	}
	if !isBuilt {
		page, err := fs.ReadFile(static, "notbuilt.html")
		if err != nil {
			panic(err) // notbuilt.html is committed beside dist, so this can't happen
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache") // so a rebuilt binary's page shows
		w.Write(page)
		return
	}
	index, err := fs.ReadFile(ui, "index.html")
	if err != nil {
		panic(err) // built checked it is there
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache") // it names the current bundles
	w.Write(index)
}
