package sessions_test

import (
	"os"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/platform/sessions/sessionstest"
)

func TestMain(m *testing.M) { os.Exit(sessionstest.Run(m)) }
