package extensions

import (
	"os/exec"
	"slices"
	"strings"
	"testing"
)

// extensions is a leaf: of Agentry's own packages it may import only these,
// none of which imports anything that imports extensions.
var mayImport = []string{"events", "agents", "tasks", "workers", "connections", "settings", "client"}

func TestExtensionsImportsNoOtherCorePackage(t *testing.T) {
	out, err := exec.Command("go", "list", "-deps", ".").Output()
	if err != nil {
		t.Fatal(err)
	}
	const internal = "gitlab.com/dev.chempa/agentry/internal/"
	for _, dep := range strings.Fields(string(out)) {
		name, ours := strings.CutPrefix(dep, internal)
		if ours && name != "extensions" && !slices.Contains(mayImport, name) {
			t.Errorf("extensions imports %s; it may import only %v", dep, mayImport)
		}
	}
}
