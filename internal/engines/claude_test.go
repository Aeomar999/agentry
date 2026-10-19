package engines

import (
	"runtime"
	"testing"
	"time"
)

// On Windows Claude names a folder C:/Users/…; it is trusted in both forms.
func TestAWindowsFolderIsTrustedBothWays(t *testing.T) {
	keys := folderKeys(`C:\Users\me\worktrees\task-1`)
	if runtime.GOOS == "windows" && (len(keys) != 2 || keys[1] != "C:/Users/me/worktrees/task-1") {
		t.Errorf("keys: %v", keys)
	}
	if runtime.GOOS != "windows" && len(keys) != 1 {
		t.Errorf("a Unix path has one form: %v", keys) // backslashes are just characters here
	}
}

// Every tool's call says what it does, so two different searches never look
// like the same command.
func TestCommandOfSaysWhatACallDoes(t *testing.T) {
	cases := map[string]string{
		`{"command": "git status"}`:                             "git status",
		`{"query": "sourdough lactobacilli"}`:                   "search: sourdough lactobacilli",
		`{"url": "https://example.com/starter", "prompt": "x"}`: "https://example.com/starter",
		`{"notebook_path": "/w/a.ipynb"}`:                       "/w/a.ipynb",
		`{"todos": []}`:                                         "",
	}
	for input, want := range cases {
		if got := commandOf([]byte(input)); got != want {
			t.Errorf("commandOf(%s) = %q, want %q", input, got, want)
		}
	}
}

// A rate_limit is a usage limit when it says so or says when it resets,
// in the future; any other time in it, or one already past, isn't enough.
func TestARateLimitIsAUsageLimitOnlyWithAFutureReset(t *testing.T) {
	now := time.Date(2026, 10, 19, 23, 40, 0, 0, time.UTC)
	cases := []struct {
		details  string
		kind     SignalKind
		resetsAt time.Time
	}{
		{`429 {"request_id":"req_1","timestamp":"2026-10-20T04:00:00Z"}`, TurnFailed, time.Time{}},
		{`429 {"error":"rate_limit","resets_at":"2026-10-20T04:00:00Z"}`, UsageLimited, time.Date(2026, 10, 20, 4, 0, 0, 0, time.UTC)},
		{"limit resets at 2026-10-19T20:00:00Z", TurnFailed, time.Time{}},
		{"You've hit your session limit · resets at 2026-10-19T20:00:00Z", UsageLimited, time.Time{}},
	}
	for _, c := range cases {
		s := claudeFailure(hookPayload{Error: "rate_limit", ErrorDetails: c.details}, now)
		if s.Kind != c.kind || !s.ResetsAt.Equal(c.resetsAt) {
			t.Errorf("%q: got %v at %v, want %v at %v", c.details, s.Kind, s.ResetsAt, c.kind, c.resetsAt)
		}
	}
}
