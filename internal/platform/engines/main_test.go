package engines_test

import (
	"os"
	"testing"
)

// Sessions trust their worktree in Claude Code's config (package engines):
// tests keep their own config, never the one of whoever runs them.
func TestMain(m *testing.M) {
	dir, _ := os.MkdirTemp("", "agentry-claude-config-")
	os.Setenv("CLAUDE_CONFIG_DIR", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
