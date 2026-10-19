package desk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/sessions"
	"gitlab.com/dev.chempa/agentry/internal/sessions/sessionstest"
)

// The desk is Claude started with its prompt, allowed to run `agentry task`
// without asking, and able to find this agentry binary and the server.
func TestTheDeskStartsClaudeWithWhatItNeeds(t *testing.T) {
	record := fakeDesk(t)
	repo := t.TempDir()
	config := Config{Dir: repo, ServerURL: "http://127.0.0.1:7800", Token: "desk-secret", Binary: "/opt/agentry/bin/agentry", Theme: "light"}
	if err := Start(config); err != nil {
		t.Fatal(err)
	}
	started := startedWith(t, record)
	for _, want := range []string{"http://127.0.0.1:7800 desk-secret", "/opt/agentry/bin:", "--append-system-prompt", "You are the desk of Agentry",
		"--settings\n{\"theme\":\"light\"}", "--permission-mode\ndefault", "--session-id\n" + conversation, "--allowedTools\nBash(agentry task:*)", "--disallowedTools\nEdit\nWrite\nMultiEdit\nNotebookEdit", repo} {
		if !strings.Contains(started, want) {
			t.Errorf("the desk was started without %q:\n%s", want, started)
		}
	}
	saveTranscript(t)

	// A running desk is left alone: opening it again doesn't restart Claude.
	os.Remove(record)
	if err := Start(config); err != nil || !sessions.IsRunning(Socket()) {
		t.Fatalf("second start: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(record); err == nil {
		t.Error("a running desk was started again")
	}

	// In another theme, it restarts in that one and continues the conversation.
	config.Theme = "dark"
	if err := Start(config); err != nil {
		t.Fatal(err)
	}
	started = startedWith(t, record)
	if !strings.Contains(started, `{"theme":"dark"}`) || !strings.Contains(started, "--resume\n"+conversation) {
		t.Errorf("after a theme change the desk should resume its own conversation in the new theme:\n%s", started)
	}
}

// A desk whose Claude exited (/exit, or a crash) comes back with the
// conversation Claude saved: Claude refuses --session-id for a saved one.
func TestADeskWhoseClaudeExitedResumesItsConversation(t *testing.T) {
	record := fakeDesk(t)
	config := Config{Dir: t.TempDir(), Binary: "/opt/agentry/bin/agentry", Theme: "light"}
	if err := Start(config); err != nil {
		t.Fatal(err)
	}
	startedWith(t, record)
	saveTranscript(t)
	exit(t, record)

	if err := Start(config); err != nil {
		t.Fatal(err)
	}
	started := startedWith(t, record)
	if !strings.Contains(started, "--resume\n"+conversation) || strings.Contains(started, "--session-id") {
		t.Errorf("the desk should resume its saved conversation:\n%s", started)
	}
}

// Nobody spoke in a desk that exited before its first message, so there is
// nothing to resume: it starts fresh with the same id.
func TestADeskWithNothingSavedStartsFreshAgain(t *testing.T) {
	record := fakeDesk(t)
	config := Config{Dir: t.TempDir(), Binary: "/opt/agentry/bin/agentry", Theme: "light"}
	if err := Start(config); err != nil {
		t.Fatal(err)
	}
	startedWith(t, record)
	exit(t, record)

	if err := Start(config); err != nil {
		t.Fatal(err)
	}
	started := startedWith(t, record)
	if !strings.Contains(started, "--session-id\n"+conversation) || strings.Contains(started, "--resume") {
		t.Errorf("the desk should start fresh with its own id:\n%s", started)
	}
}

// fakeDesk gives the test a desk of its own: its own sessions folder,
// socket and home, and a claude that records how it was started. It
// returns the record's path.
func fakeDesk(t *testing.T) string {
	sessionstest.Isolate(t)
	bin, record := t.TempDir(), filepath.Join(t.TempDir(), "started")
	// Written whole, then moved into place: the test reads it the moment it exists.
	fake := "#!/bin/sh\n{ echo \"$AGENTRY_SERVER $AGENTRY_DESK_TOKEN\"; echo \"$PATH\"; printf '%s\\n' \"$@\"; pwd; } > " + record +
		".part && mv " + record + ".part " + record + "\n" + sessionstest.Wait() + "\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("HOME", t.TempDir())
	socket = sessions.SocketFor(fmt.Sprintf("desk-test-%d", time.Now().UnixNano()))
	conversation, startedHere, runningTheme = "", false, ""
	t.Cleanup(Stop)
	return record
}

// startedWith waits for the fake claude's record of how it was started.
func startedWith(t *testing.T, record string) string {
	t.Helper()
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline); time.Sleep(50 * time.Millisecond) {
		if content, err := os.ReadFile(record); err == nil {
			return string(content)
		}
	}
	t.Fatal("claude wasn't started")
	return ""
}

// saveTranscript makes the desk's conversation look saved, as Claude saves
// it at the first message.
func saveTranscript(t *testing.T) {
	t.Helper()
	projects := filepath.Join(os.Getenv("HOME"), ".claude", "projects", "-repo")
	if err := os.MkdirAll(projects, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projects, conversation+".jsonl"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// exit ends the desk's Claude behind the desk's back, as /exit does, and
// clears the record for the next start.
func exit(t *testing.T, record string) {
	t.Helper()
	sessions.Stop(Socket())
	for deadline := time.Now().Add(3 * time.Second); sessions.IsRunning(Socket()); time.Sleep(50 * time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("the desk's claude didn't exit")
		}
	}
	os.Remove(record)
}
