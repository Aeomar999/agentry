package engines_test

import (
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/platform/engines"
)

// Usage whose time Agentry can't read may be an earlier turn's, so with a
// start known it isn't counted. When that leaves nothing, the turn says the
// times couldn't be read, not that none of the usage was this turn's.
func TestClaudeSaysWhenTheUsageTimesCantBeRead(t *testing.T) {
	used, unknown := stopTokens(t, "undated.transcript", at(t, "2026-10-24T10:00:00Z"))
	if used != (engines.Tokens{}) || unknown != engines.NoUsageDated {
		t.Errorf("used %+v, reason %q, want %q", used, unknown, engines.NoUsageDated)
	}
}

// With no start, the times don't matter: the whole transcript is the turn's.
func TestClaudeCountsUndatedUsageWithNoStart(t *testing.T) {
	used, unknown := stopTokens(t, "undated.transcript", time.Time{})
	want := engines.Tokens{Input: 3 + 5 + 2, CacheWrite: 12000 + 2100 + 600, CacheRead: 14000 + 40800 + 42900, Output: 250 + 310 + 90}
	if used != want || unknown != "" {
		t.Errorf("used %+v (%q), want %+v", used, unknown, want)
	}
}
