package engines

import (
	"bufio"
	"cmp"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// claude is Claude Code, measured in proofs 01 to 03 (Claude Code 2.1.301).
type claude struct{}

func init() { register(claude{}) }

func (claude) Name() string { return "claude" }

// Why a turn's tokens are unknown. Claude Code saves no transcript when it
// takes itself for another session's child (see IsParentSessionVar).
const (
	TranscriptNotSaved  = "transcript not saved"
	NoUsageInTranscript = "no usage in transcript"
)

var (
	claudeGates   = []string{"PreToolUse"}
	claudeReports = []string{"PostToolUse", "UserPromptSubmit", "Notification", "Stop", "StopFailure", "SessionStart", "SessionEnd"}
)

// Prepare writes a settings file for `claude --settings`: the hooks, and
// the page's theme, so the session's colours suit the terminal showing it.
// It also trusts the worktree, so the session doesn't stop to ask.
func (claude) Prepare(s Session) error {
	settings := hookSettings(s.HookCommand, claudeGates, claudeReports)
	for key, value := range unattended {
		settings[key] = value
	}
	if s.Theme != "" {
		settings["theme"] = s.Theme
	}
	if err := writeJSON(claudeSettings(s), settings); err != nil {
		return err
	}
	return trustFolder(s.Worktree)
}

// unattended are the settings that keep Claude from stopping to offer or
// ask things nobody is there to answer. The renderer is chosen, so it never
// offers the fullscreen one; tips and the feedback survey are off; and the
// one-time warning about bypass mode isn't shown, since an agent always
// runs in it, with Agentry's policy deciding what it may do.
var unattended = map[string]any{
	"tui":                               "default",
	"spinnerTipsEnabled":                false,
	"skipDangerousModePermissionPrompt": true,
	"env":                               map[string]string{"CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY": "1"},
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
	if p.HookEventName == "StopFailure" {
		return []Signal{claudeFailure(p, time.Now())}, nil
	}
	signals := sharedSignals(p)
	if p.HookEventName == "Stop" && len(signals) == 1 {
		signals[0].Tokens, signals[0].TokensUnknown = claudeTokens(p.TranscriptPath)
		if signals[0].Report == "" {
			// Claude Code before 2.1 has no last_assistant_message.
			signals[0].Report = claudeLastMessage(p.TranscriptPath)
		}
	}
	return signals, nil
}

// claudeFailure is what a StopFailure means: Claude Code sends it instead
// of Stop when the turn ends at an API error. rate_limit is a usage limit
// only when the message or the error details say so or name a reset still
// to come; it is also used for plain 429s and capacity errors, which fail
// the turn like any other.
func claudeFailure(p hookPayload, now time.Time) Signal {
	details := cmp.Or(p.LastAssistantMessage, p.ErrorDetails)
	failure := Signal{Kind: TurnFailed, Error: cmp.Or(p.Error, "unknown"), Details: details}
	both := p.LastAssistantMessage + "\n" + p.ErrorDetails
	// Claude Code's own details win over a time the model wrote.
	resetsAt := ResetTime(p.ErrorDetails, now)
	if resetsAt.IsZero() {
		resetsAt = ResetTime(both, now)
	}
	if p.Error == "rate_limit" && (hitYourLimit(both) || !resetsAt.IsZero()) {
		failure.Kind, failure.ResetsAt = UsageLimited, resetsAt
	}
	return failure
}

// hitYourLimit is Claude Code's "You've hit your session limit", with
// either apostrophe.
func hitYourLimit(text string) bool {
	return strings.Contains(strings.ReplaceAll(text, "’", "'"), "You've hit your")
}

// claudeLastMessage is the text of the last thing the model said in a
// session's transcript.
func claudeLastMessage(transcript string) string {
	file, err := os.Open(transcript)
	if err != nil {
		return ""
	}
	defer file.Close()
	last := ""
	lines := bufio.NewScanner(file)
	lines.Buffer(make([]byte, 0, 1<<20), 16<<20)
	for lines.Scan() {
		var entry struct {
			Type    string `json:"type"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal(lines.Bytes(), &entry) != nil || entry.Type != "assistant" {
			continue
		}
		var text []string
		for _, part := range entry.Message.Content {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				text = append(text, part.Text)
			}
		}
		if len(text) > 0 {
			last = strings.Join(text, "\n")
		}
	}
	return last
}

// claudeTokens adds up what a session's transcript says it used: every
// token the model read (fresh, or from the cache) and wrote. Each turn is
// its own session, so this is the turn's use. A reply can appear on several
// lines with the same usage, so each message is counted once. When there is
// nothing to count, it says why instead.
func claudeTokens(transcript string) (int, string) {
	file, err := os.Open(transcript)
	if errors.Is(err, fs.ErrNotExist) { // also when the path is ""
		return 0, TranscriptNotSaved
	}
	if err != nil {
		return 0, "transcript unreadable: " + err.Error()
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
	if len(seen) == 0 {
		return 0, NoUsageInTranscript
	}
	return total, ""
}

// GateReply: Claude's PreToolUse takes allow and deny as JSON.
func (claude) GateReply(event string, decision Decision, reason string) Reply {
	reply, _ := json.Marshal(map[string]any{"hookSpecificOutput": map[string]any{
		"hookEventName": event, "permissionDecision": decision, "permissionDecisionReason": reason}})
	return Reply{Stdout: string(reply)}
}

func claudeSettings(s Session) string { return filepath.Join(s.StateDir, "claude-settings.json") }
