package sessions

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testSocket(t *testing.T) string {
	socket := fmt.Sprintf("agentry-test-%d", time.Now().UnixNano())
	t.Cleanup(func() { Stop(socket) })
	return socket
}

func waitFor(t *testing.T, what string, condition func() bool) {
	t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if condition() {
			return
		}
	}
	t.Fatalf("timed out waiting for %s", what)
}

func TestASessionRunsWithItsEnvironmentAndShowsACrash(t *testing.T) {
	socket, dir := testSocket(t), t.TempDir()
	err := Start(Spec{Socket: socket, Dir: dir, Env: map[string]string{"AGENTRY_TASK": "task-1", "PORT": "7901"},
		Command: []string{"sh", "-c", `echo "task=$AGENTRY_TASK port=$PORT dir=$(pwd)"; sleep 30`}})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the output", func() bool { return strings.Contains(Screen(socket), "task=task-1 port=7901") })
	if !IsRunning(socket) || !strings.Contains(Screen(socket), "dir="+dir) {
		t.Errorf("running %v, screen %q", IsRunning(socket), Screen(socket))
	}

	// Kill the process, not the session: the pane stays, dead (proof 03).
	tmux(socket, "send-keys", "-t", Pane, "C-c")
	waitFor(t, "the pane to die", func() bool { return !IsRunning(socket) })
	if !Exists(socket) {
		t.Error("with remain-on-exit the dead pane should still exist")
	}
}

func TestStartReplacesTheTurnBefore(t *testing.T) {
	socket, dir := testSocket(t), t.TempDir()
	Start(Spec{Socket: socket, Dir: dir, Command: []string{"sh", "-c", "echo first; sleep 30"}})
	waitFor(t, "the first turn", func() bool { return strings.Contains(Screen(socket), "first") })
	Start(Spec{Socket: socket, Dir: dir, Command: []string{"sh", "-c", "echo second; sleep 30"}})
	waitFor(t, "the second turn", func() bool { return strings.Contains(Screen(socket), "second") })
	if strings.Contains(Screen(socket), "first") {
		t.Error("the first turn's session should be gone")
	}
}

// Proof 04: a socket file left by a killed server makes the next one die.
func TestAStaleSocketIsRemoved(t *testing.T) {
	socket, dir := testSocket(t), t.TempDir()
	stale := filepath.Join("/tmp", fmt.Sprintf("tmux-%d", os.Getuid()), socket)
	os.MkdirAll(filepath.Dir(stale), 0o700)
	if err := os.WriteFile(stale, nil, 0o600); err != nil {
		t.Skip("can't write a stale socket:", err)
	}
	if err := Start(Spec{Socket: socket, Dir: dir, Command: []string{"sleep", "30"}}); err != nil || !IsRunning(socket) {
		t.Errorf("start over a stale socket: %v, running %v", err, IsRunning(socket))
	}
}

func TestNoServerMeansNotRunning(t *testing.T) {
	socket := testSocket(t)
	if IsRunning(socket) || Exists(socket) {
		t.Error("a socket with no server is neither running nor existing")
	}
}
