package agents

import (
	"encoding/json"
	"reflect"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/deeptest"
)

// Every field, exported or not, is filled by reflection, and the saved form
// goes through JSON as a snapshot will write it: a field added to Agent that
// Save and Restore don't carry comes back empty, and the test fails.
func TestASavedRosterComesBackWhole(t *testing.T) {
	var roster Roster
	deeptest.Fill(t, reflect.ValueOf(&roster).Elem())
	written, err := json.Marshal(roster.Save())
	if err != nil {
		t.Fatal(err)
	}
	var read Saved
	if err := json.Unmarshal(written, &read); err != nil {
		t.Fatal(err)
	}
	if restored := Restore(read); !reflect.DeepEqual(roster, restored) {
		t.Errorf("the roster read back differs:\n%+v\n%+v", roster, restored)
	}
}

func TestARestoredRosterSharesNothing(t *testing.T) {
	var roster Roster
	deeptest.Fill(t, reflect.ValueOf(&roster).Elem())
	restored := Restore(roster.Save())
	if !reflect.DeepEqual(roster, restored) {
		t.Fatalf("the restored roster differs:\n%+v\n%+v", roster, restored)
	}
	deeptest.Shared(t, "Roster", reflect.ValueOf(roster), reflect.ValueOf(restored))
}

// A roster replayed from its log, and an empty one, come back the same.
func TestAReplayedRosterComesBackWhole(t *testing.T) {
	stream := seeded()
	stream = append(stream, everyAgentEvent(FromEvents(stream))...)
	for _, roster := range []Roster{nil, FromEvents(stream)} {
		written, err := json.Marshal(roster.Save())
		if err != nil {
			t.Fatal(err)
		}
		var read Saved
		if err := json.Unmarshal(written, &read); err != nil {
			t.Fatal(err)
		}
		if restored := Restore(read); !reflect.DeepEqual(roster, restored) {
			t.Errorf("the roster read back differs:\n%+v\n%+v", roster, restored)
		}
	}
}
