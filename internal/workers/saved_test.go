package workers

import (
	"reflect"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/deeptest"
)

// Every field, exported or not, is filled by reflection, and the saved form
// goes through JSON and gob as a snapshot writes it: a field added to Roster or
// Worker that Save and Restore don't carry comes back empty, and the test fails.
func TestASavedRosterComesBackWhole(t *testing.T) {
	var roster Roster
	deeptest.Fill(t, reflect.ValueOf(&roster).Elem())
	for _, encoding := range deeptest.Encodings {
		if restored := through(t, encoding, roster); !reflect.DeepEqual(roster, restored) {
			t.Errorf("the roster read back from %s differs:\n%+v\n%+v", encoding.Name, roster, restored)
		}
	}
}

func TestARestoredRosterSharesNothing(t *testing.T) {
	var roster Roster
	deeptest.Fill(t, reflect.ValueOf(&roster).Elem())
	saved := roster.Save()
	restored := Restore(saved)
	if !reflect.DeepEqual(roster, restored) {
		t.Fatalf("the restored roster differs:\n%+v\n%+v", roster, restored)
	}
	deeptest.Shared(t, "Roster", reflect.ValueOf(roster), reflect.ValueOf(restored))
	// Each side on its own, so a clone dropped from Save alone or from
	// Restore alone fails too. The invites hold only strings and times.
	for i := range roster.Workers {
		deeptest.Shared(t, "Workers[i]", reflect.ValueOf(roster.Workers[i]), reflect.ValueOf(saved.Workers[i].Worker))
		deeptest.Shared(t, "Saved.Workers[i].Worker", reflect.ValueOf(saved.Workers[i].Worker), reflect.ValueOf(restored.Workers[i]))
	}
}

// A roster replayed from its log, a new one and one with empty lists come
// back the same.
func TestAReplayedRosterComesBackWhole(t *testing.T) {
	before, each := workerLog()
	for _, roster := range []Roster{{}, {Workers: []Worker{}, invites: []invite{}}, FromEvents(append(before, each...))} {
		for _, encoding := range deeptest.Encodings {
			if restored := through(t, encoding, roster); !reflect.DeepEqual(roster, restored) {
				t.Errorf("the roster read back from %s differs:\n%#v\n%#v", encoding.Name, roster, restored)
			}
		}
	}
}

// through saves the roster, writes and reads it as encoding does, and restores it.
func through(t *testing.T, encoding deeptest.Encoding, roster Roster) Roster {
	t.Helper()
	var read Saved
	if err := encoding.RoundTrip(roster.Save(), &read); err != nil {
		t.Fatal(err)
	}
	return Restore(read)
}

// A snapshot's roster takes the events after it in place: restored at every
// cut of the log and applied the rest, it is the roster FromEvents gives.
func TestARestoredRosterTakesTheRestOfItsLog(t *testing.T) {
	before, each := workerLog()
	stream := append(before, each...)
	want := FromEvents(stream)
	for cut := range stream {
		roster := Restore(FromEvents(stream[:cut]).Save())
		for _, e := range stream[cut:] {
			roster.Apply(e)
		}
		if !reflect.DeepEqual(roster, want) {
			t.Errorf("restored after event %d, then applied the rest, the roster differs:\n%+v\n%+v", cut, roster, want)
		}
	}
}
