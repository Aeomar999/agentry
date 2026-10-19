package tasks

import "testing"

// A new task takes the limits of the task it comes from, unlimited ones too,
// and the server's defaults, unlimited or not, when it comes from none.
func TestANewTaskTakesItsSourcesLimitsElseTheDefaults(t *testing.T) {
	for _, c := range []struct {
		name     string
		defaults Limits
		from     *Task
		want     Limits
	}{
		{"unlimited defaults", Limits{}, nil, Limits{}},
		{"set defaults", Limits{Budget: 5_000_000, MaxTurns: 24}, nil, Limits{Budget: 5_000_000, MaxTurns: 24}},
		{"from a task", Limits{Budget: 5_000_000, MaxTurns: 24}, &Task{Budget: 700, MaxTurns: 3}, Limits{Budget: 700, MaxTurns: 3}},
		{"from an unlimited task", Limits{Budget: 5_000_000, MaxTurns: 24}, &Task{}, Limits{}},
	} {
		if got := NewLimits(c.defaults, c.from); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}
