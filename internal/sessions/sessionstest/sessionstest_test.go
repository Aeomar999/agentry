package sessionstest

import (
	"os"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/sessions"
)

func TestMain(m *testing.M) { os.Exit(Run(m)) }

func TestAnIsolatedTestStopsItsSessionsWhenItEnds(t *testing.T) {
	var dir string
	t.Run("a test that leaves a session", func(t *testing.T) {
		Isolate(t)
		Isolate(t) // again, as two helpers might
		dir = sessions.Dir()
		if dir == base || len(dir) <= len(base) {
			t.Fatalf("the test's folder %q should be inside the run's %q", dir, base)
		}
		if err := sessions.Start(sessions.Spec{Socket: "agentry-left", Dir: t.TempDir(), Command: []string{"sleep", "30"}}); err != nil {
			t.Fatal(err)
		}
	})
	if sessions.Dir() != base {
		t.Errorf("the run's folder should be back: %q", sessions.Dir())
	}
	t.Setenv(sessions.DirEnv, dir)
	if left := sessions.All(); len(left) != 0 {
		t.Errorf("left running: %v", left)
	}
}

func TestASessionLeftRunningIsFoundAndStopped(t *testing.T) {
	if err := sessions.Start(sessions.Spec{Socket: "agentry-forgotten", Dir: t.TempDir(), Command: []string{"sleep", "30"}}); err != nil {
		t.Fatal(err)
	}
	if left := leftovers(); len(left) == 0 {
		t.Fatal("a session left running wasn't found")
	}
	stopAll()
	if left := leftovers(); len(left) != 0 {
		t.Errorf("still running after stopping: %v", left)
	}
}
