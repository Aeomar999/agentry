package policy

import (
	"errors"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

func TestHistoryShowsEveryVersionAndRestoresOnlyThoseWithText(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	live, _ := openTestLive(t)
	change, err := live.Change(Edit{Seen: live.Current().Hash(), Apply: TurnOffUseForEveryone}, at)
	if err != nil {
		t.Fatal(err)
	}
	event := func(position int64, p events.Proposal) events.Event {
		return events.Event{Position: position, At: at.Add(time.Duration(position) * time.Minute), Proposal: p}
	}
	stream := []events.Event{
		// From before the text was recorded.
		event(3, events.Proposal{Stream: events.PolicyStream, Actor: Agentry, Type: events.PolicyLoaded,
			Data: map[string]any{"hash": "0ld", "path": "/p", "profiles": []any{"developer"}}}),
		event(7, Refused("/p", change.Before, aRefusal)),
		event(9, Loaded("/p", change, "turned off use_for_everyone", you)),
	}
	history := History(stream)
	if len(history) != 3 {
		t.Fatalf("history %+v", history)
	}
	newest, refused, old := history[0], history[1], history[2]
	if newest.Position != 9 || newest.Actor != you || newest.Change != "turned off use_for_everyone" || !newest.CanRevert ||
		newest.Backup != change.Backup || !strings.Contains(newest.Diff, "+# use_for_everyone") {
		t.Errorf("newest: %+v", newest)
	}
	if !refused.Refused || refused.Change != aRefusal.Error() || refused.Hash != change.Before.Hash() || refused.CanRevert {
		t.Errorf("refused: %+v", refused)
	}
	if old.Change != "loaded" || old.Hash != "0ld" || old.CanRevert {
		t.Errorf("an old event shows, without Revert: %+v", old)
	}

	if text, when, err := VersionAt(stream, 9); err != nil || text != change.Policy.Text() || !when.Equal(at.Add(9*time.Minute)) {
		t.Errorf("version 9: %v %v", when, err)
	}
	for _, position := range []int64{3, 7, 8} {
		if _, _, err := VersionAt(stream, position); err == nil {
			t.Errorf("version %d has no text to restore", position)
		}
	}
}

var aRefusal = errors.New("line 1, column 11: toml: expected value")
