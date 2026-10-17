// Package workers is the machines, besides the one Agentry runs on, that
// agents can run on: who has joined, how they prove it, and who is online.
// Like agents, they are events in the log, so the page and the rules read
// the same list.
//
// A machine joins with a code you make on the page. The code works once,
// for an hour, and only its hash is kept. Using it gives the machine its
// own credential, which it keeps; the log holds only that credential's hash.
package workers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"slices"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Local is the name of the machine Agentry runs on. It is always there, so
// no worker may take its name.
const Local = "local"

// InviteLifetime is how long a join code works.
const InviteLifetime = time.Hour

type Worker struct {
	Name     string    `json:"name"`
	Host     string    `json:"host"` // the machine's own name for itself
	OS       string    `json:"os"`
	Arch     string    `json:"arch"`
	JoinedAt time.Time `json:"joinedAt"`

	// What it said when it last connected.
	Online  bool      `json:"online"`
	Since   time.Time `json:"since"`   // when it last came online or went offline
	Version string    `json:"version"` // of agentry on it
	Engines []string  `json:"engines"` // the engines it has, as "claude 2.1.301"
	Missing []string  `json:"missing"` // tools agents need that it lacks, such as tmux

	credential string // its hash
}

// Has says whether the worker said it has the named engine.
func (w Worker) Has(engine string) bool {
	return slices.ContainsFunc(w.Engines, func(e string) bool { return e == engine || strings.HasPrefix(e, engine+" ") })
}

type invite struct {
	code    string // its hash
	expires time.Time
	used    bool
}

// Roster is every worker that has joined and not been removed, in the order
// they joined, and the join codes made so far.
type Roster struct {
	Workers []Worker
	invites []invite
}

func FromEvents(stream []events.Event) Roster {
	var r Roster
	for _, e := range stream {
		switch e.Type {
		case events.WorkerInvited:
			expires, _ := time.Parse(time.RFC3339, e.Text("expires"))
			r.invites = append(r.invites, invite{code: e.Text("code"), expires: expires})
		case events.WorkerJoined:
			if i := r.inviteIndex(e.Text("code")); i >= 0 {
				r.invites[i].used = true
			}
			r.Workers = append(r.Workers, Worker{Name: e.Text("name"), Host: e.Text("host"), OS: e.Text("os"),
				Arch: e.Text("arch"), JoinedAt: e.At, Since: e.At, credential: e.Text("credential")})
		case events.WorkerConnected:
			if w := r.find(e.Text("name")); w != nil {
				w.Online, w.Since, w.Version = true, e.At, e.Text("version")
				w.Engines, w.Missing = e.List("engines"), e.List("missing")
				if host := e.Text("host"); host != "" {
					w.Host, w.OS, w.Arch = host, e.Text("os"), e.Text("arch")
				}
			}
		case events.WorkerDisconnected:
			if w := r.find(e.Text("name")); w != nil {
				w.Online, w.Since = false, e.At
			}
		case events.WorkerRemoved:
			r.Workers = slices.DeleteFunc(r.Workers, func(w Worker) bool { return w.Name == e.Text("name") })
		}
	}
	return r
}

func (r Roster) Get(name string) (Worker, bool) {
	if w := r.find(name); w != nil {
		return *w, true
	}
	return Worker{}, false
}

// Names are every worker's name, in the order they joined.
func (r Roster) Names() []string {
	names := []string{}
	for _, w := range r.Workers {
		names = append(names, w.Name)
	}
	return names
}

// WithCredential is the worker the credential belongs to.
func (r Roster) WithCredential(credential string) (Worker, bool) {
	given := Hash(credential)
	for _, w := range r.Workers {
		if subtle.ConstantTimeCompare([]byte(given), []byte(w.credential)) == 1 {
			return w, true
		}
	}
	return Worker{}, false
}

// CanJoin says whether a code (its hash) is one made here, unused and not expired.
func (r Roster) CanJoin(codeHash string, now time.Time) bool {
	i := r.inviteIndex(codeHash)
	return i >= 0 && !r.invites[i].used && now.Before(r.invites[i].expires)
}

func (r Roster) find(name string) *Worker {
	for i := range r.Workers {
		if r.Workers[i].Name == name {
			return &r.Workers[i]
		}
	}
	return nil
}

func (r Roster) inviteIndex(codeHash string) int {
	return slices.IndexFunc(r.invites, func(i invite) bool {
		return subtle.ConstantTimeCompare([]byte(i.code), []byte(codeHash)) == 1
	})
}

// Invite makes a join code, and the event that records its hash.
func Invite(by events.Actor, now time.Time) (code string, p events.Proposal) {
	code = NewSecret()
	return code, events.Proposal{Stream: events.WorkersStream, Actor: by, Type: events.WorkerInvited,
		Data: map[string]any{"code": Hash(code), "expires": now.Add(InviteLifetime).UTC().Format(time.RFC3339)}}
}

// Machine is what a worker says about itself.
type Machine struct {
	Host    string   `json:"host"`
	OS      string   `json:"os"`
	Arch    string   `json:"arch"`
	Version string   `json:"version"`
	Engines []string `json:"engines"`
	Missing []string `json:"missing"`
}

// Join makes the machine's credential, and the event that lets it in under
// name with the code it was given.
func Join(name, code string, m Machine) (credential string, p events.Proposal) {
	credential = NewSecret()
	return credential, events.Proposal{Stream: events.WorkersStream, Actor: actor, Type: events.WorkerJoined,
		Data: map[string]any{"name": name, "code": Hash(code), "credential": Hash(credential),
			"host": m.Host, "os": m.OS, "arch": m.Arch}}
}

// Connected and Disconnected are the events for a worker coming online and going offline.
func Connected(name string, m Machine) events.Proposal {
	return events.Proposal{Stream: events.WorkersStream, Actor: actor, Type: events.WorkerConnected,
		Data: map[string]any{"name": name, "host": m.Host, "os": m.OS, "arch": m.Arch, "version": m.Version,
			"engines": orEmpty(m.Engines), "missing": orEmpty(m.Missing)}}
}

func Disconnected(name string) events.Proposal {
	return events.Proposal{Stream: events.WorkersStream, Actor: actor, Type: events.WorkerDisconnected,
		Data: map[string]any{"name": name}}
}

// Removal is the event that takes a worker away; its credential stops working.
func Removal(by events.Actor, name string) events.Proposal {
	return events.Proposal{Stream: events.WorkersStream, Actor: by, Type: events.WorkerRemoved,
		Data: map[string]any{"name": name}}
}

// Left is the event for a worker taking itself off (`agentry worker leave`).
func Left(name string) events.Proposal { return Removal(actor, name) }

// actor is who records joins and connections: Agentry, on the worker's word.
var actor = events.Actor{Kind: events.System, ID: "workers"}

// NewSecret is a random code or credential.
func NewSecret() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// Hash is what the log keeps of a code or credential.
func Hash(secret string) string {
	sum := sha256.Sum256([]byte(secret))
	return hex.EncodeToString(sum[:])
}

func orEmpty(list []string) []string {
	if list == nil {
		return []string{}
	}
	return list
}
