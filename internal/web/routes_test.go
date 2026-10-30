package web

import (
	"cmp"
	"flag"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/domain/workers/workerlink"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/platform/hookcall"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
	"gitlab.com/dev.chempa/agentry/internal/testkit/goldentest"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

var update = flag.Bool("update", false, "rewrite testdata/routes.txt from the routes the server registers now")

// webRoutes are web's own routes, which register puts on the mux itself,
// and the access kind of each one's wrapper, labelled by hand. Every other
// route is a domain's or an extension's, whose access kind comes from its
// Auth, through wrapperOf.
var webRoutes = map[string]string{
	"POST " + hookcall.Path:                 "hook token",
	"GET /":                                 "open",
	"POST /api/tasks/{task}/claude/message": "helper token",
	"GET /api/features":                     "UI",
	"GET /api/waiting":                      "UI",
	"GET /api/config":                       "UI",
	"GET /api/changes":                      "UI",
	"POST /api/theme":                       "UI",
	"POST /api/signin/codes":                "UI token only",
	"POST /api/signin":                      "open",
	"POST /api/signout":                     "open",
	"GET /api/tasks/{task}/terminal":        "UI",
	"GET /api/tasks/{task}/claude/terminal": "UI",
	"GET /api/desk/terminal":                "UI",
}

const rerunRoutes = "go test ./internal/web -run TestTheRouteTableIsUpToDate -update"

// testdata/routes.txt is every route a server with everything on
// registers, with its owner and access kind. A route's access kind changing,
// or one added or dropped, shows as a diff of that file for a reviewer to
// read. web registers no route of its own besides webRoutes.
func TestTheRouteTableIsUpToDate(t *testing.T) {
	installed := storetest.Installed()
	server := fullServer(t, installed)
	routes := &routeRecorder{mux: http.NewServeMux(), owners: map[string]ownedRoute{}}
	server.register(routes)

	for _, pattern := range routes.patterns {
		if _, mounted := routes.owners[pattern]; mounted {
			continue
		}
		access, labelled := webRoutes[pattern]
		if !labelled {
			t.Errorf("web registers %q itself, but its own routes are only those in webRoutes: a new route belongs in a domain's api "+
				"package (in its Routes(), mounted with s.mount in register) or in an extension, not in web/server.go", pattern)
			access = "unlabelled"
		}
		routes.owners[pattern] = ownedRoute{owner: "web", access: access}
	}
	for _, pattern := range slices.Sorted(maps.Keys(webRoutes)) {
		if !slices.Contains(routes.patterns, pattern) {
			t.Errorf("webRoutes labels %q, which web no longer registers: take it out of webRoutes", pattern)
		}
	}

	var names []string
	for _, f := range installed {
		names = append(names, f.Name)
	}
	table := "# Every route `agentry serve` registers: pattern, owner, access kind. Written by\n" +
		"# `" + rerunRoutes + "`; reviewers read its diff.\n" +
		"# The server has a task terminal, the Claude helper's terminal, the desk, workers\n" +
		"# (their routes and the shared files' blobs) and the extensions serve installs: " + strings.Join(names, ", ") + ".\n"
	for _, pattern := range sortedRoutes(routes.patterns) {
		route := routes.owners[pattern]
		table += fmt.Sprintf("%-56s %-28s %s\n", pattern, route.owner, route.access)
	}
	goldentest.Check(t, filepath.Join("testdata", "routes.txt"), table, *update, rerunRoutes)
}

// web's own routes that take the UI token answer 401 to a request with
// none, so a route that loses its wrapper fails here: the hand labels in
// webRoutes say what each should have, and this checks they still do.
func TestWebsOwnRoutesRefuseARequestWithNoToken(t *testing.T) {
	handler := fullServer(t, storetest.Installed()).Handler()
	for _, pattern := range slices.Sorted(maps.Keys(webRoutes)) {
		if access := webRoutes[pattern]; access != "UI" && access != "UI token only" {
			continue
		}
		method, path, _ := strings.Cut(pattern, " ")
		reply := httptest.NewRecorder()
		handler.ServeHTTP(reply, httptest.NewRequest(method, strings.ReplaceAll(path, "{task}", "task-1"), nil))
		if reply.Code != http.StatusUnauthorized {
			t.Errorf("%s with no token: %d, want 401: is it still behind the wrapper webRoutes names?", pattern, reply.Code)
		}
	}
}

// fullServer is a server with every route on: a task terminal, the Claude
// helper's terminal, the desk, workers (their routes and the shared files'
// blobs) and the extensions in installed.
func fullServer(t *testing.T, installed []extensions.Extension) *Server {
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	fleet := machines.NewFleet(&machines.Local{Label: "here", WorktreeRoot: t.TempDir()})
	return NewServer(s, state, Config{UIToken: token, Fleet: fleet, Workers: workerlink.NewServer(s, fleet),
		Helper: http.NotFoundHandler(), Desk: http.NotFoundHandler(), Extensions: installed}, http.NotFoundHandler(), http.NotFoundHandler())
}

// sortedRoutes are patterns sorted by path, then method, so a path's
// routes sit together.
func sortedRoutes(patterns []string) []string {
	sorted := slices.Clone(patterns)
	slices.SortFunc(sorted, func(a, b string) int {
		methodA, pathA, _ := strings.Cut(a, " ")
		methodB, pathB, _ := strings.Cut(b, " ")
		return cmp.Or(strings.Compare(pathA, pathB), strings.Compare(methodA, methodB))
	})
	return sorted
}

type ownedRoute struct{ owner, access string }

// routeRecorder is a routeTable that passes each route on to mux, so a
// clash panics as it does when serving, and notes its pattern and, for a
// mounted one, its owner and access kind.
type routeRecorder struct {
	mux      *http.ServeMux
	patterns []string
	owners   map[string]ownedRoute
}

func (r *routeRecorder) Handle(pattern string, handler http.Handler) {
	r.mux.Handle(pattern, handler)
	r.patterns = append(r.patterns, pattern)
}

func (r *routeRecorder) HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request)) {
	r.Handle(pattern, http.HandlerFunc(handler))
}

func (r *routeRecorder) mounted(owner, pattern, access string) {
	r.owners[pattern] = ownedRoute{owner: owner, access: access}
}

var _ mountNoter = (*routeRecorder)(nil)
