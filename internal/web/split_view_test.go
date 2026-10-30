package web

import (
	"encoding/json"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// A turn the engine split carries the four parts and its session; one it
// didn't carries neither, so the page shows its total as before.
func TestATurnCarriesItsSplitAndSessionOnlyWhenKnown(t *testing.T) {
	task := &tasks.Task{TokensUsed: 54174, TokensSplit: tasks.Split{Input: 4, CacheWrite: 12800, CacheRead: 40000, Output: 370},
		Turns: []*tasks.Turn{
			{ID: "turn-1", Ended: true, Tokens: 53174, EngineSession: "4f1c", Split: tasks.Split{Input: 4, CacheWrite: 12800, CacheRead: 40000, Output: 370}},
			{ID: "turn-2", Ended: true, Tokens: 1000},
		}}
	detail := detailOf(task, agents.Roster{})
	split, _ := json.Marshal(detail.Turns[0])
	want := `"split":{"input":4,"cacheWrite":12800,"cacheRead":40000,"output":370},"engineSession":"4f1c"`
	if !strings.Contains(string(split), want) {
		t.Errorf("turn-1 %s, want %s", split, want)
	}
	if plain, _ := json.Marshal(detail.Turns[1]); strings.Contains(string(plain), "split") || strings.Contains(string(plain), "engineSession") {
		t.Errorf("turn-2 %s has a split or session it never had", plain)
	}
	body, _ := json.Marshal(detail)
	if want := `"tokensSplit":{"input":4,"cacheWrite":12800,"cacheRead":40000,"output":370},"tokensUnsplit":1000`; !strings.Contains(string(body), want) {
		t.Errorf("task %s, want %s", body, want)
	}
}

// A task none of whose turns said how they split has no split at all.
func TestATaskWithNoSplitTurnsHasNoSplit(t *testing.T) {
	task := &tasks.Task{TokensUsed: 1000, Turns: []*tasks.Turn{{ID: "turn-1", Ended: true, Tokens: 1000}}}
	body, _ := json.Marshal(detailOf(task, agents.Roster{}))
	if strings.Contains(string(body), "tokensSplit") || strings.Contains(string(body), "tokensUnsplit") {
		t.Errorf("task %s has a split", body)
	}
}

// A turn says whether it resumed a session or started fresh and why, and
// when the session was a sibling story's, which one and whether it forked
// it; a turn from a log that said none of this carries none of it.
func TestATurnSaysWhetherItResumedOnlyWhenKnown(t *testing.T) {
	task := &tasks.Task{Turns: []*tasks.Turn{
		{ID: "turn-1", Ended: true, FreshBecause: "first turn"},
		{ID: "turn-2", Ended: true, FromSession: "4f1c"},
		{ID: "turn-3", Ended: true, FromSession: "4f1c", SessionFrom: "task-1"},
		{ID: "turn-4", Ended: true, FromSession: "4f1c", SessionFrom: "task-1", Forked: true},
		{ID: "turn-5", Ended: true},
	}}
	detail := detailOf(task, agents.Roster{})
	for i, want := range []string{`"freshBecause":"first turn"`, `"fromSession":"4f1c"`,
		`"fromSession":"4f1c","sessionFrom":"task-1"`, `"fromSession":"4f1c","sessionFrom":"task-1","forked":true`} {
		if body, _ := json.Marshal(detail.Turns[i]); !strings.Contains(string(body), want) {
			t.Errorf("turn-%d %s, want %s", i+1, body, want)
		}
	}
	if own, _ := json.Marshal(detail.Turns[1]); strings.Contains(string(own), "sessionFrom") || strings.Contains(string(own), "forked") {
		t.Errorf("turn-2 %s names a story or a fork though its session was this task's own", own)
	}
	if old, _ := json.Marshal(detail.Turns[4]); strings.Contains(string(old), "ession") || strings.Contains(string(old), "fresh") || strings.Contains(string(old), "forked") {
		t.Errorf("turn-5 %s says how it started though its log didn't", old)
	}
}
