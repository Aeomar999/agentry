// Package workerservice keeps `agentry worker run` running in the
// background on a worker machine: started at login, restarted if it stops,
// its output in one log file. It uses what the machine has: a systemd user
// service on Linux, a launchd agent on macOS, or, where there is neither
// (a container, say), a plain background process.
package workerservice

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

// Spec is what the service runs.
type Spec struct {
	Binary string // agentry
	Dir    string // the worker's folder, passed as -dir
	Log    string // where its output goes
	Path   string // the PATH it runs with: the one it was started from, so it finds claude, git and tmux
}

func (s Spec) command() []string { return []string{s.Binary, "worker", "run", "-dir", s.Dir} }

// Service is one way of running the worker in the background.
type Service interface {
	Kind() string // as `agentry worker status` names it
	// Start writes the service's definition (again, so a new PATH or binary
	// takes effect) and starts it, or restarts it if it was running.
	Start(spec Spec) error
	Stop() error
	Running() bool
	// Remove stops it and deletes its definition.
	Remove() error
}

// ForThisMachine picks the service this machine supports.
func ForThisMachine(dir string) Service {
	switch {
	case runtime.GOOS == "darwin":
		// launchd won't take an agent from a session with no login, as over
		// SSH; then it runs in the background, until a start from the Mac
		// itself sets the agent up.
		return fallback{first: launchd{}, second: background{pidFile: dir + "/worker.pid"}}
	case hasSystemdUser():
		return systemd{}
	default:
		return background{pidFile: dir + "/worker.pid"}
	}
}

func hasSystemdUser() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	return exec.Command("systemctl", "--user", "show-environment").Run() == nil
}

func run(name string, args ...string) error {
	output, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return errors.New(name + " " + args[0] + ": " + string(output))
	}
	return nil
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
