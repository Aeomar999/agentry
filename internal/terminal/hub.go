package terminal

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sync"

	"github.com/coder/websocket"
)

// Hub holds one watch per session being watched, a task's or the desk's:
// the pane and its viewers. A watch starts with the first viewer and ends
// with the last one, or when the session ends (each turn is a new session;
// the page reconnects).
type Hub struct {
	mu      sync.Mutex
	watches map[string]*viewers
}

func NewHub() *Hub { return &Hub{watches: map[string]*viewers{}} }

// Serve connects one viewer to the session named name, a task's or the
// desk's, opened with open: on this machine (Attach) or on a worker.
// The websocket carries:
//
//	server → browser  binary: terminal bytes   text: {"type":"state", ..., "cols", "rows"}
//	browser → server  binary: keystrokes       text: {"type":"take"} / {"type":"release"}
//	                                                 {"type":"resize","cols":c,"rows":r}
//
// The session has one size, sent in every state, and each viewer shows it at
// that size. A viewer may resize it to fit their screen while nobody else is
// in control.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, name string, open Opener) {
	watch, err := h.watch(name, open)
	if errors.Is(err, ErrNoSession) {
		http.Error(w, "no session is running for "+name, http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	conn, err := websocket.Accept(w, r, nil)
	if err != nil {
		h.leaveIfEmpty(name, watch)
		return
	}
	conn.SetReadLimit(1 << 20)
	defer conn.CloseNow()
	connect(r.Context(), conn, watch, func() { h.leave(name, watch) })
}

// Handler serves Serve for the task in the path's {task}, opened with open.
func (h *Hub) Handler(open Opener) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { h.Serve(w, r, r.PathValue("task"), open) })
}

// watch returns the session's watch, opening its pane if there is none.
func (h *Hub) watch(name string, open Opener) (*viewers, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if watch, ok := h.watches[name]; ok && !hasEnded(watch.control) {
		return watch, nil
	} // else its session ended (as when the desk restarts) and the cleanup hasn't run yet
	watch := newViewers()
	pane, err := open(name, watch.broadcast)
	if err != nil {
		return nil, err
	}
	watch.control = pane // set before anyone joins, so typeFrom always has it
	watch.cols, watch.rows = pane.Size()
	h.watches[name] = watch
	go func() {
		<-pane.Ended() // the session ended: everyone reconnects to the next one
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
	watch.control.Detach()
}

// leaveIfEmpty closes a watch nobody joined, as when the websocket failed.
func (h *Hub) leaveIfEmpty(name string, watch *viewers) {
	watch.mu.Lock()
	empty := len(watch.all) == 0
	watch.mu.Unlock()
	if empty {
		h.leave(name, watch)
	}
}

func hasEnded(pane Pane) bool {
	select {
	case <-pane.Ended():
		return true
	default:
		return false
	}
}

// connect runs one viewer until their tab closes or the session ends.
func connect(ctx context.Context, conn *websocket.Conn, watch *viewers, lastOneOut func()) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Join first so no output is missed, then send the session's size, so
	// the viewer's terminal is that size before it draws anything, then the
	// current screen, before the live output that queued up meanwhile.
	me := watch.join()
	defer func() {
		if watch.leave(me) {
			lastOneOut()
		}
	}()
	if conn.Write(ctx, websocket.MessageText, watch.stateFor(me)) != nil ||
		conn.Write(ctx, websocket.MessageBinary, watch.control.Snapshot()) != nil {
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
