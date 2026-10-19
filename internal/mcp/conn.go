// Package mcp is Agentry as a Model Context Protocol server, so Claude Code
// in any repo has Agentry's tools: create and plan work, hear what needs
// you, answer, and approve. An approval is asked of you through Claude
// Code (elicitation), never of the model, so Claude can't approve for you.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"sync"
)

// The stdio transport: one JSON-RPC 2.0 message per line, both ways. Claude
// Code sends requests and notifications; the server answers, and sends its
// own requests (elicitation) whose answers come back on the same stream.

type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *rpcError) Error() string { return e.Message }

// handler answers one request: a result, or an error to send back.
type handler func(ctx context.Context, method string, params json.RawMessage) (any, error)

type conn struct {
	out     io.Writer
	writing sync.Mutex

	asking  sync.Mutex
	next    int
	pending map[string]chan message // our requests, by id, until their answer
}

func newConn(out io.Writer) *conn { return &conn{out: out, pending: map[string]chan message{}} }

// serve reads messages until in ends, answering each request in its own
// goroutine, so a tool that waits for you doesn't hold up the rest. It
// returns once the requests it read are answered.
func (c *conn) serve(ctx context.Context, in io.Reader, handle handler) error {
	var answering sync.WaitGroup
	defer answering.Wait()
	lines := bufio.NewScanner(in)
	lines.Buffer(make([]byte, 1<<20), 16<<20)
	for lines.Scan() {
		var m message
		if json.Unmarshal(lines.Bytes(), &m) != nil {
			continue
		}
		switch {
		case m.Method == "" && m.ID != nil: // the answer to one of ours
			c.asking.Lock()
			waiting := c.pending[string(m.ID)]
			delete(c.pending, string(m.ID))
			c.asking.Unlock()
			if waiting != nil {
				waiting <- m
			}
		case m.ID == nil: // a notification: nothing to answer
		default:
			answering.Add(1)
			go func(m message) {
				defer answering.Done()
				result, err := handle(ctx, m.Method, m.Params)
				reply := message{JSONRPC: "2.0", ID: m.ID}
				var failure *rpcError
				switch {
				case errors.As(err, &failure):
					reply.Error = failure
				case err != nil:
					reply.Error = &rpcError{Code: -32603, Message: err.Error()}
				default:
					reply.Result, _ = json.Marshal(result)
				}
				c.write(reply)
			}(m)
		}
	}
	return lines.Err()
}

// request asks Claude Code something and waits for its answer.
func (c *conn) request(ctx context.Context, method string, params any, into any) error {
	c.asking.Lock()
	c.next++
	id := json.RawMessage(strconv.Itoa(c.next))
	answer := make(chan message, 1)
	c.pending[string(id)] = answer
	c.asking.Unlock()

	encoded, _ := json.Marshal(params)
	if err := c.write(message{JSONRPC: "2.0", ID: id, Method: method, Params: encoded}); err != nil {
		return err
	}
	select {
	case m := <-answer:
		if m.Error != nil {
			return fmt.Errorf("%s: %s", method, m.Error.Message)
		}
		return json.Unmarshal(m.Result, into)
	case <-ctx.Done():
		c.asking.Lock()
		delete(c.pending, string(id))
		c.asking.Unlock()
		return ctx.Err()
	}
}

// notify tells Claude Code something that needs no answer.
func (c *conn) notify(method string) error {
	return c.write(message{JSONRPC: "2.0", Method: method})
}

func (c *conn) write(m message) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	c.writing.Lock()
	defer c.writing.Unlock()
	_, err = c.out.Write(append(data, '\n'))
	return err
}
