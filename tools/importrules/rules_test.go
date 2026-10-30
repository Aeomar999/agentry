// Package importrules checks that every import of Agentry's own packages
// points down, by the rules of docs/design/repo-organization.md, section 3.
// It holds only tests; go test ./... runs them.
package importrules

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os/exec"
	"slices"
	"strings"
	"testing"
)

const module = "gitlab.com/dev.chempa/agentry/"

// A layer is a row of section 3's table.
type layer int

const (
	unplaced   layer = iota
	platform         // internal/platform/*
	events           // internal/core/events
	domainRoot       // internal/domain/<area>, the model
	framework        // internal/extensions, the framework
	httpapi          // internal/core/httpapi
	coreState        // internal/core/{store,current,rules}
	runtime          // internal/runtime/*
	surface          // internal/domain/<area>/<sub> and internal/extensions/<name>
	top              // internal/web, internal/mcp
	command          // cmd/*, tools/*
	testkit          // internal/testkit/*
)

// newPath is section 4's table: each package's path today, and where its
// move puts it. A package's layer comes from its new path. Step 11, when
// every package is where it belongs, drops this table and reads the path.
var newPath = map[string]string{
	"internal/platform/client":                "internal/platform/client",
	"internal/platform/engines":               "internal/platform/engines",
	"internal/platform/exactgob":              "internal/platform/exactgob",
	"internal/platform/hookcall":              "internal/platform/hookcall",
	"internal/platform/hostaddrs":             "internal/platform/hostaddrs",
	"internal/platform/launch":                "internal/platform/launch",
	"internal/platform/pinned":                "internal/platform/pinned",
	"internal/platform/procs":                 "internal/platform/procs",
	"internal/platform/schema":                "internal/platform/schema",
	"internal/platform/sessions":              "internal/platform/sessions",
	"internal/platform/sessions/sessionstest": "internal/platform/sessions/sessionstest",
	"internal/platform/terminal":              "internal/platform/terminal",
	"internal/platform/tlsca":                 "internal/platform/tlsca",
	"internal/platform/workspace":             "internal/platform/workspace",
	"internal/platform/worktrees":             "internal/platform/worktrees",

	"internal/events":  "internal/core/events",
	"internal/store":   "internal/core/store",
	"internal/current": "internal/core/current",
	"internal/rules":   "internal/core/rules",
	"internal/httpapi": "internal/core/httpapi",

	"internal/agents":      "internal/domain/agents",
	"internal/tasks":       "internal/domain/tasks",
	"internal/workers":     "internal/domain/workers",
	"internal/connections": "internal/domain/connections",
	"internal/policy":      "internal/domain/policy",
	"internal/settings":    "internal/domain/settings",
	"internal/sharedfiles": "internal/domain/sharedfiles",

	"internal/tasks/taskstest": "internal/domain/tasks/taskstest",
	"internal/waitlist":        "internal/domain/tasks/waitlist",
	"internal/workerlink":      "internal/domain/workers/workerlink",
	"internal/workerservice":   "internal/domain/workers/workerservice",
	"internal/hooks":           "internal/domain/policy/hooks",
	"internal/settingsfile":    "internal/domain/settings/settingsfile",
	"internal/webpush":         "internal/domain/notifications/webpush",

	"internal/extensions": "internal/extensions",
	"internal/ideas":      "internal/extensions/ideas",
	"internal/schedules":  "internal/extensions/schedules",

	"internal/coordinator": "internal/runtime/coordinator",
	"internal/turns":       "internal/runtime/turns",
	"internal/watchdog":    "internal/runtime/watchdog",
	"internal/archive":     "internal/runtime/archive",
	"internal/machines":    "internal/runtime/machines",
	"internal/desk":        "internal/runtime/desk",

	"internal/web": "internal/web",
	"internal/mcp": "internal/mcp",

	"internal/testkit/storetest": "internal/testkit/storetest",
	"internal/testkit/deeptest":  "internal/testkit/deeptest",

	"tools/speccheck":   "tools/speccheck",
	"tools/packaging":   "tools/packaging",
	"tools/importrules": "tools/importrules",
	"tools/relayout":    "tools/relayout",
	"cmd/agentry":       "cmd/agentry",
}

