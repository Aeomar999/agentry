package extensions

import (
	"context"
	"encoding/json"
)

// A Tool is one MCP tool: one thing Claude can do with Agentry. agentry mcp
// lists an extension's tools while GET /api/features says the extension is
// enabled or visible, and their output is the text Claude reads.
type Tool struct {
	Name, Description string
	Input             map[string]any // JSON Schema of its arguments (internal/schema)
	Run               func(ctx context.Context, arguments json.RawMessage) (string, error)
}

// API is the Agentry server as agentry mcp reaches it, as client.Client
// does: Call sends body as JSON and decodes the answer into into, and
// PageURL is a page's address for a person. An interface, so extensions
// needn't import client and what it brings.
type API interface {
	Call(method, path string, body, into any) error
	PageURL(path string) string
}

// Ask asks you something through Claude Code (elicitation) and returns your
// answer's fields, to schema; ok is false if you declined or closed it. The
// model sees none of it until the tool replies.
type Ask func(ctx context.Context, text string, schema map[string]any) (answer map[string]any, ok bool, err error)

type askKey struct{}

// WithAsk is ctx for a tool's Run that may ask you with ask.
func WithAsk(ctx context.Context, ask Ask) context.Context {
	return context.WithValue(ctx, askKey{}, ask)
}

// AskOf is how a tool's Run may ask you, or nil when Claude Code can't ask:
// then a tool that needs your decision acts on its arguments.
func AskOf(ctx context.Context) Ask {
	ask, _ := ctx.Value(askKey{}).(Ask)
	return ask
}

// Waiting is one thing waiting for you, as GET /api/waiting lists it and
// agentry mcp shows it: a line in its replies, the status line and the
// hooks, and news when Key is new.
type Waiting struct {
	Key  string `json:"key"` // the same while it lasts: a new one has a new key
	Task string `json:"task"`
	Line string `json:"line"` // for a person: "task-4: Waiting for you to approve the push to dev"
	Kind string `json:"kind"` // "needs you", "done", "said", "offline", or an extension's own, such as "ideas"
}
