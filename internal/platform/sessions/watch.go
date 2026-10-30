package sessions

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"
)

// Watch is one viewer's connection to a session: what it prints comes to
// onOutput, and what is typed goes to its program. Watches are many; the
// session is one.
type Watch struct {
	conn     net.Conn
	writing  sync.Mutex
	ended    chan struct{}
	stopOnce sync.Once
}

// Attach watches the session until it is stopped or the watch is closed.
// onDead is told when its program exits; the session stays, its screen kept.
func Attach(name string, onOutput func([]byte), onDead func()) (*Watch, error) {
	if liveUnderTest() {
		return nil, errLiveUnderTest
	}
	conn, err := net.DialTimeout("unix", socketPath(name), 2*time.Second)
	if err != nil {
		return nil, err
	}
	encoded, _ := json.Marshal(request{Op: "attach"})
	if err := writeFrame(conn, frameRequest, encoded); err != nil {
		conn.Close()
		return nil, err
	}
	conn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if kind, _, err := readFrame(conn); err != nil || kind != frameAnswer {
		conn.Close()
		return nil, fmt.Errorf("the session didn't let a viewer in")
	}
	conn.SetReadDeadline(time.Time{})
	w := &Watch{conn: conn, ended: make(chan struct{})}
	go func() {
		defer w.Close()
		for {
			kind, payload, err := readFrame(conn)
			if err != nil {
				return
			}
			switch kind {
			case frameOutput:
				onOutput(payload)
			case frameDead:
				if onDead != nil {
					onDead()
				}
			}
		}
	}()
	return w, nil
}

// Type sends keystrokes to the program exactly as given.
func (w *Watch) Type(keys []byte) { w.send(frameKeys, keys) }

// Resize sets the session's size; the program sees it as in any terminal.
func (w *Watch) Resize(cols, rows int) { w.send(frameSize, []byte(fmt.Sprintf("%d %d", cols, rows))) }

func (w *Watch) send(kind byte, payload []byte) {
	w.writing.Lock()
	defer w.writing.Unlock()
	writeFrame(w.conn, kind, payload)
}

// Close ends this watch only; the session runs on.
func (w *Watch) Close() {
	w.stopOnce.Do(func() {
		w.conn.Close()
		close(w.ended)
	})
}

// Ended is closed when the watch ends: closed, or the session stopped.
func (w *Watch) Ended() <-chan struct{} { return w.ended }

// Snapshot is the session's screen with its colours and the cursor, for a
// viewer joining.
func Snapshot(name string) []byte {
	a, _ := ask(name, request{Op: "snapshot"})
	return []byte(a.Text)
}

// Size is the session's size, in columns and rows.
func Size(name string) (cols, rows int) {
	a, err := ask(name, request{Op: "status"})
	if err != nil || a.Cols == 0 {
		return Columns, Rows
	}
	return a.Cols, a.Rows
}
