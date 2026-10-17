package mcp

import (
	"context"
	"encoding/json"
	"io"
	"os/exec"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/client"
)

// Server is one Claude Code session's Agentry: its tools, for the repo the
// session runs in, on one Agentry server.
type Server struct {
	agentry   client.Client
	repo      string // the repo Claude Code runs in; "" outside one
	conn      *conn
	canElicit bool // Claude Code can ask you a question itself
	version   string
}

// New is the server for Claude Code started in dir.
func New(agentry client.Client, dir, version string) *Server {
	return &Server{agentry: agentry, repo: repoOf(dir), version: version}
}

// Serve speaks MCP on in and out until in ends.
func (s *Server) Serve(ctx context.Context, in io.Reader, out io.Writer) error {
	s.conn = newConn(out)
	return s.conn.serve(ctx, in, s.handle)
}

// latest is the protocol version this server speaks; a client asking for
// an older one gets that one, which is the same for what is used here.
const latest = "2025-06-18"

func (s *Server) handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "initialize":
		var hello struct {
			ProtocolVersion string `json:"protocolVersion"`
			Capabilities    struct {
				Elicitation json.RawMessage `json:"elicitation"`
			} `json:"capabilities"`
		}
		json.Unmarshal(params, &hello)
		s.canElicit = hello.Capabilities.Elicitation != nil
		version := latest
		if hello.ProtocolVersion != "" && hello.ProtocolVersion < latest {
			version = hello.ProtocolVersion
		}
		return map[string]any{
			"protocolVersion": version,
			"capabilities":    map[string]any{"tools": map[string]any{}, "prompts": map[string]any{}},
			"serverInfo":      map[string]any{"name": "agentry", "version": s.version},
			"instructions":    instructions,
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		list := []map[string]any{}
		for _, t := range s.tools() {
			list = append(list, map[string]any{"name": t.name, "description": t.description, "inputSchema": t.input})
		}
		return map[string]any{"tools": list}, nil
	case "tools/call":
		var call struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(params, &call); err != nil {
			return nil, &rpcError{Code: -32602, Message: "a tools/call needs a name and arguments"}
		}
		return s.call(ctx, call.Name, call.Arguments), nil
	case "prompts/list":
		return map[string]any{"prompts": []map[string]any{{"name": "board", "description": "Your Agentry board for this repo: what needs you, what runs, what is planned"}}}, nil
	case "prompts/get":
		return map[string]any{"messages": []map[string]any{{"role": "user", "content": map[string]any{"type": "text", "text": boardPrompt}}}}, nil
	}
	return nil, &rpcError{Code: -32601, Message: "no method " + method}
}

// call runs a tool. Its reply ends with what waits for you, so Claude can
// tell you without being asked.
func (s *Server) call(ctx context.Context, name string, arguments json.RawMessage) map[string]any {
	for _, t := range s.tools() {
		if t.name != name {
			continue
		}
		if len(arguments) == 0 {
			arguments = json.RawMessage("{}")
		}
		text, err := t.run(ctx, arguments)
		if err != nil {
			return map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": err.Error()}}}
		}
		if t.name != "status" {
			text += s.waitingNote()
		}
		return map[string]any{"content": []map[string]any{{"type": "text", "text": text}}}
	}
	return map[string]any{"isError": true, "content": []map[string]any{{"type": "text", "text": "no tool " + name}}}
}

// elicit asks you, through Claude Code, and returns your answer's fields;
// ok is false if you declined or closed it. The model sees none of it
// until the tool replies.
func (s *Server) elicit(ctx context.Context, text string, schema map[string]any) (answer map[string]any, ok bool, err error) {
	var reply struct {
		Action  string         `json:"action"` // accept, decline or cancel
		Content map[string]any `json:"content"`
	}
	if err := s.conn.request(ctx, "elicitation/create", map[string]any{"message": text, "requestedSchema": schema}, &reply); err != nil {
		return nil, false, err
	}
	return reply.Content, reply.Action == "accept", nil
}

// repoOf is the git repo dir is in, or "".
func repoOf(dir string) string {
	out, err := exec.Command("git", "-C", dir, "rev-parse", "--show-toplevel").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// instructions tell Claude what Agentry is and how to use it well.
const instructions = `Agentry is the user's task board where AI agents do the work: each task goes to a team's coordinator agent, which hands it from agent to agent (engineer, reviewer, tester, deployer, or another team's) until it is checked and done. Its tools let you hand work to it from this repo and keep the user informed.

- Turning a request into work: write a task with a one-line imperative title and a description of what to change, why, and how to tell it works. New tasks work in this repo unless you name another. Show the user the title and description first unless they said to go ahead.
- Epics: a goal too big for one change is an epic (kind "epic"); its coordinator splits it into stories, which wait in the user's Backlog until they start them. File tasks under an epic with file_under_epic, set priority with set_priority, and start stories with start.
- Questions from coordinators: relay them to the user and send the user's own answer with answer_question. Never answer for the user.
- Approvals: review_approvals and close_epic ask the user directly in Claude Code; you can't approve for them. Offer to run them when something waits.
- Every reply ends with what is waiting for the user. When something is, tell them in a line and offer to deal with it.`

const boardPrompt = `Show me my Agentry board for this repo. Call the status tool, then tell me, briefly: first what needs me (and offer to deal with it), then what is running, then what is planned and not started.`
