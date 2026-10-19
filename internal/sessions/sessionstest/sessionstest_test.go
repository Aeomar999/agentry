package sessionstest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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

// Two runs started from a shell that names a sessions folder each take one
// of their own: a run never stops what another left in the shared one, and
// removes only its own folder. Here the other run is this test binary
// again, running a test that starts and stops a session.
func TestARunLeavesTheSessionsOfAFolderItInherits(t *testing.T) {
	shared, err := os.MkdirTemp("", "") // short: a socket's path is limited
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(shared) })
	t.Setenv(sessions.DirEnv, shared)
	if err := sessions.Start(sessions.Spec{Socket: "agentry-other", Dir: t.TempDir(), Command: []string{"sleep", "30"}}); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { sessions.Stop("agentry-other") })

	self, _ := os.Executable()
	run := exec.Command(self, "-test.run=^TestAnIsolatedTestStopsItsSessionsWhenItEnds$")
	run.Env = os.Environ() // the shared folder with it
	if output, err := run.CombinedOutput(); err != nil {
		t.Errorf("the other run failed: %v %s", err, output)
	}
	if !sessions.IsRunning("agentry-other") {
		t.Error("the other run stopped a session in the folder it inherited")
	}
	own := filepath.Join(os.TempDir(), fmt.Sprintf("agentry-test-%d", run.Process.Pid))
	if _, err := os.Stat(own); !os.IsNotExist(err) {
		t.Errorf("the other run's own folder should be gone: %v", err)
	}
	if _, err := os.Stat(shared); err != nil {
		t.Errorf("the shared folder should stay: %v", err)
	}
}
