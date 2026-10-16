package terminal

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/coder/websocket"

	"gitlab.com/dev.chempa/agentry/internal/sessions"
)

// Hub holds one watch per session being watched, a task's or the desk's:
// the control client and its viewers. A watch starts with the first viewer
// and ends with the last one, or when the session ends (each turn is a new
// session; the page reconnects).
type Hub struct {
	mu      sync.Mutex
	watches map[string]*viewers
}

func NewHub() *Hub { return &Hub{watches: map[string]*viewers{}} }

// Handler serves the websocket for the task in the path's {task}:
//
//	server → browser  binary: terminal bytes   text: {"type":"state", ..., "cols", "rows"}
//	browser → server  binary: keystrokes       text: {"type":"take"} / {"type":"release"}
//	                                                 {"type":"resize","cols":c,"rows":r}
//
// The session has one size, sent in every state, and each viewer shows it at
// that size. A viewer may resize it to fit their screen while nobody else is
// in control.
func (h *Hub) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.Serve(w, r, r.PathValue("task")) })
}

// Serve connects one viewer to the session named name: a task's, or the desk's.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, name string) {
	if !sessions.Exists(sessions.SocketFor(name)) {
		http.Error(w, "no session is running for "+name, http.StatusNotFound)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	conn.SetReadLimit(1 << 20)
	defer conn.CloseNow()

	watch, err := h.watch(name)
	if err != nil {
		conn.Close(websocket.StatusInternalError, err.Error())
		return
	}
	connect(r.Context(), conn, watch, func() { h.leave(name, watch) })
}

// watch returns the session's watch, attaching a control client if there is none.
func (h *Hub) watch(name string) (*viewers, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if watch, ok := h.watches[name]; ok && !watch.control.hasEnded() {
		return watch, nil
	} // else its session ended (as when the desk restarts) and the cleanup hasn't run yet
	watch := newViewers()
	c, err := attach(sessions.SocketFor(name), watch.broadcast)
	if err != nil {
		return nil, err
	}
	watch.control = c // set before anyone joins, so typeFrom always has it
	watch.cols, watch.rows = c.size()
	h.watches[name] = watch
	go func() {
		<-c.ended // the session ended: everyone reconnects to the next one
		h.mu.Lock()
		if h.watches[name] == watch {
			delete(h.watches, name)
		}
		h.mu.Unlock()
		watch.closeAll()
	}()
	return watch, nil
}

func (h *Hub) leave(name string, watch *viewers) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.watches[name] == watch {
		delete(h.watches, name)
	}
	watch.control.detach()
}

// connect runs one viewer until their tab closes or the session ends.
func connect(ctx context.Context, conn *websocket.Conn, watch *viewers, lastOneOut func()) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Join first so no output is missed, then send the current screen before
	// the live output that queued up meanwhile.
	me := watch.join()
	defer func() {
		if watch.leave(me) {
			lastOneOut()
		}
	}()
	if conn.Write(ctx, websocket.MessageBinary, watch.control.snapshot()) != nil {
		return
	}

	go func() {
		defer cancel()
		for {
			select {
			case data, open := <-me.output:
				if !open || conn.Write(ctx, websocket.MessageBinary, data) != nil {
					return
				}
			case state := <-me.state:
				if conn.Write(ctx, websocket.MessageText, state) != nil {
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		kind, message, err := conn.Read(ctx)
		if err != nil {
			return
		}
		if kind == websocket.MessageBinary {
			watch.typeFrom(me, message)
			continue
		}
		var request struct {
			Type       string
			Cols, Rows int
		}
		json.Unmarshal(message, &request)
		switch request.Type {
		case "take":
			watch.takeControl(me)
		case "release":
			watch.releaseControl(me)
		case "resize":
			watch.resizeFrom(me, request.Cols, request.Rows)
		}
	}
}
