// Package workerlink connects a worker to the Agentry server. The worker
// dials out (it may be behind a router; the server is the one with an
// address) and keeps one websocket open, over which the server asks it to do
// what a machine does (package machines) and the worker says what it runs.
// A terminal being watched gets a websocket of its own, which the worker
// dials when asked, so its bytes never queue behind a request.
//
// Hooks don't use the link: sessions on a worker post them to the server's
// address, as sessions on the server do.
package workerlink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"github.com/coder/websocket"

	"gitlab.com/dev.chempa/agentry/internal/terminal"
	"gitlab.com/dev.chempa/agentry/internal/workspace"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

// message is one websocket text message. With an ID and a method it is a
// request, which gets a reply with the same ID; with a method and no ID, a
// notice, which gets none.
type message struct {
	ID     uint64          `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// Handler answers the requests one side gets.
type Handler func(ctx context.Context, method string, params json.RawMessage) (any, error)

// conn is one side of the link.
type conn struct {
	ws      *websocket.Conn
	handle  Handler                            // requests from the other side; nil answers none
	notice  func(method string, params []byte) // notices from the other side; nil drops them
	mu      sync.Mutex
	next    uint64
	pending map[uint64]chan message
}

func newConn(ws *websocket.Conn, handle Handler, notice func(string, []byte)) *conn {
	ws.SetReadLimit(64 << 20) // a file read for the page can be a few MB
	return &conn{ws: ws, handle: handle, notice: notice, pending: map[uint64]chan message{}}
}

// run reads until the link closes. Each request is answered on its own, so a
// long one (a first copy of a repo) holds up nothing else.
func (c *conn) run(ctx context.Context) error {
	defer c.failPending()
	for {
		_, data, err := c.ws.Read(ctx)
		if err != nil {
			return err
		}
		var m message
		if json.Unmarshal(data, &m) != nil {
			continue
		}
		switch {
		case m.Method == "":
			c.deliver(m)
		case m.ID == 0:
			if c.notice != nil {
				c.notice(m.Method, m.Params)
			}
		default:
			go c.answer(ctx, m)
		}
	}
}

func (c *conn) answer(ctx context.Context, request message) {
	reply := message{ID: request.ID}
	if c.handle == nil {
		reply.Error = "this side takes no requests"
	} else if result, err := c.handle(ctx, request.Method, request.Params); err != nil {
		reply.Error = err.Error()
	} else {
		reply.Result, _ = json.Marshal(result)
	}
	c.send(ctx, reply)
}

func (c *conn) deliver(reply message) {
	c.mu.Lock()
	waiting, ok := c.pending[reply.ID]
	delete(c.pending, reply.ID)
	c.mu.Unlock()
	if ok {
		waiting <- reply
	}
}

func (c *conn) failPending() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id, waiting := range c.pending {
		waiting <- message{ID: id, Error: errLinkClosed.Error()}
		delete(c.pending, id)
	}
}

var errLinkClosed = errors.New("the worker's link closed")

// call sends a request and decodes its reply into result.
func (c *conn) call(ctx context.Context, method string, params, result any) error {
	c.mu.Lock()
	c.next++
	id := c.next
	waiting := make(chan message, 1)
	c.pending[id] = waiting
	c.mu.Unlock()

	encoded, _ := json.Marshal(params)
	if err := c.send(ctx, message{ID: id, Method: method, Params: encoded}); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return fmt.Errorf("%w: %v", errLinkClosed, err)
	}
	select {
	case reply := <-waiting:
		if reply.Error != "" {
			return knownError(reply.Error)
		}
		if result != nil {
			return json.Unmarshal(reply.Result, result)
		}
		return nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return ctx.Err()
	}
}

func (c *conn) notify(ctx context.Context, method string, params any) error {
	encoded, _ := json.Marshal(params)
	return c.send(ctx, message{Method: method, Params: encoded})
}

func (c *conn) send(ctx context.Context, m message) error {
	data, _ := json.Marshal(m)
	return c.ws.Write(ctx, websocket.MessageText, data)
}

// knownError gives back the errors callers tell apart, which only their
// text survives the link.
func knownError(text string) error {
	for _, known := range []error{worktrees.ErrNotMerged, terminal.ErrNoSession, workspace.ErrOutside} {
		if text == known.Error() {
			return known
		}
	}
	return errors.New(text)
}
