package engines

import (
	"encoding/json"
	"time"
)

// Signal is what Agentry sees from a hook call, whichever engine made it.
type Signal struct {
	Kind          SignalKind
	ToolUseID     string // ties an action's request to its result
	Tool          string
	Command       string    // the shell command, or the file path for file tools
	Prompt        string    // the exact prompt text the engine received
	Report        string    // the agent's final message of the turn
	Question      string    // what the engine is waiting for
	EngineSession string    // the engine's own session id
	Tokens        Tokens    // what the turn used, when the engine says (TurnEnded)
	TokensUnknown string    // why Tokens is zero, when the engine's count couldn't be read (TurnEnded)
	Error         string    // the kind of error that stopped the turn, e.g. rate_limit (UsageLimited, TurnFailed)
	Details       string    // what the engine said about it
	ResetsAt      time.Time // when a usage limit resets, zero when the engine didn't say in a way Agentry reads (UsageLimited)
}

// Tokens is what a turn used, split as the engine bills it. On the wire
// (hookcall.Ending) it is an object of these four numbers.
type Tokens struct {
	Input      int `json:"input"`      // new input, read at the full price
	CacheWrite int `json:"cacheWrite"` // input written to the cache
	CacheRead  int `json:"cacheRead"`  // input read from the cache
	Output     int `json:"output"`
}

// Total is every token the turn used, which the budget counts.
func (t Tokens) Total() int { return t.Input + t.CacheWrite + t.CacheRead + t.Output }

type SignalKind string

const (
	SessionStarted  SignalKind = "session started"
	PromptReceived  SignalKind = "prompt received"
	ActionRequested SignalKind = "action requested"
	ActionFinished  SignalKind = "action finished"
	Waiting         SignalKind = "waiting"
	TurnEnded       SignalKind = "turn ended"
	UsageLimited    SignalKind = "usage limited" // the turn stopped at the account's usage limit; it can go on once that resets
	TurnFailed      SignalKind = "turn failed"   // the turn stopped at an error from the engine's API
	SessionEnded    SignalKind = "session ended"
)

// hookPayload is the part of a hook's stdin both engines share: Codex uses
// the same field names as Claude Code (proof 07).
type hookPayload struct {
	HookEventName        string          `json:"hook_event_name"`
	SessionID            string          `json:"session_id"`
	Prompt               string          `json:"prompt"`
	ToolName             string          `json:"tool_name"`
	ToolInput            json.RawMessage `json:"tool_input"`
	ToolUseID            string          `json:"tool_use_id"`
	LastAssistantMessage string          `json:"last_assistant_message"`
	Message              string          `json:"message"`
	NotificationType     string          `json:"notification_type"` // Claude only
	Error                string          `json:"error"`             // Claude's StopFailure only
	ErrorDetails         string          `json:"error_details"`     // Claude's StopFailure only
	TranscriptPath       string          `json:"transcript_path"`
}

func parsePayload(payload []byte) (hookPayload, error) {
	var p hookPayload
	err := json.Unmarshal(payload, &p)
	return p, err
}

// commandOf pulls what the policy judges out of tool_input: the shell
// command, or the file path for file tools.
func commandOf(toolInput json.RawMessage) string {
	var input struct {
		Command      string `json:"command"`
		FilePath     string `json:"file_path"`
		NotebookPath string `json:"notebook_path"`
		Path         string `json:"path"`
		Pattern      string `json:"pattern"`
		URL          string `json:"url"`
		Query        string `json:"query"`
		Prompt       string `json:"prompt"`
	}
	json.Unmarshal(toolInput, &input)
	for _, value := range []string{input.Command, input.FilePath, input.NotebookPath, input.Path, input.Pattern, input.URL} {
		if value != "" {
			return value
		}
	}
	// Not a path: labelled, so the policy never takes it for one, and two
	// different searches never look like the same command.
	switch {
	case input.Query != "":
		return "search: " + input.Query
	case input.Prompt != "":
		return "prompt: " + input.Prompt
	}
	return ""
}

// sharedSignals maps the hook events both engines have in common.
func sharedSignals(p hookPayload) []Signal {
	switch p.HookEventName {
	case "SessionStart":
		return []Signal{{Kind: SessionStarted, EngineSession: p.SessionID}}
	case "UserPromptSubmit":
		return []Signal{{Kind: PromptReceived, Prompt: p.Prompt}}
	case "PreToolUse":
		return []Signal{{Kind: ActionRequested, ToolUseID: p.ToolUseID, Tool: p.ToolName, Command: commandOf(p.ToolInput)}}
	case "PostToolUse":
		return []Signal{{Kind: ActionFinished, ToolUseID: p.ToolUseID, Tool: p.ToolName, Command: commandOf(p.ToolInput)}}
	case "Stop":
		return []Signal{{Kind: TurnEnded, Report: p.LastAssistantMessage}}
	case "SessionEnd":
		return []Signal{{Kind: SessionEnded}}
	}
	return nil
}
