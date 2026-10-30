package web

import (
	"os"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/platform/sessions/sessionstest"
)

// Sessions trust their worktree in Claude Code's config (package engines):
// tests keep their own config, never the one of whoever runs them. A
// session a test leaves running fails the run (sessionstest).
func TestMain(m *testing.M) {
	dir, _ := os.MkdirTemp("", "agentry-claude-config-")
	os.Setenv("CLAUDE_CONFIG_DIR", dir)
	code := sessionstest.Run(m)
	os.RemoveAll(dir)
	os.Exit(code)
}
