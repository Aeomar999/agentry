package engines

import (
	"encoding/json"
	"path/filepath"
	"slices"
	"time"
)

// codex is OpenAI's Codex CLI, measured in proof 07 (codex-cli 0.163.0).
type codex struct{}

func init() { register(codex{}) }

func (codex) Name() string { return "codex" }

// Codex has no Notification hook; PermissionRequest is where it waits, and
// the only place an allow is accepted, so it is a gate too.
var (
	codexGates   = []string{"PreToolUse", "PermissionRequest"}
	codexReports = []string{"PostToolUse", "UserPromptSubmit", "Stop", "SessionStart", "SessionEnd"}
)

// Prepare writes <worktree>/.codex/hooks.json, which Codex reads for the project.
func (codex) Prepare(s Session) error {
	return writeJSON(filepath.Join(s.Worktree, ".codex", "hooks.json"), hookSettings(s.HookCommand, codexGates, codexReports))
}

// Launch: hook trust is bypassed as in proof 07; registering trust properly is T-028.
func (codex) Launch(s Session, prompt string) []string {
	command := []string{"codex"}
	if s.Model != "" {
		command = append(command, "--model", s.Model)
	}
	return append(command, "-s", "workspace-write", "-a", "never", "--dangerously-bypass-hook-trust", prompt)
}

func (codex) IsGate(event string) bool { return slices.Contains(codexGates, event) }

func (codex) ParseHook(event string, payload []byte, _ time.Time) ([]Signal, error) {
	p, err := parsePayload(payload)
	if err != nil {
		return nil, err
	}
	if p.HookEventName == "PermissionRequest" {
		return []Signal{{Kind: ActionRequested, ToolUseID: p.ToolUseID, Tool: p.ToolName, Command: commandOf(p.ToolInput)}}, nil
	}
	signals := sharedSignals(p)
	if p.HookEventName == "Stop" && len(signals) == 1 {
		signals[0].TokensUnknown = CodexTokensNotRead
	}
	return signals, nil
}

// CodexTokensNotRead: Agentry doesn't read what a Codex turn used yet, so
// its Context is unknown too.
const CodexTokensNotRead = "Codex's token use isn't read"

// GateReply: Codex's PreToolUse rejects "allow" as unsupported and needs a
// reason with "deny", so allow prints nothing and the call proceeds in
// never-ask mode. PermissionRequest takes allow and deny (proof 07).
func (codex) GateReply(event string, decision Decision, reason string) Reply {
	if event == "PermissionRequest" {
		reply, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
			"hookEventName": event, "decision": map[string]any{"behavior": decision, "message": reason}}})
		return Reply{Stdout: string(reply)}
	}
	if decision == Allow {
		return Reply{}
	}
	if reason == "" {
		reason = "Denied by Agentry"
	}
	reply, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": event, "permissionDecision": "deny", "permissionDecisionReason": reason}})
	return Reply{Stdout: string(reply)}
}
