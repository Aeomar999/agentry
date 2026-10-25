package engines_test

import (
	"path/filepath"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/engines"
)

// stopTokens is what Claude's Stop hook says a turn that started at started
// used, with the recorded transcript in testdata. Transcripts aren't named
// .jsonl, which under testdata is an event log the events tests check.
func stopTokens(t *testing.T, transcript string, started time.Time) (engines.Tokens, string) {
	t.Helper()
	signal := stopSignal(t, transcript, started)
	return signal.Tokens, signal.TokensUnknown
}

func stopSignal(t *testing.T, transcript string, started time.Time) engines.Signal {
	t.Helper()
	claude, _ := engines.Get("claude")
	path := filepath.Join("testdata", transcript)
	signals, err := claude.ParseHook("Stop", []byte(`{"hook_event_name":"Stop","last_assistant_message":"Done.","transcript_path":"`+path+`"}`), started)
	if err != nil || len(signals) != 1 {
		t.Fatalf("%s: signals %+v, err %v", transcript, signals, err)
	}
	return signals[0]
}

// A turn's split sums all its calls, but each call reads the whole context
// again, so the session's size is the last counted call's input alone: its
// new input, cache writes and cache reads. An entry copied in with an old
// time isn't counted, so it isn't the last call even at the end of the file.
func TestClaudeSaysHowBigTheContextGrew(t *testing.T) {
	want := engines.Tokens{Input: 3 + 2 + 1, CacheWrite: 12000 + 3000 + 900, CacheRead: 14000 + 26000 + 29000, Output: 250 + 400 + 120}
	if s := stopSignal(t, "growing.transcript", at(t, "2026-10-24T10:00:00Z")); s.Tokens != want || s.Context != 1+900+29000 || s.TokensUnknown != "" {
		t.Errorf("tokens %+v, context %d (%q); want %+v, context %d", s.Tokens, s.Context, s.TokensUnknown, want, 1+900+29000)
	}
}

// With nothing counted, the context is unknown: 0.
func TestClaudesContextIsUnknownWithNoUsage(t *testing.T) {
	if s := stopSignal(t, "growing.transcript", at(t, "2026-10-24T12:00:00Z")); s.Context != 0 || s.TokensUnknown != engines.NoUsageThisTurn {
		t.Errorf("signal %+v", s)
	}
}

func at(t *testing.T, text string) time.Time {
	t.Helper()
	when, err := time.Parse(time.RFC3339, text)
	if err != nil {
		t.Fatal(err)
	}
	return when
}

// A turn counts only what it used: in a fresh session that is the whole
// transcript, in a resumed one the entries after the earlier turns, and in
// a forked one the entries after those copied in with their old times.
// Claude writes a reply on a line per content block, each with the same
// usage, so each message is counted once.
func TestClaudeCountsOnlyThisTurnsTokens(t *testing.T) {
	for _, c := range []struct {
		transcript, started string
		want                engines.Tokens
	}{
		{"fresh.transcript", "2026-10-24T09:00:00Z", engines.Tokens{Input: 3 + 1, CacheWrite: 12000 + 800, CacheRead: 14000 + 26000, Output: 250 + 120}},
		{"resumed.transcript", "2026-10-24T10:00:00Z", engines.Tokens{Input: 5 + 2, CacheWrite: 2100 + 600, CacheRead: 40800 + 42900, Output: 310 + 90}},
		{"forked.transcript", "2026-10-24T11:00:00Z", engines.Tokens{Input: 4 + 1, CacheWrite: 1500 + 300, CacheRead: 41000 + 42700, Output: 500 + 60}},
	} {
		used, unknown := stopTokens(t, c.transcript, at(t, c.started))
		if used != c.want || unknown != "" {
			t.Errorf("%s: used %+v (%q), want %+v", c.transcript, used, unknown, c.want)
		}
	}
	if total := (engines.Tokens{Input: 4, CacheWrite: 12800, CacheRead: 40000, Output: 370}).Total(); total != 53174 {
		t.Errorf("the total is all four: %d", total)
	}
}

// A session started by a worker older than the start stamp has no start,
// so its whole transcript is the turn's, as when each turn was a session.
func TestClaudeCountsTheWholeTranscriptWithNoStart(t *testing.T) {
	used, unknown := stopTokens(t, "resumed.transcript", time.Time{})
	want := engines.Tokens{Input: 3 + 1 + 5 + 2, CacheWrite: 12000 + 800 + 2100 + 600, CacheRead: 14000 + 26000 + 40800 + 42900, Output: 250 + 120 + 310 + 90}
	if used != want || unknown != "" {
		t.Errorf("used %+v (%q), want %+v", used, unknown, want)
	}
}

// A transcript whose usage is all from earlier turns says so, rather than
// counting them or a silent 0.
func TestClaudeSaysWhenNoneOfTheUsageIsThisTurns(t *testing.T) {
	used, unknown := stopTokens(t, "resumed.transcript", at(t, "2026-10-24T12:00:00Z"))
	if used != (engines.Tokens{}) || unknown != engines.NoUsageThisTurn {
		t.Errorf("used %+v, reason %q, want %q", used, unknown, engines.NoUsageThisTurn)
	}
}

// Agentry doesn't read Codex's use yet, whatever the start, nor its context.
func TestCodexTokensStayUnread(t *testing.T) {
	codex, _ := engines.Get("codex")
	signals, err := codex.ParseHook("Stop", []byte(`{"hook_event_name":"Stop","last_assistant_message":"Done."}`), at(t, "2026-10-24T09:00:00Z"))
	if err != nil || len(signals) != 1 || signals[0].Tokens != (engines.Tokens{}) || signals[0].Context != 0 || signals[0].TokensUnknown != engines.CodexTokensNotRead {
		t.Errorf("signals %+v, err %v", signals, err)
	}
}
