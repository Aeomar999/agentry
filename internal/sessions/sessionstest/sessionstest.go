// Package sessionstest keeps the sessions a test run starts to that run.
// A session's host outlives the program that started it (package
// sessions), so a test that leaves one running leaves a process behind for
// good. Each test gets a sessions folder of its own, stopped when it ends
// (Isolate), and the run fails if anything it started still runs (Run).
package sessionstest

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/sessions"
)

// base is the run's own sessions folder, set by package sessions.
var base = sessions.Dir()

// Run runs the tests, in place of m.Run in a TestMain. After them it stops
// every session they left and kills what still runs from them; anything
// left that way fails the run, named on stderr.
func Run(m *testing.M) int {
	code := m.Run()
	if left := leftovers(); len(left) > 0 {
		fmt.Fprintf(os.Stderr, "sessionstest: the tests left %d running:\n  %s\n", len(left), strings.Join(left, "\n  "))
		if code == 0 {
			code = 1
		}
	}
	stopAll()
	killStartedHere(base)
	if filepath.Base(base) == fmt.Sprintf("agentry-test-%d", os.Getpid()) {
		os.RemoveAll(base)
	}
	return code
}

var isolated sync.Map // the tests that have their own folder

// Isolate gives the test a sessions folder of its own, inside the run's,
// and stops every session in it when the test ends, passed or failed.
// Calling it again in the same test does nothing.
func Isolate(t testing.TB) {
	t.Helper()
	if _, done := isolated.LoadOrStore(t, true); done {
		return
	}
	os.MkdirAll(base, 0o700)
	dir, err := os.MkdirTemp(base, "") // short: a socket's path is limited
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv(sessions.DirEnv, dir)
	t.Cleanup(func() {
		sessions.StopAll()
		isolated.Delete(t)
	})
}

// leftovers are the sessions and processes still running from this run,
// given a moment to finish going.
func leftovers() []string {
	var left []string
	for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(100 * time.Millisecond) {
		left = nil
		eachFolder(func(dir string) {
			for name, pid := range sessions.All() {
				left = append(left, fmt.Sprintf("session %s (host pid %d) in %s", name, pid, dir))
			}
		})
		left = append(left, startedHere(base)...)
		if len(left) == 0 {
			break
		}
	}
	sort.Strings(left)
	return left
}

func stopAll() { eachFolder(func(string) { sessions.StopAll() }) }

// eachFolder runs do with the sessions folder set to the run's, then to
// each test's in turn.
func eachFolder(do func(dir string)) {
	defer os.Setenv(sessions.DirEnv, os.Getenv(sessions.DirEnv))
	dirs := []string{base}
	entries, _ := os.ReadDir(base)
	for _, entry := range entries {
		if entry.IsDir() {
			dirs = append(dirs, filepath.Join(base, entry.Name()))
		}
	}
	for _, dir := range dirs {
		os.Setenv(sessions.DirEnv, dir)
		do(dir)
	}
}
