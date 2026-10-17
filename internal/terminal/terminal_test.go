package terminal

import (
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
	mux.Handle("GET /tasks/{task}/terminal", NewHub().Handler(Attach))
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
	mux.Handle("GET /tasks/{task}/terminal", NewHub().Handler(Attach))
	server := httptest.NewServer(mux)
	defer server.Close()
	response, _ := http.Get(server.URL + "/tasks/task-none/terminal")
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("got %d", response.StatusCode)
	}
}

// The session has one size: a viewer may set it while nobody else is in
// control, and every viewer is told the new size.
func TestAViewerResizesTheSession(t *testing.T) {
	task := fmt.Sprintf("terminal-resize-%d", time.Now().UnixNano())
	socket := sessions.SocketFor(task)
	defer sessions.Stop(socket)
	if err := sessions.Start(sessions.Spec{Socket: socket, Dir: t.TempDir(), Command: []string{"sh", "-c", "cat"}}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(200 * time.Millisecond)
	mux := http.NewServeMux()
	mux.Handle("GET /tasks/{task}/terminal", NewHub().Handler(Attach))
	server := httptest.NewServer(mux)
	defer server.Close()
	url := "ws" + strings.TrimPrefix(server.URL, "http") + "/tasks/" + task + "/terminal"

	ctx := context.Background()
	alice, bob := connectWatcher(t, ctx, url), connectWatcher(t, ctx, url)
	defer alice.conn.CloseNow()
	defer bob.conn.CloseNow()
	sized := func(w *watcher, cols, rows int) func() bool {
		return func() bool { return w.state["cols"] == float64(cols) && w.state["rows"] == float64(rows) }
	}
	alice.readUntil(ctx, sized(alice, sessions.Columns, sessions.Rows))
	bob.readUntil(ctx, func() bool { return bob.state["viewers"] == float64(2) })

	alice.send(ctx, websocket.MessageText, `{"type":"resize","cols":90,"rows":50}`)
	bob.readUntil(ctx, sized(bob, 90, 50))
	if cols, rows := sessions.Size(socket); cols != 90 || rows != 50 {
		t.Errorf("the session is %dx%d, want 90x50", cols, rows)
	}

	// Once bob has control, alice's resize is ignored.
	bob.send(ctx, websocket.MessageText, `{"type":"take"}`)
	bob.readUntil(ctx, func() bool { return bob.state["controller"] == bob.state["you"] })
	alice.send(ctx, websocket.MessageText, `{"type":"resize","cols":60,"rows":20}`)
	bob.send(ctx, websocket.MessageText, `{"type":"resize","cols":9999,"rows":1}`) // clamped
	alice.readUntil(ctx, sized(alice, maxCols, minRows))
}
