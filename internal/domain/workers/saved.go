package workers

import (
	"slices"
	"time"
)

// Saved is a roster in a form that can be written out, as JSON, and read
// back whole: what Roster and Worker keep unexported, which JSON would
// leave out, is held in exported fields.
type Saved struct {
	Workers []SavedWorker `json:"workers"`
	Invites []SavedInvite `json:"invites"`
}

// SavedWorker is a worker and its credential's hash.
type SavedWorker struct {
	Worker     Worker `json:"worker"` // credential left empty: it is the field below
	Credential string `json:"credential"`
}

// SavedInvite is a join code: its hash, when it stops working, and whether
// it was used.
type SavedInvite struct {
	Code    string    `json:"code"`
	Expires time.Time `json:"expires"`
	Used    bool      `json:"used"`
}

// Save is the roster's saved form. It shares nothing with the roster.
func (r Roster) Save() Saved {
	var saved Saved
	if r.Workers != nil {
		saved.Workers = make([]SavedWorker, len(r.Workers))
	}
	for i, w := range r.Workers {
		saved.Workers[i] = SavedWorker{Worker: w.clone(), Credential: w.credential}
		saved.Workers[i].Worker.credential = ""
	}
	if r.invites != nil {
		saved.Invites = make([]SavedInvite, len(r.invites))
	}
	for i, invite := range r.invites {
		saved.Invites[i] = SavedInvite{Code: invite.code, Expires: invite.expires, Used: invite.used}
	}
	return saved
}

// Restore is the roster s was saved from. It shares nothing with s.
func Restore(s Saved) Roster {
	var r Roster
	if s.Workers != nil {
		r.Workers = make([]Worker, len(s.Workers))
	}
	for i, saved := range s.Workers {
		r.Workers[i] = saved.Worker.clone()
		r.Workers[i].credential = saved.Credential
	}
	if s.Invites != nil {
		r.invites = make([]invite, len(s.Invites))
	}
	for i, saved := range s.Invites {
		r.invites[i] = invite{code: saved.Code, expires: saved.Expires, used: saved.Used}
	}
	return r
}

func (w Worker) clone() Worker {
	w.Engines, w.Missing = slices.Clone(w.Engines), slices.Clone(w.Missing)
	return w
}
