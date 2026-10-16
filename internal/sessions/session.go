// Package sessions runs each task's agent in its own tmux server, on its own
// socket, so nothing touches the user's tmux and a crash is visible as a dead
// pane (proofs 03 and 04).
package sessions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// The size a session starts at. A viewer can resize it to fit their screen
// (internal/terminal), and the program in it sees the change as in any terminal.
const (
	Columns = 120
	Rows    = 36
)

// Pane is the one tmux target each task's server has.
const Pane = "agent"

// Spec is one agent turn's session.
type Spec struct {
	Socket  string            // the tmux server's socket name, from SocketFor
	Dir     string            // where the command starts
	Env     map[string]string // added to the command's environment
	Command []string
}

func SocketFor(task string) string { return "agentry-" + task }

// Start replaces whatever the task's server was running with a new session.
// remain-on-exit is set before the command starts, so even a command that
// exits at once leaves a dead pane behind instead of vanishing (proof 03).
func Start(spec Spec) error {
	Stop(spec.Socket)
	removeStaleSocket(spec.Socket)

	// focus-events lets programs such as Claude Code know when the terminal
	// has focus; without it they print a notice asking for it.
	args := []string{"start-server", ";", "set-option", "-g", "remain-on-exit", "on", ";",
		"set-option", "-g", "focus-events", "on", ";",
		"new-session", "-d", "-s", Pane, "-x", fmt.Sprint(Columns), "-y", fmt.Sprint(Rows), "-c", spec.Dir}
	for _, name := range sortedKeys(spec.Env) {
		args = append(args, "-e", name+"="+spec.Env[name])
	}
	args = append(args, spec.Command...)
	args = append(args, ";", "set-option", "-t", Pane, "window-size", "manual")
	if _, err := tmux(spec.Socket, args...); err != nil {
		return fmt.Errorf("starting the session: %w", err)
	}
	return nil
}

// IsRunning is true while the session exists and its process is alive.
func IsRunning(socket string) bool {
	output, err := tmux(socket, "display-message", "-p", "-t", Pane, "#{pane_dead}")
	return err == nil && output == "0"
}

// Exists is true while the task's tmux server has the session, alive or dead.
func Exists(socket string) bool {
	_, err := tmux(socket, "has-session", "-t", Pane)
	return err == nil
}

// Screen is the session's visible text, without colours.
func Screen(socket string) string {
	output, _ := tmux(socket, "capture-pane", "-p", "-t", Pane)
	return output
}

// Stop ends the task's tmux server and everything in it.
func Stop(socket string) {
	tmux(socket, "kill-server")
}

// removeStaleSocket deletes the socket file a killed tmux server can leave
// behind; a new server on it dies at once, "server exited unexpectedly" (proof 04).
func removeStaleSocket(socket string) {
	dir := os.Getenv("TMUX_TMPDIR")
	if dir == "" {
		dir = "/tmp"
	}
	os.Remove(filepath.Join(dir, fmt.Sprintf("tmux-%d", os.Getuid()), socket))
}

func tmux(socket string, args ...string) (string, error) {
	output, err := exec.Command("tmux", append([]string{"-L", socket}, args...)...).CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		return text, fmt.Errorf("tmux %s: %v: %s", args[0], err, text)
	}
	return text, nil
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
