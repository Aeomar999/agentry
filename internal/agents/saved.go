package agents

import "slices"

// Saved is a roster in a form that can be written out, as JSON, and read
// back whole.
type Saved []SavedAgent

// SavedAgent is an agent, and beside it what Agent keeps unexported, which
// JSON would leave out.
type SavedAgent struct {
	Agent     Agent `json:"agent"` // untouched and patched left zero: they are the fields below
	Untouched bool  `json:"untouched"`
	Patched   int   `json:"patched"`
}

// Save is the roster's saved form. It shares nothing with the roster.
func (r Roster) Save() Saved {
	if r == nil {
		return nil
	}
	saved := make(Saved, len(r))
	for i, a := range r {
		saved[i] = SavedAgent{Agent: a.clone(), Untouched: a.untouched, Patched: a.patched}
		saved[i].Agent.untouched, saved[i].Agent.patched = false, 0
	}
	return saved
}

// Restore is the roster s was saved from. It shares nothing with s.
func Restore(s Saved) Roster {
	if s == nil {
		return nil
	}
	r := make(Roster, len(s))
	for i, saved := range s {
		r[i] = saved.Agent.clone()
		r[i].untouched, r[i].patched = saved.Untouched, saved.Patched
	}
	return r
}

func (a Agent) clone() Agent {
	a.Workers = slices.Clone(a.Workers)
	return a
}
