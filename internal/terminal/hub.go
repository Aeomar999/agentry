package terminal

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"

	"github.com/coder/websocket"

	"gitlab.com/dev.chempa/agentry/internal/sessions"
)

// Hub holds one watch per task being watched: the control client and its
// viewers. A watch starts with the first viewer and ends with the last one,
// or when the session ends (each turn is a new session; the page reconnects).
type Hub struct {
	mu      sync.Mutex
	watches map[string]*viewers
}

func NewHub() *Hub { return &Hub{watches: map[string]*viewers{}} }

// Handler serves the websocket for the task in the path's {task}:
//
//	server → browser  binary: terminal bytes   text: {"type":"state",...}
//	browser → server  binary: keystrokes       text: {"type":"take"} / {"type":"release"}
func (h *Hub) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		task := r.PathValue("task")
		if !sessions.Exists(sessions.SocketFor(task)) {
			http.Error(w, "no agent is running on "+task, http.StatusNotFound)
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if err != nil {
			return
		}
		conn.SetReadLimit(1 << 20)
		defer conn.CloseNow()

		watch, err := h.watch(task)
		if err != nil {
			conn.Close(websocket.StatusInternalError, err.Error())
			return
		}
		connect(r.Context(), conn, watch, func() { h.leave(task, watch) })
	})
}

// watch returns the task's watch, attaching a control client if there is none.
func (h *Hub) watch(task string) (*viewers, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if watch, ok := h.watches[task]; ok {
		return watch, nil
	}
	watch := newViewers()
	c, err := attach(sessions.SocketFor(task), watch.broadcast)
	if err != nil {
		return nil, err
	}
	watch.control = c // set before anyone joins, so typeFrom always has it
	h.watches[task] = watch
	go func() {
		<-c.ended // the session ended: everyone reconnects to the next one
		h.mu.Lock()
		if h.watches[task] == watch {
			delete(h.watches, task)
		}
		h.mu.Unlock()
		watch.closeAll()
	}()
	return watch, nil
}

func (h *Hub) leave(task string, watch *viewers) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.watches[task] == watch {
		delete(h.watches, task)
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
		var request struct{ Type string }
		json.Unmarshal(message, &request)
		switch request.Type {
		case "take":
			watch.takeControl(me)
		case "release":
			watch.releaseControl(me)
		}
	}
}
