package connections

import "time"

// Saved is a roster in a form that can be written out, as JSON, and read
// back whole: what Roster and Connection keep unexported, which JSON would
// leave out, is held in exported fields.
type Saved struct {
	Connections []SavedConnection `json:"connections"`
	Invites     []SavedInvite     `json:"invites"`
}

// SavedConnection is a connection and its credential's hash.
type SavedConnection struct {
	Connection Connection `json:"connection"` // credential left empty: it is the field below
	Credential string     `json:"credential"`
}

// SavedInvite is an install code: its hash, when it stops working, and
// whether it was used.
type SavedInvite struct {
	Code    string    `json:"code"`
	Expires time.Time `json:"expires"`
	Used    bool      `json:"used"`
}

// Save is the roster's saved form. It shares nothing with the roster.
func (r Roster) Save() Saved {
	var saved Saved
	if r.Connections != nil {
		saved.Connections = make([]SavedConnection, len(r.Connections))
	}
	for i, c := range r.Connections {
		saved.Connections[i] = SavedConnection{Connection: c, Credential: c.credential}
		saved.Connections[i].Connection.credential = ""
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
	if s.Connections != nil {
		r.Connections = make([]Connection, len(s.Connections))
	}
	for i, saved := range s.Connections {
		r.Connections[i] = saved.Connection
		r.Connections[i].credential = saved.Credential
	}
	if s.Invites != nil {
		r.invites = make([]invite, len(s.Invites))
	}
	for i, saved := range s.Invites {
		r.invites[i] = invite{code: saved.Code, expires: saved.Expires, used: saved.Used}
	}
	return r
}
