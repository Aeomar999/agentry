package hooks

import (
	"context"
	"fmt"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/platform/engines"
	"gitlab.com/dev.chempa/agentry/internal/platform/hookcall"
)

// turnEnded is the data of the turn.ended the handler wrote for turn-2.
func turnEnded(t *testing.T, ending *hookcall.Ending) map[string]any {
	t.Helper()
	s, state := engineerAtWork(t)
	stop := hookCall("Stop", `{"hook_event_name":"Stop","last_assistant_message":"Done."}`)
	stop.Ending = ending
	NewHandler(s, state, judgeByCommand).Handle(context.Background(), stop)
	stream, _ := s.Stream("task-1")
	for _, e := range stream {
		if e.Type == events.TurnEnded && e.Correlation == "turn-2" {
			return e.Data
		}
	}
	t.Fatal("no turn.ended for turn-2")
	return nil
}

// The turn's tokens are kept split as the engine bills them, with "tokens"
// still their total for the budget. An older worker sends only the total,
// and gets no split of zeros.
func TestATurnKeepsItsTokensSplit(t *testing.T) {
	split := engines.Tokens{Input: 7, CacheWrite: 2700, CacheRead: 83700, Output: 400}
	data := turnEnded(t, &hookcall.Ending{Tokens: split.Total(), Split: &split})
	for field, want := range map[string]int{"tokens": 86807, "inputTokens": 7, "cacheWriteTokens": 2700, "cacheReadTokens": 83700, "outputTokens": 400} {
		if got := fmt.Sprint(data[field]); got != fmt.Sprint(want) {
			t.Errorf("%s: %s, want %d (data %v)", field, got, want, data)
		}
	}

	older := turnEnded(t, &hookcall.Ending{Tokens: 1234})
	if fmt.Sprint(older["tokens"]) != "1234" {
		t.Errorf("an older worker's total: %v", older)
	}
	for _, field := range []string{"inputTokens", "cacheWriteTokens", "cacheReadTokens", "outputTokens", "tokensUnknown"} {
		if _, ok := older[field]; ok {
			t.Errorf("an older worker's turn.ended has %s: %v", field, older)
		}
	}
}

// The turn's context, the input of its last call, rides along when the
// hook knows it. Unknown, from Codex or an older worker, it is left out.
func TestATurnKeepsHowBigItsContextGrew(t *testing.T) {
	split := engines.Tokens{Input: 7, CacheWrite: 2700, CacheRead: 83700, Output: 400}
	data := turnEnded(t, &hookcall.Ending{Tokens: split.Total(), Split: &split, Context: 43502})
	if fmt.Sprint(data["contextTokens"]) != "43502" {
		t.Errorf("contextTokens %v, want 43502 (data %v)", data["contextTokens"], data)
	}
	if unknown := turnEnded(t, &hookcall.Ending{Tokens: split.Total(), Split: &split}); unknown["contextTokens"] != nil {
		t.Errorf("an unknown context is left out: %v", unknown)
	}
}
