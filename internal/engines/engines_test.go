package engines_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/engines"
	"gitlab.com/dev.chempa/agentry/internal/rules"
)

func engine(t *testing.T, name string) engines.Engine {
	t.Helper()
	e, ok := engines.Get(name)
	if !ok {
		t.Fatalf("no %s adapter", name)
	}
	return e
}

func TestTheRulesKnowExactlyTheEnginesThereAreAdaptersFor(t *testing.T) {
	if !slices.Equal(engines.Names(), rules.KnownEngines) {
		t.Errorf("adapters %v, rules.KnownEngines %v", engines.Names(), rules.KnownEngines)
	}
}

// Real payloads, trimmed, recorded from Claude Code 2.1.301 and Codex 0.163.0 (proof 07).
var recorded = []struct {
	engine, event, payload string
	want                   engines.Signal
}{
	{"claude", "SessionStart", `{"hook_event_name":"SessionStart","session_id":"535d7c02","source":"startup"}`,
		engines.Signal{Kind: engines.SessionStarted, EngineSession: "535d7c02"}},
	{"claude", "UserPromptSubmit", `{"hook_event_name":"UserPromptSubmit","prompt":"Read .agentry/turn-1.md"}`,
		engines.Signal{Kind: engines.PromptReceived, Prompt: "Read .agentry/turn-1.md"}},
	{"claude", "PreToolUse", `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"echo ok"},"tool_use_id":"toolu_01Eq"}`,
		engines.Signal{Kind: engines.ActionRequested, ToolUseID: "toolu_01Eq", Tool: "Bash", Command: "echo ok"}},
	{"claude", "PreToolUse", `{"hook_event_name":"PreToolUse","tool_name":"Edit","tool_input":{"file_path":"/w/a.go"},"tool_use_id":"toolu_02"}`,
		engines.Signal{Kind: engines.ActionRequested, ToolUseID: "toolu_02", Tool: "Edit", Command: "/w/a.go"}},
	{"claude", "PostToolUse", `{"hook_event_name":"PostToolUse","tool_name":"Bash","tool_input":{"command":"echo ok"},"tool_use_id":"toolu_01Eq"}`,
		engines.Signal{Kind: engines.ActionFinished, ToolUseID: "toolu_01Eq", Tool: "Bash", Command: "echo ok"}},
	{"claude", "Notification", `{"hook_event_name":"Notification","message":"Claude needs your permission","notification_type":"permission_prompt"}`,
		engines.Signal{Kind: engines.Waiting, Question: "Claude needs your permission"}},
	{"claude", "Stop", `{"hook_event_name":"Stop","last_assistant_message":"Removed 3 TODOs.","stop_hook_active":false}`,
		engines.Signal{Kind: engines.TurnEnded, Report: "Removed 3 TODOs.", TokensUnknown: engines.TranscriptNotSaved}},

	{"codex", "SessionStart", `{"hook_event_name":"SessionStart","session_id":"01a14257","model":"gpt-6-luna","source":"startup"}`,
		engines.Signal{Kind: engines.SessionStarted, EngineSession: "01a14257"}},
	{"codex", "UserPromptSubmit", `{"hook_event_name":"UserPromptSubmit","turn_id":"t1","prompt":"Read .agentry/turn-1.md"}`,
		engines.Signal{Kind: engines.PromptReceived, Prompt: "Read .agentry/turn-1.md"}},
	{"codex", "PreToolUse", `{"hook_event_name":"PreToolUse","tool_name":"Bash","tool_input":{"command":"echo ok"},"tool_use_id":"exec-33981fa3"}`,
		engines.Signal{Kind: engines.ActionRequested, ToolUseID: "exec-33981fa3", Tool: "Bash", Command: "echo ok"}},
	{"codex", "PermissionRequest", `{"hook_event_name":"PermissionRequest","tool_name":"Bash","tool_input":{"command":"touch x.txt","description":"May I run..."}}`,
		engines.Signal{Kind: engines.ActionRequested, Tool: "Bash", Command: "touch x.txt"}},
	{"codex", "Stop", `{"hook_event_name":"Stop","last_assistant_message":"FIRST-OK","turn_id":"t1"}`,
		engines.Signal{Kind: engines.TurnEnded, Report: "FIRST-OK", TokensUnknown: engines.CodexTokensNotRead}},
}

func TestParseHookGivesTheSameSignalsOnBothEngines(t *testing.T) {
	for _, r := range recorded {
		signals, err := engine(t, r.engine).ParseHook(r.event, []byte(r.payload))
		if err != nil {
			t.Fatalf("%s %s: %v", r.engine, r.event, err)
		}
		if len(signals) != 1 || signals[0] != r.want {
			t.Errorf("%s %s: got %+v, want %+v", r.engine, r.event, signals, r.want)
		}
	}
	idle, _ := engine(t, "claude").ParseHook("Notification", []byte(`{"hook_event_name":"Notification","notification_type":"idle_prompt"}`))
	if len(idle) != 0 {
		t.Errorf("idle_prompt should give no signal, got %+v", idle)
	}
}

