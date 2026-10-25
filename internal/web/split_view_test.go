package web

import (
	"encoding/json"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
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
