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
		event(4, Loaded("/p", change, "first loaded", Agentry)),
		// From before the onDisk flag, in both of its wordings.
		event(5, Loaded("/p", change, "changed on disk", Agentry)),
		event(6, Loaded("/p", change, "noticed on disk", Agentry)),
		event(7, NoticedOnDisk("/p", change)),
		event(8, Refused("/p", change.Before, aRefusal)),
		event(10, Loaded("/p", change, "turned off use_for_everyone", you)),
	}
	history := History(stream)
	if len(history) != 7 {
		t.Fatalf("history %+v", history)
	}
	newest, refused, flagged, unflagged, older, first, old := history[0], history[1], history[2], history[3], history[4], history[5], history[6]
	if newest.Position != 10 || newest.Actor != you || newest.Change != "turned off use_for_everyone" || !newest.CanRevert ||
		newest.Backup != change.Backup || !strings.Contains(newest.Diff, "+# use_for_everyone") || newest.OnDisk {
		t.Errorf("newest: %+v", newest)
	}
	if !refused.Refused || refused.Change != aRefusal.Error() || refused.Hash != change.Before.Hash() || refused.CanRevert || !refused.OnDisk {
		t.Errorf("refused: %+v", refused)
	}
	if flagged.Position != 7 || !flagged.OnDisk || flagged.Actor != Agentry || flagged.Change != "noticed on disk" {
		t.Errorf("a hand edit is on disk: %+v", flagged)
	}
	if unflagged.Position != 6 || !unflagged.OnDisk {
		t.Errorf("an old hand edit, without the flag, is on disk too: %+v", unflagged)
	}
	if older.Position != 5 || !older.OnDisk {
		t.Errorf("an older hand edit, worded 'changed on disk', is on disk too: %+v", older)
	}
	if first.Position != 4 || first.OnDisk {
		t.Errorf("the first version loaded isn't a hand edit: %+v", first)
	}
	if old.Change != "loaded" || old.Hash != "0ld" || old.CanRevert || old.OnDisk {
		t.Errorf("an old event shows, without Revert: %+v", old)
	}

	if text, when, err := VersionAt(stream, 10); err != nil || text != change.Policy.Text() || !when.Equal(at.Add(10*time.Minute)) {
		t.Errorf("version 10: %v %v", when, err)
	}
	for _, position := range []int64{3, 8} {
		if _, _, err := VersionAt(stream, position); err == nil || errors.Is(err, ErrNoVersion) {
			t.Errorf("version %d is there, with no text to restore: %v", position, err)
		}
	}
	if _, _, err := VersionAt(stream, 9); !errors.Is(err, ErrNoVersion) {
		t.Errorf("position 9 holds no version: %v", err)
	}
}

var aRefusal = errors.New("line 1, column 11: toml: expected value")
