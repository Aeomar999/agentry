package ideas_test

import (
	"encoding/json"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// The ideas in testdata/repeated-ideas.json, suggested again as a session
// would, each one raised again saying which it repeats, then some accepted
// and the rest dismissed: the state that followed the log event by event,
// and one loaded from it at a restart, both equal a replay. storetest checks
// each when the test ends.
func TestTheRepeatedIdeasLogEqualsAReplay(t *testing.T) {
	data, err := os.ReadFile("testdata/repeated-ideas.json")
	if err != nil {
		t.Fatal(err)
	}
	var byID map[string]*ideas.Idea
	if err := json.Unmarshal(data, &byID); err != nil {
		t.Fatal(err)
	}
	repeats := map[string]string{"idea-21": "idea-11", "idea-22": "idea-12", "idea-26": "idea-16", "idea-27": "idea-17", "idea-28": "idea-18"}
	path := filepath.Join(t.TempDir(), "agentry.db")
	s, state, repo := openSeededAt(t, path)
	made := map[string]string{} // the fixture's id to the one the log gave it
	for _, id := range slices.SortedFunc(maps.Keys(byID), byNumber) {
		idea := byID[id]
		fields := map[string]any{"title": idea.Title, "problem": idea.Problem, "proposal": idea.Proposal}
		if original, found := repeats[id]; found {
			fields["repeats"] = made[original]
		}
		made[id] = suggest(t, s, repo, fields)
	}
	for i, id := range ideas.List(state, "") {
		if i%3 == 0 {
			if acceptance, err := ideas.Accept(s, state, id.ID, ideas.Choice{Note: "yes"}); err != nil || !acceptance.Accepted {
				t.Fatalf("accepting %s: %+v %v", id.ID, acceptance, err)
			}
			continue
		}
		mustPropose(t, s, events.Proposal{Stream: id.ID, Actor: you, Type: ideas.IdeaDismissed, Data: map[string]any{"note": "not now"}})
	}
	if order := state.OwnedIDs(ideas.Prefix); len(order) != len(byID) {
		t.Fatalf("the state lists %d ideas, want %d", len(order), len(byID))
	}

	_, restarted := storetest.OpenWith(t, path, storetest.WithExtensions())
	if got, want := ideas.All(restarted), ideas.All(state); len(got) != len(want) || got[len(got)-1].ID != want[len(want)-1].ID {
		t.Errorf("after a restart the ideas are %d, ending %s; before, %d", len(got), got[len(got)-1].ID, len(want))
	}
}

func byNumber(a, b string) int {
	number := func(id string) int {
		n, _ := strconv.Atoi(strings.TrimPrefix(id, ideas.Prefix))
		return n
	}
	return number(a) - number(b)
}
