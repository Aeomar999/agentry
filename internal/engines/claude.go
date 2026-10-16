package engines

import (
	"encoding/json"
	"path/filepath"
	"slices"
)

// claude is Claude Code, measured in proofs 01 to 03 (Claude Code 2.1.301).
type claude struct{}

func init() { register(claude{}) }

func (claude) Name() string { return "claude" }

var (
	claudeGates   = []string{"PreToolUse"}
	claudeReports = []string{"PostToolUse", "UserPromptSubmit", "Notification", "Stop", "SessionStart", "SessionEnd"}
)

// Prepare writes a settings file for `claude --settings`: the hooks, and
// the page's theme, so the session's colours suit the terminal showing it.
func (claude) Prepare(s Session) error {
	settings := hookSettings(s.HookCommand, claudeGates, claudeReports)
	if s.Theme != "" {
		settings["theme"] = s.Theme
	}
	return writeJSON(claudeSettings(s), settings)
}

func (claude) Launch(s Session, prompt string) []string {
	command := []string{"claude"}
	if s.Model != "" {
		command = append(command, "--model", s.Model)
	}
	return append(command, "--settings", claudeSettings(s), "--permission-mode", "bypassPermissions", prompt)
}

func (claude) IsGate(event string) bool { return slices.Contains(claudeGates, event) }

func (claude) ParseHook(event string, payload []byte) ([]Signal, error) {
	p, err := parsePayload(payload)
	if err != nil {
		return nil, err
	}
	if p.HookEventName == "Notification" {
		// permission_prompt means Claude is showing its own prompt despite
		// never-ask mode. idle_prompt comes about 60 s after Stop: nothing new.
		if p.NotificationType == "permission_prompt" {
			return []Signal{{Kind: Waiting, Question: p.Message}}, nil
		}
		return nil, nil
	}
	return sharedSignals(p), nil
}

// GateReply: Claude's PreToolUse takes allow and deny as JSON.
func (claude) GateReply(event string, decision Decision, reason string) Reply {
	reply, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": event, "permissionDecision": decision, "permissionDecisionReason": reason}})
	return Reply{Stdout: string(reply)}
}

func claudeSettings(s Session) string { return filepath.Join(s.StateDir, "claude-settings.json") }
