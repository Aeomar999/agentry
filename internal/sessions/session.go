// Package sessions runs each task's agent in a session of its own: a
// terminal that a small host process keeps (host.go), so it runs on when
// Agentry restarts, a viewer can watch it and take control (package
// terminal), and a crash is visible as a dead session, its last screen kept
// (proofs 03 and 04). It works the same on Linux, macOS and Windows.
package sessions

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/procs"
)

// The size a session starts at. A viewer can resize it to fit their screen
// (internal/terminal), and the program in it sees the change as in any terminal.
const (
	Columns = 120
	Rows    = 36
)

// Spec is one session: an agent's turn, the desk, or a Claude beside a task.
type Spec struct {
	Socket  string            // the session's name, from SocketFor
	Dir     string            // where the command starts
	Env     map[string]string // added to the command's environment
	Command []string
}

// SocketFor is the name of a task's (or the desk's) session.
func SocketFor(task string) string { return "agentry-" + task }

// Start replaces whatever the session was running with the command, in a
// host of its own that outlives this process.
func Start(spec Spec) error {
	if liveUnderTest() {
		return errLiveUnderTest
	}
	Stop(spec.Socket)
	self, err := os.Executable()
	if err != nil {
		return err
	}
	os.MkdirAll(Dir(), 0o700)
	log, err := os.Create(filepath.Join(Dir(), spec.Socket+".log"))
	if err != nil {
		return err
	}
	defer log.Close()
	encoded, _ := json.Marshal(spec)
	host := exec.Command(self)
	host.Env = append(os.Environ(), hostEnv+"="+spec.Socket, specEnv+"="+string(encoded))
	host.Stdout, host.Stderr = log, log
	procs.Detach(host)
	if err := host.Start(); err != nil {
		return fmt.Errorf("starting the session: %w", err)
	}
	exited := make(chan struct{})
	go func() { host.Wait(); close(exited) }()
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		if Exists(spec.Socket) {
			return nil
		}
		select {
		case <-exited:
			said, _ := os.ReadFile(log.Name())
			return fmt.Errorf("starting the session: %s", strings.TrimSpace(string(said)))
		default:
		}
	}
	return fmt.Errorf("starting the session: its host didn't answer")
}

// IsRunning is true while the session exists and its program is alive.
func IsRunning(name string) bool {
	a, err := ask(name, request{Op: "status"})
	return err == nil && a.Alive
}

// Exists is true while the session has a host, its program alive or not.
func Exists(name string) bool {
	_, err := ask(name, request{Op: "status"})
	return err == nil
}

// Screen is the session's visible text, without colours.
func Screen(name string) string {
	a, _ := ask(name, request{Op: "screen"})
	return a.Text
}

// Stop ends the session, its program and everything that started.
func Stop(name string) {
	if liveUnderTest() {
		return
	}
	if a, err := ask(name, request{Op: "stop"}); err == nil {
		for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline) && procs.Alive(a.PID); {
			time.Sleep(20 * time.Millisecond)
		}
	}
	os.Remove(socketPath(name)) // a host that died left it behind
}

// Running are the tasks whose agent is alive on this machine, found by
// their sessions' sockets, so a worker that restarts still knows.
func Running() []string {
	entries, _ := os.ReadDir(Dir())
	var tasks []string
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".sock")
		task, isTask := strings.CutPrefix(name, "agentry-")
		if ok && isTask && IsRunning(name) {
			tasks = append(tasks, task)
		}
	}
	return tasks
}

var errLiveUnderTest = errors.New("a test can't use the live sessions folder: give it its own (sessionstest)")

// liveUnderTest is true when a test would reach the sessions of the
// Agentry that runs on this machine: tests keep to a folder of their own.
func liveUnderTest() bool { return testing.Testing() && Dir() == sessionsDir() }

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
