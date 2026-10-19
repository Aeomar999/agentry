package workers

import (
	"encoding/json"
	"reflect"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/deeptest"
)

// Every field, exported or not, is filled by reflection, and the saved form
// goes through JSON as a snapshot will write it: a field added to Roster or
// Worker that Save and Restore don't carry comes back empty, and the test fails.
func TestASavedRosterComesBackWhole(t *testing.T) {
	var roster Roster
	deeptest.Fill(t, reflect.ValueOf(&roster).Elem())
	if restored := throughJSON(t, roster); !reflect.DeepEqual(roster, restored) {
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
	before, each := workerLog()
	for _, roster := range []Roster{{}, FromEvents(append(before, each...))} {
		if restored := throughJSON(t, roster); !reflect.DeepEqual(roster, restored) {
			t.Errorf("the roster read back differs:\n%+v\n%+v", roster, restored)
		}
	}
}

// throughJSON saves the roster, writes and reads it as JSON, and restores it.
func throughJSON(t *testing.T, roster Roster) Roster {
	t.Helper()
	written, err := json.Marshal(roster.Save())
	if err != nil {
		t.Fatal(err)
	}
	var read Saved
	if err := json.Unmarshal(written, &read); err != nil {
		t.Fatal(err)
	}
	return Restore(read)
}