// layerOf gives a package's layer, and for a surface the folder whose other
// subpackages it may import: "internal/workerlink" is a surface in
// "internal/domain/workers".
func layerOf(path string) (layer, string) {
	moved, ok := newPath[path]
	if !ok {
		return unplaced, ""
	}
	parts := strings.Split(moved, "/")
	switch {
	case parts[0] == "cmd" || parts[0] == "tools":
		return command, ""
	case moved == "internal/web" || moved == "internal/mcp":
		return top, ""
	case parts[1] == "platform":
		return platform, ""
	case moved == "internal/core/events":
		return events, ""
	case moved == "internal/core/httpapi":
		return httpapi, ""
	case parts[1] == "core":
		return coreState, ""
	case parts[1] == "runtime":
		return runtime, ""
	case parts[1] == "testkit":
		return testkit, ""
	case moved == "internal/extensions":
		return framework, ""
	case parts[1] == "extensions":
		return surface, strings.Join(parts[:3], "/")
	case parts[1] == "domain" && len(parts) == 3:
		return domainRoot, ""
	case parts[1] == "domain":
		return surface, strings.Join(parts[:3], "/")
	}
	return unplaced, ""
}

// mayImport is section 3's table, row by row: the layers each layer may
// import. Testkit, never imported by code, is TestTestkitIsOnlyForTests's.
var mayImport = map[layer][]layer{
	platform:   {platform},
	events:     {platform},
	domainRoot: {platform, events, domainRoot},
	framework:  {platform, events, domainRoot},
	httpapi:    {platform, events, domainRoot, framework},
	coreState:  {platform, events, domainRoot, framework, coreState},
	runtime:    {platform, events, domainRoot, framework, httpapi, coreState, runtime},
	surface:    {platform, events, domainRoot, framework, httpapi, coreState, runtime, surface},
	top:        {platform, events, domainRoot, framework, httpapi, coreState, runtime, surface},
	command:    {platform, events, domainRoot, framework, httpapi, coreState, runtime, surface, top, command},
	testkit:    {platform, events, domainRoot, framework, httpapi, coreState, runtime, surface, top, command},
}

// allowed says whether from may import to: to's layer is one from's row
// names, and a surface reaches another surface only in its own folder.
func allowed(from, to string) bool {
	fromLayer, fromFolder := layerOf(from)
	toLayer, toFolder := layerOf(to)
	if !slices.Contains(mayImport[fromLayer], toLayer) {
		return false
	}
	return fromLayer != surface || toLayer != surface || fromFolder == toFolder
}

type edge struct{ from, to string }

// known are the edges that break the rules today; one may be removed, never added.
// The paths are those of step 2; steps 8 and 9 rewrite them like any other path,
// to internal/extensions/ideas and internal/runtime/coordinator.
var known = []edge{{"internal/coordinator", "internal/ideas"}}

type pkg struct {
	path    string
	imports []string
}

// ourPackages lists every package of the module, each with the module's
// packages it imports outside its tests, paths relative to the module.
func ourPackages(t *testing.T) []pkg {
	t.Helper()
	cmd := exec.Command("go", "list", "-json", "./...")
	cmd.Dir = "../.."
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("go list: %v", err)
	}
	var pkgs []pkg
	dec := json.NewDecoder(bytes.NewReader(out))
	for {
		var listed struct {
			ImportPath string
			Imports    []string
		}
		if err := dec.Decode(&listed); errors.Is(err, io.EOF) {
			return pkgs
		} else if err != nil {
			t.Fatalf("reading go list: %v", err)
		}
		p := pkg{path: strings.TrimPrefix(listed.ImportPath, module)}
		for _, imp := range listed.Imports {
			if ours, ok := strings.CutPrefix(imp, module); ok {
				p.imports = append(p.imports, ours)
			}
		}
		pkgs = append(pkgs, p)
	}
}

func TestEveryImportPointsDown(t *testing.T) {
	seen := map[edge]bool{}
	for _, p := range ourPackages(t) {
		for _, to := range p.imports {
			e := edge{p.path, to}
			seen[e] = true
			if toLayer, _ := layerOf(to); toLayer == testkit || allowed(p.path, to) || slices.Contains(known, e) {
				continue
			}
			t.Errorf("%s imports %s, which is not below it (docs/design/repo-organization.md, section 3)", p.path, to)
		}
	}
	for _, e := range known {
		if !seen[e] {
			t.Errorf("%s no longer imports %s: take the edge off known", e.from, e.to)
		}
	}
}

func TestEveryPackageHasALayer(t *testing.T) {
	for _, p := range ourPackages(t) {
		if l, _ := layerOf(p.path); l == unplaced {
			t.Errorf("%s has no layer: put it under platform, core, domain, extensions, runtime or testkit, and add it to newPath", p.path)
		}
	}
}

func TestTestkitIsOnlyForTests(t *testing.T) {
	for _, p := range ourPackages(t) {
		for _, to := range p.imports {
			if l, _ := layerOf(to); l == testkit {
				t.Errorf("%s imports %s outside its tests; testkit is only for tests", p.path, to)
			}
		}
	}
}
