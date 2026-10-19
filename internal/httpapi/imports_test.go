package httpapi

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// httpapi is a leaf, as features is: of Agentry's own packages it may import
// only events, features and what features itself may import, so web and
// every feature's routes can use it without a cycle.
var mayImport = []string{"events", "features", "agents", "tasks", "workers", "connections", "settings", "client"}

func TestHTTPAPIImportsNoOtherCorePackage(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	const internal = "gitlab.com/dev.chempa/agentry/internal/"
	for _, dep := range strings.Fields(string(out)) {
		name, ours := strings.CutPrefix(dep, internal)
		if ours && name != "httpapi" && !slices.Contains(mayImport, name) {
			t.Errorf("httpapi imports %s; it may import only %v", dep, mayImport)
		}
	}
}