// The gate replies each engine accepts, as found in proofs 01 and 07.
func TestGateReplies(t *testing.T) {
	decisionIn := func(stdout string) string {
		var reply struct {
			HookSpecificOutput struct {
				PermissionDecision string
				Decision           struct{ Behavior string }
			}
		}
		json.Unmarshal([]byte(stdout), &reply)
		if reply.HookSpecificOutput.Decision.Behavior != "" {
			return reply.HookSpecificOutput.Decision.Behavior
		}
		return reply.HookSpecificOutput.PermissionDecision
	}
	cases := []struct {
		engine, event string
		decision      engines.Decision
		reason, want  string // want: the decision in the JSON, "" for no output at all
	}{
		{"claude", "PreToolUse", engines.Allow, "ok", "allow"},
		{"claude", "PreToolUse", engines.Deny, "never sudo", "deny"},
		{"codex", "PreToolUse", engines.Allow, "ok", ""},                   // Codex rejects allow in PreToolUse
		{"codex", "PreToolUse", engines.Deny, "", "deny"},                  // and needs a reason, so one is filled in
		{"codex", "PermissionRequest", engines.Allow, "approved", "allow"}, // allow goes here instead
	}
	for _, c := range cases {
		reply := engine(t, c.engine).GateReply(c.event, c.decision, c.reason)
		if reply.ExitCode != 0 || decisionIn(reply.Stdout) != c.want || (c.want == "" && reply.Stdout != "") {
			t.Errorf("%s %s %s: got %+v, want decision %q", c.engine, c.event, c.decision, reply, c.want)
		}
		if c.engine == "codex" && c.decision == engines.Deny && !strings.Contains(reply.Stdout, "Denied by Agentry") {
			t.Errorf("codex deny without a reason should get one: %q", reply.Stdout)
		}
	}
}

func TestPrepareWritesHooksThatFailClosed(t *testing.T) {
	worktree := t.TempDir()
	s := engines.Session{Worktree: worktree, StateDir: filepath.Join(worktree, ".agentry"), HookCommand: "/bin/agentry hook"}
	for _, name := range []string{"claude", "codex"} {
		if err := engine(t, name).Prepare(s); err != nil {
			t.Fatal(err)
		}
	}
	for _, path := range []string{".agentry/claude-settings.json", ".codex/hooks.json"} {
		content, err := os.ReadFile(filepath.Join(worktree, path))
		if err != nil || !json.Valid(content) {
			t.Fatalf("%s: missing or not JSON (%v)", path, err)
		}
		text := string(content)
		if !strings.Contains(text, `/bin/agentry hook PreToolUse || exit 2`) || !strings.Contains(text, `/bin/agentry hook Stop || true`) {
			t.Errorf("%s: the gate must fail closed and reports never block:\n%s", path, text)
		}
	}
}

// Claude's settings choose its renderer, so it never stops to offer the
// fullscreen one, and turn off what else it shows on its own.
func TestClaudeSettingsKeepItUnattended(t *testing.T) {
	worktree := t.TempDir()
	if err := engine(t, "claude").Prepare(engines.Session{Worktree: worktree, StateDir: filepath.Join(worktree, ".agentry"), HookCommand: "agentry hook"}); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(filepath.Join(worktree, ".agentry", "claude-settings.json"))
	var settings struct {
		TUI         string            `json:"tui"`
		SpinnerTips *bool             `json:"spinnerTipsEnabled"`
		Env         map[string]string `json:"env"`
	}
	json.Unmarshal(content, &settings)
	if settings.TUI != "default" || settings.SpinnerTips == nil || *settings.SpinnerTips || settings.Env["CLAUDE_CODE_DISABLE_FEEDBACK_SURVEY"] != "1" {
		t.Errorf("settings:\n%s", content)
	}
}

func TestLaunchIsNeverAskWithThePrompt(t *testing.T) {
	s := engines.Session{StateDir: "/w/.agentry", Model: "haiku"}
	claude := strings.Join(engine(t, "claude").Launch(s, "Read .agentry/turn-1.md"), " ")
	codex := strings.Join(engine(t, "codex").Launch(s, "Read .agentry/turn-1.md"), " ")
	for command, want := range map[string]string{claude: "--permission-mode bypassPermissions", codex: "-a never"} {
		if !strings.Contains(command, want) || !strings.HasSuffix(command, "Read .agentry/turn-1.md") || !strings.Contains(command, "--model haiku") {
			t.Errorf("launch %q: want %q, the model, and the prompt last", command, want)
		}
	}
}

