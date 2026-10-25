package engines_test

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/engines"
)

// usageLine is one assistant entry with the given time and usage.
func usageLine(id, timestamp string, input, output int) string {
	stamp := ""
	if timestamp != "" {
		stamp = `"timestamp":"` + timestamp + `",`
	}
	return `{"type":"assistant",` + stamp + `"message":{"id":"` + id + `","usage":{"input_tokens":` +
		strconv.Itoa(input) + `,"cache_creation_input_tokens":0,"cache_read_input_tokens":0,"output_tokens":` + strconv.Itoa(output) + `}}}`
}

// tokensOf is what Claude's Stop hook counts in a transcript of these lines.
func tokensOf(t *testing.T, started time.Time, lines ...string) (engines.Tokens, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "t.transcript")
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	claude, _ := engines.Get("claude")
	signals, err := claude.ParseHook("Stop", []byte(`{"hook_event_name":"Stop","last_assistant_message":"Done.","transcript_path":"`+path+`"}`), started)
	if err != nil || len(signals) != 1 {
		t.Fatalf("signals %+v, err %v", signals, err)
	}
	return signals[0].Tokens, signals[0].TokensUnknown
}

// An entry written at the very instant the turn started is the turn's.
func TestClaudeCountsAnEntryAtTheStartItself(t *testing.T) {
	start := time.Date(2026, 10, 24, 9, 0, 0, 120_000_000, time.UTC)
	used, unknown := tokensOf(t, start,
		usageLine("m1", "2026-10-24T09:00:00.119Z", 100, 100),
		usageLine("m2", "2026-10-24T09:00:00.120Z", 3, 7))
	if used != (engines.Tokens{Input: 3, Output: 7}) || unknown != "" {
		t.Errorf("used %+v (%q), want only m2", used, unknown)
	}
}

// Times in another zone compare by the instant, not the text.
func TestClaudeComparesTimesAcrossZones(t *testing.T) {
	start := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	used, _ := tokensOf(t, start,
		usageLine("m1", "2026-10-24T10:30:00+02:00", 100, 100), // 08:30 UTC: earlier
		usageLine("m2", "2026-10-24T05:30:00-04:00", 3, 7))     // 09:30 UTC: this turn's
	if used != (engines.Tokens{Input: 3, Output: 7}) {
		t.Errorf("used %+v, want only m2", used)
	}
}

// Undated usage beside dated usage of this turn: the dated part is counted,
// with no reason given, and the undated part is left out.
func TestClaudeCountsTheDatedPartWhenSomeIsUndated(t *testing.T) {
	start := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	used, unknown := tokensOf(t, start,
		usageLine("m1", "", 100, 100),
		usageLine("m2", "not a time", 100, 100),
		usageLine("m3", "2026-10-24T09:00:05Z", 3, 7))
	if used != (engines.Tokens{Input: 3, Output: 7}) || unknown != "" {
		t.Errorf("used %+v (%q), want only m3", used, unknown)
	}
}

// A message copied in with its old time and then written again in this
// turn is counted once, from this turn.
func TestClaudeCountsAMessageSeenEarlierOnceWhenItRecurs(t *testing.T) {
	start := time.Date(2026, 10, 24, 9, 0, 0, 0, time.UTC)
	used, _ := tokensOf(t, start,
		usageLine("m1", "2026-10-24T08:00:00Z", 3, 7),
		usageLine("m1", "2026-10-24T09:00:01Z", 3, 7),
		usageLine("m1", "2026-10-24T09:00:02Z", 3, 7))
	if used != (engines.Tokens{Input: 3, Output: 7}) {
		t.Errorf("used %+v, want m1 once", used)
	}
}
