package schedules

import (
	"errors"
	"reflect"
	"testing"
)

// store is a Source over a map of streams, in order; one can be broken.
type store struct {
	order  []string
	held   map[string]any
	broken map[string]error
}

func (s store) Owned(stream string) (any, error) { return s.held[stream], s.broken[stream] }

func (s store) OwnedIDs(prefix string) []string {
	if prefix != Prefix {
		return nil
	}
	return s.order
}

func TestGetAndAllReadTheSchedules(t *testing.T) {
	first, third := &Schedule{ID: "schedule-1", State: On}, &Schedule{ID: "schedule-3", State: Removed}
	broken := errors.New("schedule-2 is broken")
	source := store{order: []string{"schedule-1", "schedule-2", "schedule-3", "schedule-4"},
		held:   map[string]any{"schedule-1": first, "schedule-3": third},
		broken: map[string]error{"schedule-2": broken}}

	if got, err := Get(source, "schedule-1"); got != first || err != nil {
		t.Errorf("Get(schedule-1) = %v, %v", got, err)
	}
	if got, err := Get(source, "schedule-2"); got != nil || err != broken {
		t.Errorf("Get(a broken schedule) = %v, %v", got, err)
	}
	if got, err := Get(source, "schedule-4"); got != nil || err != nil {
		t.Errorf("Get(an empty stream) = %v, %v", got, err)
	}
	if got, err := Get(source, "schedule-9"); got != nil || err != nil {
		t.Errorf("Get(no such schedule) = %v, %v", got, err)
	}
	if got, want := All(source), []*Schedule{first, third}; !reflect.DeepEqual(got, want) {
		t.Errorf("All = %v, want %v", got, want)
	}
}
