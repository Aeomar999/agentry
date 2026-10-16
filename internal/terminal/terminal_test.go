package terminal

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"gitlab.com/dev.chempa/agentry/internal/sessions"
)

func TestUnescape(t *testing.T) {
	cases := map[string][]byte{
		`plain text`:           []byte("plain text"),
		`\033[32mgreen\033[0m`: []byte("\x1b[32mgreen\x1b[0m"),
		`line\015\012next`:     []byte("line\r\nnext"),
		`back\134slash`:        []byte(`back\slash`),
		`ends with \01`:        []byte(`ends with \01`), // too short to be an escape
	}
	for escaped, want := range cases {
		if got := unescape(escaped); !bytes.Equal(got, want) {
			t.Errorf("unescape(%q) = %q, want %q", escaped, got, want)
		}
	}
}

// watcher is one browser tab: it collects what the server sends.
type watcher struct {
	conn   *websocket.Conn
	screen strings.Builder
	state  map[string]any
	t      *testing.T
}

func connectWatcher(t *testing.T, ctx context.Context, url string) *watcher {
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	conn.SetReadLimit(4 << 20)
	return &watcher{conn: conn, t: t}
}

// readUntil reads until the screen contains text or the state matches.
func (w *watcher) readUntil(ctx context.Context, done func() bool) {
	w.t.Helper()
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for !done() {
		kind, message, err := w.conn.Read(ctx)
		if err != nil {
			w.t.Fatalf("waiting: %v; screen so far %q, state %v", err, w.screen.String(), w.state)
		}
		if kind == websocket.MessageBinary {
			w.screen.Write(message)
		} else {
			json.Unmarshal(message, &w.state)
		}
	}
}

func (w *watcher) send(ctx context.Context, kind websocket.MessageType, message string) {
	w.conn.Write(ctx, kind, []byte(message))
}

// Proof 04's rules: everyone sees the same output, only the one in control types.
func TestOnlyTheViewerInControlCanType(t *testing.T) {
	task := fmt.Sprintf("terminal-test-%d", time.Now().UnixNano())
	socket := sessions.SocketFor(task)
	defer sessions.Stop(socket)
	if err := sessions.Start(sessions.Spec{Socket: socket, Dir: t.TempDir(), Command: []string{"sh", "-c", "echo agent-ready; cat"}}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)

	mux := http.NewServeMux()
	mux.Handle("GET /tasks/{task}/terminal", NewHub().Handler())
	server := httptest.NewServer(mux)
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/tasks/" + task + "/terminal"

	ctx := context.Background()
	alice, bob := connectWatcher(t, ctx, url), connectWatcher(t, ctx, url)
	defer alice.conn.CloseNow()
	defer bob.conn.CloseNow()
	for _, w := range []*watcher{alice, bob} {
		w.readUntil(ctx, func() bool {
			return strings.Contains(w.screen.String(), "agent-ready") && w.state["viewers"] == float64(2)
		})
	}

	alice.send(ctx, websocket.MessageBinary, "ignored-while-watching\r")
	alice.send(ctx, websocket.MessageText, `{"type":"take"}`)
	alice.readUntil(ctx, func() bool { return alice.state["controller"] == alice.state["you"] })
	bob.send(ctx, websocket.MessageText, `{"type":"take"}`) // refused: alice has it
	alice.send(ctx, websocket.MessageBinary, "typed-by-alice\r")

	for _, w := range []*watcher{alice, bob} {
		w.readUntil(ctx, func() bool { return strings.Count(w.screen.String(), "typed-by-alice") >= 2 }) // the echo and cat's copy
		if strings.Contains(w.screen.String(), "ignored-while-watching") {
			t.Error("keys typed while watching must be dropped")
		}
	}
	if bob.state["controller"] == bob.state["you"] {
		t.Error("bob took control while alice had it")
	}

	// When the session ends, viewers are disconnected, to reconnect to the next turn's.
	sessions.Stop(socket)
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	for {
		if _, _, err := bob.conn.Read(ctx); err != nil {
			if ctx.Err() != nil {
				t.Error("the viewer was not disconnected when the session ended")
			}
			break
		}
	}
}

func TestNoSessionNoWatch(t *testing.T) {
	mux := http.NewServeMux()
	mux.Handle("GET /tasks/{task}/terminal", NewHub().Handler())
	server := httptest.NewServer(mux)
	defer server.Close()
	response, _ := http.Get(server.URL + "/tasks/task-none/terminal")
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("got %d", response.StatusCode)
	}
}
