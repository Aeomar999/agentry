package engines_test

import (
	"os"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/platform/engines"
)

func TestOnlyParentSessionMarkersAreDropped(t *testing.T) {
	markers := []string{"CLAUDECODE", "CLAUDE_CODE_CHILD_SESSION", "CLAUDE_CODE_MESSAGING_TOKEN", "CODEX_SANDBOX"}
	kept := []string{"CLAUDE_CONFIG_DIR", "CLAUDE_CODE_USE_BEDROCK", "CODEX_HOME", "ANTHROPIC_API_KEY", "AGENTRY_URL"}
	for _, kv := range os.Environ() { // whoever runs the tests may be in a session
		if name, _, _ := strings.Cut(kv, "="); engines.IsParentSessionVar(name) {
			t.Setenv(name, "") // restored after the test
			os.Unsetenv(name)
		}
	}
	for _, name := range append(markers, kept...) {
		t.Setenv(name, "1")
	}

	removed := engines.DropParentSession()
	slices.Sort(markers)
	if !slices.Equal(removed, markers) {
		t.Errorf("removed %v, want %v", removed, markers)
	}
	for _, name := range markers {
		if _, set := os.LookupEnv(name); set {
			t.Errorf("%s is still set", name)
		}
	}
	for _, name := range kept {
		if os.Getenv(name) != "1" {
			t.Errorf("%s, the user's own, was removed", name)
		}
	}
	if again := engines.DropParentSession(); len(again) != 0 {
		t.Errorf("nothing is left to remove, but it removed %v", again)
	}
}
