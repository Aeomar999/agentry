package engines

import "encoding/json"

// Signal is what Agentry sees from a hook call, whichever engine made it.
type Signal struct {
	Kind          SignalKind
	ToolUseID     string // ties an action's request to its result
	Tool          string
	Command       string // the shell command, or the file path for file tools
	Prompt        string // the exact prompt text the engine received
	Report        string // the agent's final message of the turn
	Question      string // what the engine is waiting for
	EngineSession string // the engine's own session id
	Tokens        int    // tokens the turn used, when the engine says (TurnEnded)
}

type SignalKind string

const (
	SessionStarted  SignalKind = "session started"
	PromptReceived  SignalKind = "prompt received"
	ActionRequested SignalKind = "action requested"
	ActionFinished  SignalKind = "action finished"
	Waiting         SignalKind = "waiting"
	TurnEnded       SignalKind = "turn ended"
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
