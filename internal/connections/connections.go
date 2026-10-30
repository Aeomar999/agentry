// Package connections is the machines whose Claude Code has Agentry's
// tools (agentry mcp), connected from the Integrations page: who they are,
// and how they prove it. Like workers, they are events in the log.
//
// You make an install command on the page. Its code works once, for
// InviteLifetime, and only its hash is kept. The install swaps it for the
// machine's own credential, which it keeps; the log holds only its hash.
// Revoking a connection stops its credential working.
package connections

import (
	"crypto/subtle"
	"slices"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
)

// InviteLifetime is how long an install command works.
const InviteLifetime = 15 * time.Minute

type Connection struct {
	ID     string    `json:"id"`
	Host   string    `json:"host"` // the machine's own name for itself
	User   string    `json:"user"`
	OS     string    `json:"os"`
	MadeAt time.Time `json:"madeAt"`

	credential string // its hash
}

type invite struct {
	code    string // its hash
	expires time.Time
	used    bool
}

// Roster is every connection not revoked, oldest first, and the codes made.
type Roster struct {
	Connections []Connection
	invites     []invite
}

func FromEvents(stream []events.Event) Roster {
	var r Roster
	for _, e := range stream {
		r.Apply(e)
	}
	return r
}

// Next is the roster after e, built on a copy: r itself is left as it was.
// Copying the slices is enough, as Apply replaces whole any slice inside them.
func (r Roster) Next(e events.Event) Roster {
	r.Connections, r.invites = slices.Clone(r.Connections), slices.Clone(r.invites)
	r.Apply(e)
	return r
}

// Apply changes the roster in place by one accepted event of its stream.
func (r *Roster) Apply(e events.Event) {
	switch e.Type {
	case events.ConnectionInvited:
		expires, _ := time.Parse(time.RFC3339, e.Text("expires"))
		r.invites = append(r.invites, invite{code: e.Text("code"), expires: expires})
	case events.ConnectionMade:
		if i := r.inviteIndex(e.Text("code")); i >= 0 {
			r.invites[i].used = true
		}
		r.Connections = append(r.Connections, Connection{ID: e.Text("id"), Host: e.Text("host"), User: e.Text("user"),
			OS: e.Text("os"), MadeAt: e.At, credential: e.Text("credential")})
	case events.ConnectionRevoked:
		r.Connections = slices.DeleteFunc(r.Connections, func(c Connection) bool { return c.ID == e.Text("id") })
	}
}

// WithCredential is the connection the credential belongs to.
func (r Roster) WithCredential(credential string) (Connection, bool) {
	given := workers.Hash(credential)
	for _, c := range r.Connections {
		if subtle.ConstantTimeCompare([]byte(given), []byte(c.credential)) == 1 {
			return c, true
		}
	}
	return Connection{}, false
}

// CanConnect says whether a code (its hash) was made here, is unused and unexpired.
func (r Roster) CanConnect(codeHash string, now time.Time) bool {
	i := r.inviteIndex(codeHash)
	return i >= 0 && !r.invites[i].used && now.Before(r.invites[i].expires)
}

// Has says whether the connection is there.
func (r Roster) Has(id string) bool {
	return slices.ContainsFunc(r.Connections, func(c Connection) bool { return c.ID == id })
}

func (r Roster) inviteIndex(codeHash string) int {
	return slices.IndexFunc(r.invites, func(i invite) bool {
		return subtle.ConstantTimeCompare([]byte(i.code), []byte(codeHash)) == 1
	})
}

// Invite makes an install code, and the event that records its hash.
func Invite(by events.Actor, now time.Time) (code string, p events.Proposal) {
	code = workers.NewSecret()
	return code, events.Proposal{Stream: events.ConnectionsStream, Actor: by, Type: events.ConnectionInvited,
		Data: map[string]any{"code": workers.Hash(code), "expires": now.Add(InviteLifetime).UTC().Format(time.RFC3339)}}
}

// Machine is what an install says about where it runs.
type Machine struct {
	Host string `json:"host"`
	User string `json:"user"`
	OS   string `json:"os"`
}

// Connect makes the machine's credential, and the event that lets it in
// with the code it was given.
func Connect(code string, m Machine) (credential string, p events.Proposal) {
	credential = workers.NewSecret()
	return credential, events.Proposal{Stream: events.ConnectionsStream, Actor: actor, Type: events.ConnectionMade,
		Data: map[string]any{"id": workers.NewSecret()[:8], "code": workers.Hash(code), "credential": workers.Hash(credential),
			"host": m.Host, "user": m.User, "os": m.OS}}
}

// Revocation is the event that stops a connection's credential working.
func Revocation(by events.Actor, id string) events.Proposal {
	return events.Proposal{Stream: events.ConnectionsStream, Actor: by, Type: events.ConnectionRevoked, Data: map[string]any{"id": id}}
}

// actor records connections: Agentry, on the install's word and code.
var actor = events.Actor{Kind: events.System, ID: "connections"}
