package engines

import (
	"bufio"
	"encoding/json"
	"os"
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
	signals := sharedSignals(p)
	if p.HookEventName == "Stop" && len(signals) == 1 {
		signals[0].Tokens = claudeTokens(p.TranscriptPath)
	}
	return signals, nil
}

// claudeTokens adds up what a session's transcript says it used: every
// token the model read (fresh, or from the cache) and wrote. Each turn is
// its own session, so this is the turn's use. A reply can appear on several
// lines with the same usage, so each message is counted once.
func claudeTokens(transcript string) int {
	file, err := os.Open(transcript)
	if err != nil {
		return 0
	}
	defer file.Close()
	seen, total := map[string]bool{}, 0
	lines := bufio.NewScanner(file)
	lines.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for lines.Scan() {
		var entry struct {
			Message struct {
				ID    string `json:"id"`
				Usage *struct {
					Input      int `json:"input_tokens"`
					Output     int `json:"output_tokens"`
					CacheWrite int `json:"cache_creation_input_tokens"`
					CacheRead  int `json:"cache_read_input_tokens"`
				} `json:"usage"`
			} `json:"message"`
		}
		if json.Unmarshal(lines.Bytes(), &entry) != nil || entry.Message.Usage == nil || seen[entry.Message.ID] {
			continue
		}
		seen[entry.Message.ID] = true
		u := entry.Message.Usage
		total += u.Input + u.Output + u.CacheWrite + u.CacheRead
	}
	return total
}

// GateReply: Claude's PreToolUse takes allow and deny as JSON.
func (claude) GateReply(event string, decision Decision, reason string) Reply {
	reply, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": event, "permissionDecision": decision, "permissionDecisionReason": reason}})
	return Reply{Stdout: string(reply)}
}

func claudeSettings(s Session) string { return filepath.Join(s.StateDir, "claude-settings.json") }
