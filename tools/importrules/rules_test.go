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
	command          // cmd/**, tools/**: a program and the packages beneath it
	testkit          // internal/testkit/*
)

// layerOf gives a package's layer, read off its path as section 2 lays it
// out, and for a surface the folder whose other subpackages it may import:
// "internal/domain/workers/workerlink" is a surface in "internal/domain/workers".
// A path section 2 has no place for is unplaced.
func layerOf(path string) (layer, string) {
	parts := strings.Split(path, "/")
	switch {
	case (parts[0] == "cmd" || parts[0] == "tools") && len(parts) >= 2:
		return command, ""
	case parts[0] != "internal" || len(parts) < 2:
		return unplaced, ""
	case path == "internal/web" || path == "internal/mcp":
		return top, ""
	case parts[1] == "platform" && (len(parts) == 3 || len(parts) == 4):
		return platform, ""
	case path == "internal/core/events":
		return events, ""
	case path == "internal/core/httpapi":
		return httpapi, ""
	case slices.Contains([]string{"internal/core/store", "internal/core/current", "internal/core/rules"}, path):
		return coreState, ""
	case parts[1] == "runtime" && len(parts) == 3:
		return runtime, ""
	case parts[1] == "testkit" && len(parts) == 3:
		return testkit, ""
	case path == "internal/extensions":
		return framework, ""
	case parts[1] == "extensions" && len(parts) == 3:
		return surface, path
	case parts[1] == "domain" && len(parts) == 3:
		return domainRoot, ""
	case parts[1] == "domain" && len(parts) == 4:
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
var known []edge

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
			t.Errorf("%s has no layer: put it under platform, core, domain, extensions, runtime or testkit, where section 2 of docs/design/repo-organization.md puts it", p.path)
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