// A turn's tokens come from its transcript, each reply counted once.
func TestClaudeCountsTheTurnsTokens(t *testing.T) {
	transcript := filepath.Join(t.TempDir(), "session.jsonl")
	os.WriteFile(transcript, []byte(`{"type":"user","message":{"role":"user","content":"go"}}
{"message":{"id":"msg_1","usage":{"input_tokens":6,"cache_creation_input_tokens":27370,"cache_read_input_tokens":105289,"output_tokens":1228}}}
{"message":{"id":"msg_1","usage":{"input_tokens":6,"cache_creation_input_tokens":27370,"cache_read_input_tokens":105289,"output_tokens":1228}}}
{"message":{"id":"msg_2","usage":{"input_tokens":2,"output_tokens":100}}}
not json
`), 0o644)
	claude, _ := engines.Get("claude")
	signals, err := claude.ParseHook("Stop", []byte(`{"hook_event_name":"Stop","last_assistant_message":"Done.","transcript_path":"`+transcript+`"}`))
	if err != nil || len(signals) != 1 || signals[0].Tokens != 6+27370+105289+1228+2+100 || signals[0].TokensUnknown != "" {
		t.Errorf("signals %+v, err %v", signals, err)
	}
}

// With nothing to count, the turn says why rather than a silent 0: Claude
// Code saves no transcript in a session it takes for another's child.
func TestClaudeSaysWhyATurnsTokensAreUnknown(t *testing.T) {
	dir := t.TempDir()
	noUsage := filepath.Join(dir, "no-usage.jsonl")
	os.WriteFile(noUsage, []byte(`{"type":"user","message":{"role":"user","content":"go"}}
{"type":"assistant","message":{"id":"msg_1","content":[{"type":"text","text":"Done."}]}}
`), 0o644)
	claude, _ := engines.Get("claude")
	for transcript, want := range map[string]string{
		"":                               engines.TranscriptNotSaved,
		filepath.Join(dir, "gone.jsonl"): engines.TranscriptNotSaved,
		noUsage:                          engines.NoUsageInTranscript,
	} {
		path, _ := json.Marshal(transcript)
		signals, err := claude.ParseHook("Stop", []byte(`{"hook_event_name":"Stop","last_assistant_message":"Done.","transcript_path":`+string(path)+`}`))
		if err != nil || len(signals) != 1 || signals[0].Tokens != 0 || signals[0].TokensUnknown != want {
			t.Errorf("transcript %q: signals %+v, err %v, want the reason %q", transcript, signals, err, want)
		}
	}
}

// Claude Code before 2.1 sends no last_assistant_message with Stop: the
// report is the last thing the model said, from the transcript.
func TestOlderClaudesReportComesFromTheTranscript(t *testing.T) {
	transcript := filepath.Join(t.TempDir(), "session.jsonl")
	os.WriteFile(transcript, []byte(`{"type":"user","message":{"role":"user","content":"go"}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Looking."},{"type":"tool_use","name":"Bash"}]}}
{"type":"user","message":{"content":[{"type":"tool_result","content":"ok"}]}}
{"type":"assistant","message":{"content":[{"type":"text","text":"Assign it. {\"action\":\"done\",\"summary\":\"ok\"}"}]}}
`), 0o644)
	claude, _ := engines.Get("claude")
	signals, err := claude.ParseHook("Stop", []byte(`{"hook_event_name":"Stop","transcript_path":"`+transcript+`"}`))
	if err != nil || len(signals) != 1 || signals[0].Report != `Assign it. {"action":"done","summary":"ok"}` {
		t.Errorf("got %+v %v", signals, err)
	}
}

// Preparing a Claude session trusts its worktree in Claude's config, so it
// doesn't stop at "Do you trust this folder?", and keeps the rest as it was.
func TestPrepareTrustsTheWorktree(t *testing.T) {
	config := filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), ".claude.json")
	os.WriteFile(config, []byte(`{"userID":"u1","projects":{"/home/dev/app":{"hasTrustDialogAccepted":true,"allowedTools":["Bash"]}}}`), 0o600)
	worktree := t.TempDir()
	claude, _ := engines.Get("claude")
	if err := claude.Prepare(engines.Session{Worktree: worktree, StateDir: filepath.Join(worktree, ".agentry"), HookCommand: "agentry hook"}); err != nil {
		t.Fatal(err)
	}
	content, _ := os.ReadFile(config)
	var got struct {
		UserID   string                    `json:"userID"`
		Projects map[string]map[string]any `json:"projects"`
	}
	json.Unmarshal(content, &got)
	if got.UserID != "u1" || got.Projects["/home/dev/app"]["allowedTools"] == nil || got.Projects[worktree]["hasTrustDialogAccepted"] != true {
		t.Errorf("config after: %s", content)
	}
	if info, _ := os.Stat(config); info.Mode().Perm() != 0o600 {
		t.Errorf("mode %v, want it kept at 0600", info.Mode().Perm())
	}
}
