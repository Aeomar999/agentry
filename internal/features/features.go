// Package features is what a feature, such as ideas, adds to Agentry. It is
// a leaf: core packages import it, and it imports none that could import it
// back. Core reads a list of features; it never names one.
package features

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/settings"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workers"
)

// A Feature is one capability a team or a setting turns on. Every field but
// Name may be left out.
type Feature struct {
	Name        string             // "ideas": its manifest entry, routes and UI folder use it
	Version     int                // raised when its events or state change
	Streams     *Streams           // the streams it owns, if any
	Writes      []events.Type      // the core types it proposes, beside its own: ideas makes a task.created (Checked)
	Coordinator *Coordinator       // its part in coordinators' briefs and decisions
	Routes      func(Core) []Route // its HTTP API, which web mounts behind each Route's Auth
	Manifest    func(Reader) Entry // whether the page shows it, and why

	// In agentry mcp, a separate process that reaches the server only over
	// its API: Tools are its MCP tools for a session in repo ("" outside
	// one), and Instructions its lines in the server instructions Claude
	// Code reads, each a "- " bullet; both only while the feature is
	// enabled or visible. Waiting is what of it waits for you, about repo
	// ("" when not known), read over the API until the server lists it in
	// story 13 (GET /api/waiting).
	Tools        func(agentry API, repo string) []Tool
	Instructions string
	Waiting      func(agentry API, repo string) []Waiting

	// Jobs is its background work, which serve starts beside the loop
	// (StartJobs) and ctx ends. A panic in it stops it, not the server.
	Jobs func(ctx context.Context, c Core)
}

// Streams are a feature's own streams, each named Prefix and an ID: how the
// state builds and keeps each one. Its types are in its events.Registration.
// FromEvents builds a stream's state from its accepted events, as at Open;
// Next is the state after one more event, built on a copy, so prev stays as
// it was for whoever holds it. Both as tasks.FromEvents and tasks.Next, and a
// nil state means the stream holds nothing yet.
type Streams struct {
	Prefix     string // "idea-"
	FromEvents func(accepted []events.Event) any
	Next       func(prev any, e events.Event) any
}

// Reader is the read side of the state, for features and for the rules:
// current.State implements it, and rules.State is this type. A feature
// never holds the store. Broken is why a stream could not be built, or nil.
// NextTask is the stream Store.Create names for the next task.
type Reader interface {
	Task(id string) (*tasks.Task, error)
	TaskIDs() []string
	NextTask() string
	Roster() agents.Roster
	Workers() workers.Roster
	Connections() connections.Roster
	Settings() settings.Settings
	Broken(stream string) error
	Owned(stream string) (any, error) // a feature's stream, as its Next left it; nil, nil when it holds nothing
	OwnedIDs(prefix string) []string  // the streams with a prefix, in log order, broken ones too
}

// Proposer writes to the log, as *store.Store does. A feature is given the
// one Checked made for it, in its Core, never the store itself.
type Proposer interface {
	Propose(p events.Proposal) (events.Event, *events.Rejection, error)
	Create(prefix string, p events.Proposal) (events.Event, *events.Rejection, error)
}

// EventLog reads a stream's accepted events from the log, as *store.Store
// does. It only reads: a feature writes through its Proposer.
type EventLog interface {
	Stream(name string) ([]events.Event, error)
}

// ReadOnly is log with only EventLog's methods: no type assertion on what
// it returns gets the store, or a way to write, back.
func ReadOnly(log EventLog) EventLog { return readOnly{log} }

type readOnly struct{ log EventLog }

func (r readOnly) Stream(name string) ([]events.Event, error) { return r.log.Stream(name) }

// Core is all a feature's hooks and routes may use: it has no store. Events reads the
// log and never writes to it. Defaults are the server's limits for a task
// Agentry makes that comes from no other task. Settings are the feature's
// own table in the settings file.
type Core struct {
	Log      Proposer // from Checked
	Events   EventLog
	Read     Reader
	Defaults tasks.Limits
	Settings Settings
}

