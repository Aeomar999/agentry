package mcp

import (
	"os"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/platform/sessions/sessionstest"
)

// Tests here can start sessions: one left running fails the run.
func TestMain(m *testing.M) { os.Exit(sessionstest.Run(m)) }
