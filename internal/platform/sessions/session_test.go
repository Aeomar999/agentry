package sessions

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
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

	// Kill the program, not the session: the session stays, dead, with its
	// last screen (proof 03).
	died := make(chan struct{}, 1)
	watch, err := Attach(socket, func([]byte) {}, func() { died <- struct{}{} })
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	watch.Type([]byte{3}) // Ctrl-C
	waitFor(t, "the program to die", func() bool { return !IsRunning(socket) })
	select {
	case <-died:
	case <-time.After(5 * time.Second):
		t.Error("the watch wasn't told the program died")
	}
	if !Exists(socket) || !strings.Contains(Screen(socket), "task=task-1") {
		t.Errorf("the dead session should still exist with its screen: %q", Screen(socket))
	}
}

// Agentry started from inside a Claude Code session must not hand that
// session's markers on: with them, Claude saves no transcript. What the user
// set up, and what Agentry sets, still reaches the program.
func TestASessionDoesNotInheritAParentEngineSession(t *testing.T) {
	markers := []string{"CLAUDECODE", "CLAUDE_CODE_CHILD_SESSION", "CLAUDE_CODE_MESSAGING_SOCKET", "CODEX_THREAD_ID"}
	for _, name := range markers {
		t.Setenv(name, "1")
	}
	t.Setenv("CLAUDE_CONFIG_DIR", "/claude-config")
	t.Setenv("AGENTRY_INHERITED", "kept")
	socket, dir := testSocket(t), t.TempDir()
	err := Start(Spec{Socket: socket, Dir: dir, Env: map[string]string{"AGENTRY_TASK": "task-1"},
		Command: []string{"sh", "-c", "env > env.txt; echo done; sleep 30"}})
	if err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the program", func() bool { return strings.Contains(Screen(socket), "done") })
	saved, err := os.ReadFile(filepath.Join(dir, "env.txt"))
	if err != nil {
		t.Fatal(err)
	}
	env := strings.Split(string(saved), "\n")
	for _, want := range []string{"CLAUDE_CONFIG_DIR=/claude-config", "AGENTRY_INHERITED=kept", "AGENTRY_TASK=task-1"} {
		if !slices.Contains(env, want) {
			t.Errorf("the session should have %s", want)
		}
	}
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if slices.Contains(markers, name) {
			t.Errorf("the session inherited %s", name)
		}
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

// Proof 04: a socket file left by a killed host doesn't stop the next.
func TestAStaleSocketIsRemoved(t *testing.T) {
	socket, dir := testSocket(t), t.TempDir()
	stale := socketPath(socket)
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

// A session outlives the program that started it, sizes like a terminal,
// and its snapshot carries colours and the cursor.
func TestASessionResizesAndSnapshots(t *testing.T) {
	socket, dir := testSocket(t), t.TempDir()
	if err := Start(Spec{Socket: socket, Dir: dir, Command: []string{"sh", "-c", `printf '\033[31mred\033[0m\n'; while true; do stty size; sleep 0.2; done`}}); err != nil {
		t.Fatal(err)
	}
	waitFor(t, "the size", func() bool { return strings.Contains(Screen(socket), "36 120") })
	watch, err := Attach(socket, func([]byte) {}, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer watch.Close()
	watch.Resize(100, 30)
	waitFor(t, "the new size", func() bool { return strings.Contains(Screen(socket), "30 100") })
	if cols, rows := Size(socket); cols != 100 || rows != 30 {
		t.Errorf("size %dx%d", cols, rows)
	}
	if snapshot := string(Snapshot(socket)); !strings.Contains(snapshot, "red") || !strings.Contains(snapshot, "\x1b[") {
		t.Errorf("the snapshot should keep the colours: %q", snapshot)
	}
	if !slices.Contains(Running(), strings.TrimPrefix(socket, "agentry-")) {
		t.Errorf("running: %v", Running())
	}
}

// A test that lost its own sessions folder fails, rather than start, find
// or stop the sessions of the Agentry running on this machine.
func TestATestCantUseTheLiveSessions(t *testing.T) {
	t.Setenv(DirEnv, "")
	socket := testSocket(t)
	err := Start(Spec{Socket: socket, Dir: t.TempDir(), Command: []string{"sleep", "30"}})
	if err == nil || !strings.Contains(err.Error(), "live sessions folder") {
		t.Errorf("start in the live folder: %v", err)
	}
	if _, err := Attach(socket, func([]byte) {}, nil); err == nil {
		t.Error("a test attached to a live session")
	}
	if _, err := os.Stat(filepath.Join(sessionsDir(), socket+".log")); err == nil {
		t.Error("a test wrote to the live sessions folder")
	}
}

// A host a test started ends when the test run does, even one killed: it
// would otherwise run on for good. Here the run is this test binary again,
// which starts a session and leaves at once.
func TestAHostEndsWithTheTestsThatStartedIt(t *testing.T) {
	socket := testSocket(t)
	if os.Getenv("AGENTRY_TEST_START_AND_LEAVE") != "" {
		t.Setenv(DirEnv, os.Getenv("AGENTRY_TEST_START_AND_LEAVE_IN")) // the first run's, where it looks
		if err := Start(Spec{Socket: os.Getenv("AGENTRY_TEST_START_AND_LEAVE"), Dir: t.TempDir(), Command: []string{"sleep", "30"}}); err != nil {
			t.Fatal(err)
		}
		os.Exit(0) // without stopping it
	}
	self, _ := os.Executable()
	run := exec.Command(self, "-test.run=^TestAHostEndsWithTheTestsThatStartedIt$")
	run.Env = append(os.Environ(), "AGENTRY_TEST_START_AND_LEAVE="+socket, "AGENTRY_TEST_START_AND_LEAVE_IN="+Dir())
	if output, err := run.CombinedOutput(); err != nil {
		t.Fatalf("the run that starts the session: %v %s", err, output)
	}
	if !Exists(socket) {
		t.Fatal("the session didn't start")
	}
	waitFor(t, "the host to end", func() bool { return !Exists(socket) })
}
