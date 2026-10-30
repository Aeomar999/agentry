package main

import (
	"path/filepath"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/platform/engines"
	"gitlab.com/dev.chempa/agentry/internal/platform/hookcall"
)

func stopEnding(t *testing.T) *hookcall.Ending {
	t.Helper()
	t.Setenv(hookcall.EnvWorktree, t.TempDir())
	transcript, _ := filepath.Abs("../../internal/platform/engines/testdata/resumed.transcript")
	end := ending(hookcall.Call{Engine: "claude", Event: "Stop",
		Payload: []byte(`{"hook_event_name":"Stop","last_assistant_message":"Done.","transcript_path":"` + transcript + `"}`)})
	if end == nil || end.Split == nil {
		t.Fatalf("ending %+v", end)
	}
	return end
}

// The hook counts from the start the machine stamped on the session, and
// sends the split with its total, which an older server reads alone.
func TestTheHookCountsFromTheTurnsStart(t *testing.T) {
	t.Setenv(hookcall.EnvTurnStarted, "2026-10-24T10:00:00.5Z")
	end := stopEnding(t)
	want := engines.Tokens{Input: 7, CacheWrite: 2700, CacheRead: 83700, Output: 400}
	if *end.Split != want || end.Tokens != want.Total() || end.TokensUnknown != "" {
		t.Errorf("ending %+v, split %+v, want %+v", end, *end.Split, want)
	}
	if end.Context != 2+600+42900 { // the turn's last call
		t.Errorf("context %d, want the last call's input", end.Context)
	}
}

// With no stamp, from a server older than it, the whole transcript counts.
func TestTheHookCountsTheWholeTranscriptWithNoStart(t *testing.T) {
	t.Setenv(hookcall.EnvTurnStarted, "")
	if end := stopEnding(t); end.Tokens != 53174+86807 {
		t.Errorf("ending %+v, want the whole transcript", end)
	}
}
