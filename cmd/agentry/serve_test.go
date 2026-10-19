package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/engines"
	"gitlab.com/dev.chempa/agentry/internal/hostaddrs"
)

// Started from inside a Claude Code session, serve, open and the worker
// drop that session's markers and say which, by name.
func TestStartingDropsAParentSessionAndSaysWhat(t *testing.T) {
	for _, kv := range os.Environ() { // whoever runs the tests may be in a session
		if name, _, _ := strings.Cut(kv, "="); engines.IsParentSessionVar(name) {
			t.Setenv(name, "") // restored after the test
			os.Unsetenv(name)
		}
	}
	var quiet strings.Builder
	if dropParentSession(&quiet); quiet.Len() != 0 {
		t.Errorf("nothing to remove, but it said %q", quiet.String())
	}

	t.Setenv("CLAUDECODE", "1")
	t.Setenv("CLAUDE_CODE_CHILD_SESSION", "secret-value")
	t.Setenv("CLAUDE_CONFIG_DIR", "/claude-config")
	var said strings.Builder
	dropParentSession(&said)
	want := "agentry: removed variables inherited from a Claude Code or Codex session: CLAUDECODE, CLAUDE_CODE_CHILD_SESSION\n"
	if said.String() != want {
		t.Errorf("said %q, want %q", said.String(), want)
	}
	if _, set := os.LookupEnv("CLAUDE_CODE_CHILD_SESSION"); set || os.Getenv("CLAUDE_CONFIG_DIR") == "" {
		t.Error("it should remove the markers and keep the user's configuration")
	}
}

// Hooks and the desk call the server on loopback when it listens on every
// address, as it does in a container.
func TestLocalURL(t *testing.T) {
	for addr, want := range map[string]string{
		"0.0.0.0:7800":   "http://127.0.0.1:7800",
		"127.0.0.1:7801": "http://127.0.0.1:7801",
		"[::]:7802":      "http://127.0.0.1:7802",
	} {
		tcp, _ := net.ResolveTCPAddr("tcp", addr)
		if got := localURL(tcp, "http"); got != want {
			t.Errorf("localURL(%s) = %s, want %s", addr, got, want)
		}
	}
}

// A page on this machine opens the server at localhost, unless it listens on
// one address that isn't loopback.
func TestLocalhostURL(t *testing.T) {
	for addr, want := range map[string]string{
		"0.0.0.0:7800":     "http://localhost:7800",
		"127.0.0.1:7801":   "http://localhost:7801",
		"[::]:7802":        "http://localhost:7802",
		"192.168.1.5:7803": "",
	} {
		tcp, _ := net.ResolveTCPAddr("tcp", addr)
		if got := localhostURL(tcp, "http"); got != want {
			t.Errorf("localhostURL(%s) = %s, want %s", addr, got, want)
		}
	}
}

// Listening on every address, serve prints a link for each of this machine's
// own addresses other machines reach; listening on one, it prints none.
func TestNetworkURLs(t *testing.T) {
	one, _ := net.ResolveTCPAddr("tcp", "127.0.0.1:7800")
	if urls := networkURLs(one, "http"); urls != nil {
		t.Errorf("on one address: %v", urls)
	}
	every, _ := net.ResolveTCPAddr("tcp", "0.0.0.0:7800")
	for _, url := range networkURLs(every, "http") {
		if !strings.HasPrefix(url, "http://") || !strings.HasSuffix(url, ":7800") || strings.Contains(url, "127.0.0.1") {
			t.Errorf("a network URL: %s", url)
		}
	}
	var want []string
	for _, ip := range hostaddrs.Network() {
		want = append(want, "http://"+ip.String()+":7800")
	}
	if got := networkURLs(every, "http"); !slices.Equal(got, want) {
		t.Errorf("networkURLs = %v, want one for each network address: %v", got, want)
	}
}

// A log with streams of a feature this build doesn't have says so once per
// prefix at start, not once per event; an unknown type alone says nothing.
func TestStartingSaysOncePerSkippedPrefix(t *testing.T) {
	var said strings.Builder
	reportSkipped(&said, []current.Unknown{
		{Prefix: "idea-", Streams: 12, Events: 31, Last: 48211},
		{Prefix: "note-", Streams: 1, Events: 2, Last: 48100},
		{Type: "git.pr_opened", Events: 4, Last: 48190},
	})
	want := "agentry: skipped 31 events in 12 idea- streams, from a feature this build doesn't have; the log keeps them\n" +
		"agentry: skipped 2 events in 1 note- streams, from a feature this build doesn't have; the log keeps them\n"
	if said.String() != want {
		t.Errorf("said %q, want %q", said.String(), want)
	}
	said.Reset()
	if reportSkipped(&said, []current.Unknown{}); said.Len() != 0 {
		t.Errorf("nothing skipped, but it said %q", said.String())
	}
}

// servedOnce starts serve on dataDir, waits until it is running and stops
// it cleanly. It returns what it logged.
func servedOnce(t *testing.T, dataDir string) string {
	t.Helper()
	var logged, out syncBuffer
	previous := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	defer slog.SetDefault(previous)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- serve(ctx, serveOptions{address: "127.0.0.1:0", repo: t.TempDir(), dataDir: dataDir}, &out)
	}()
	for deadline := time.Now().Add(10 * time.Second); !strings.Contains(out.String(), "Agentry is running"); time.Sleep(20 * time.Millisecond) {
		if time.Now().After(deadline) {
			cancel()
			t.Fatalf("serve printed:\n%s", out.String())
		}
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serve: %v", err)
	}
	return logged.String()
}

// serve keeps the state's snapshot in the data folder, readable by its
// owner alone: it removes the temporary files of writes that never
// finished, writes a snapshot at a clean shutdown, and the next start goes
// on from it with no fallback.
func TestServeKeepsTheSnapshotAcrossARestart(t *testing.T) {
	dataDir := t.TempDir()
	leftover := filepath.Join(dataDir, snapshotFile+".tmp-123")
	if err := os.WriteFile(leftover, []byte("half a snapshot"), 0o600); err != nil {
		t.Fatal(err)
	}
	if said := servedOnce(t, dataDir); !strings.Contains(said, "there is no snapshot") {
		t.Errorf("the first start should say there is no snapshot; it logged:\n%s", said)
	}
	if _, err := os.Stat(leftover); !os.IsNotExist(err) {
		t.Errorf("the unfinished write is still there: %v", err)
	}
	info, err := os.Stat(filepath.Join(dataDir, snapshotFile))
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Errorf("the snapshot's mode is %v, want 0600", mode)
	}
	if said := servedOnce(t, dataDir); strings.Contains(said, "replayed") {
		t.Errorf("the second start didn't go on from the snapshot; it logged:\n%s", said)
	}
}
