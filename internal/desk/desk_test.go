package desk

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/sessions"
)

// The desk is Claude started with its prompt, allowed to run `agentry task`
// without asking, and able to find this agentry binary and the server.
func TestTheDeskStartsClaudeWithWhatItNeeds(t *testing.T) {
	bin, record := t.TempDir(), filepath.Join(t.TempDir(), "started")
	fake := "#!/bin/sh\n{ echo \"$AGENTRY_SERVER $AGENTRY_DESK_TOKEN\"; echo \"$PATH\"; printf '%s\\n' \"$@\"; pwd; } > " + record + "\nsleep 30\n"
	if err := os.WriteFile(filepath.Join(bin, "claude"), []byte(fake), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	socket = sessions.SocketFor(fmt.Sprintf("desk-test-%d", time.Now().UnixNano()))
	defer Stop()

	repo := t.TempDir()
	config := Config{Dir: repo, ServerURL: "http://127.0.0.1:7800", Token: "desk-secret", Binary: "/opt/agentry/bin/agentry"}
	if err := Start(config); err != nil {
		t.Fatal(err)
	}
	var started string
	for deadline := time.Now().Add(3 * time.Second); time.Now().Before(deadline) && started == ""; time.Sleep(50 * time.Millisecond) {
		content, _ := os.ReadFile(record)
		started = string(content)
	}
	for _, want := range []string{"http://127.0.0.1:7800 desk-secret", "/opt/agentry/bin:", "--append-system-prompt", "You are the desk of Agentry",
		"--permission-mode\ndefault", "--allowedTools\nBash(agentry task:*)", "--disallowedTools\nEdit\nWrite\nMultiEdit\nNotebookEdit", repo} {
		if !strings.Contains(started, want) {
			t.Errorf("the desk was started without %q:\n%s", want, started)
		}
	}

	// A running desk is left alone: opening it again doesn't restart Claude.
	os.Remove(record)
	if err := Start(config); err != nil || !sessions.IsRunning(Socket()) {
		t.Fatalf("second start: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if _, err := os.Stat(record); err == nil {
		t.Error("a running desk was started again")
	}
}