// Coordinator is a feature's part in the coordinator loop, for the
// coordinators For says. Brief and OnDone run on the state of the moment,
// so Brief is given the Reader to read the feature's streams from.
type Coordinator struct {
	For      func(coordinator agents.Agent) bool                          // a Visionaries coordinator
	Brief    func(state Reader, task *tasks.Task) []Section               // more sections for its brief
	Decision string                                                       // what its done decision may add, for "Your decision"
	Field    string                                                       // the done decision's field it reads: "ideas"
	Check    func(field json.RawMessage) error                            // optional: refuses a done whose field it can't read, so the coordinator is asked again; it never writes
	OnDone   func(c Core, task *tasks.Task, field json.RawMessage) string // acts on it; returns a note for the summary
}

// A Route is one endpoint of a feature's HTTP API. Web mounts it behind the
// wrapper its Auth names; there is no way to mount one without, and none
// sits behind the wrapper that hands out the UI token. Its writes should be
// made as you (httpapi.You), with your authority and no more, and a signed
// route's as System (httpapi.System); that is the route's to keep, since the
// checked proposer limits only the event type it writes, not the actor.
type Route struct {
	Pattern string // as http.ServeMux takes it: "GET /api/ideas"
	Auth    Auth
	Handler http.HandlerFunc
}

// Auth says who may call a Route: UI, Desk or Signed(verify). The zero Auth
// is UI. It holds a func, so it can't be compared: ask DeskToo and Verify.
type Auth struct {
	deskToo bool
	verify  func(r *http.Request, body []byte) error
}

var (
	UI   = Auth{}              // the page, and Claude Code connections, which carry the UI token
	Desk = Auth{deskToo: true} // those, and the desk with its own token
)

// Signed is for a webhook: a request whose body verify accepts, checked
// against that hook's secret, and no token opens it. verify is given the
// request for its headers, so a missing signature is its to refuse. A nil
// verify panics: a signed route that checks nothing would let anyone write.
func Signed(verify func(r *http.Request, body []byte) error) Auth {
	if verify == nil {
		panic("features.Signed needs a verify: a signed route that checks nothing would let anyone in")
	}
	return Auth{verify: verify}
}

// DeskToo is whether the desk's token opens the route too: Desk.
func (a Auth) DeskToo() bool { return a.deskToo }

// Verify is a Signed route's check of a request and its body, nil for UI and Desk.
func (a Auth) Verify() func(r *http.Request, body []byte) error { return a.verify }

// Section is one more part of a coordinator's brief.
type Section struct{ Heading, Text string }

// Entry is whether the page shows a feature, and why.
type Entry struct {
	Enabled bool   `json:"enabled"`         // its trigger is present, so it can make new data
	Visible bool   `json:"visible"`         // the page shows it: enabled, or it has data to look at
	Why     string `json:"why,omitempty"`   // one line saying why it is on, kept or off
	Badge   int    `json:"badge,omitempty"` // a count for the sidebar, such as open ideas
}

// Line is a feature's line in GET /api/features.
type Line struct {
	Name string `json:"name"`
	Entry
}

// Manifest is every feature's line, in list's order, as of state. A feature
// with no Manifest is listed hidden, and so is one whose Manifest panics,
// with Why saying so: one feature's bug doesn't cost the others their lines.
func Manifest(list []Feature, state Reader) []Line {
	lines := []Line{}
	for _, f := range list {
		line := Line{Name: f.Name}
		if f.Manifest != nil {
			line.Entry = entryOf(f, state)
		}
		lines = append(lines, line)
	}
	return lines
}

// entryOf is f's Manifest, a panic in it turned into a hidden, disabled
// entry.
func entryOf(f Feature, state Reader) (entry Entry) {
	defer func() {
		if recovered := recover(); recovered != nil {
			entry = Entry{Why: fmt.Sprintf("its manifest failed: %v", recovered)}
		}
	}()
	return f.Manifest(state)
}
