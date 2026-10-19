package policy

import (
	"errors"
	"fmt"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Agentry records each version of the policy it puts in force as
// policy.loaded in the policy stream, with the whole text, so any of them
// can be restored, and each file it refuses as policy.refused. Events from
// before the text was recorded carry only the hash: they show in the
// history but can't be restored.

// Agentry is who records what it loads by itself: at start, and changes
// it notices on disk.
var Agentry = events.Actor{Kind: events.System, ID: "policy"}

// Loaded is the policy.loaded event for c, put in force at path by actor.
// change says in a few words what changed: "added profile read-mostly".
func Loaded(path string, c *Change, change string, actor events.Actor) events.Proposal {
	data := map[string]any{"hash": c.Policy.Hash(), "path": path, "profiles": c.Policy.ProfileNames(),
		"text": c.Policy.Text(), "change": change}
	if c.Diff != "" {
		data["diff"] = c.Diff
	}
	if c.Backup != "" {
		data["backup"] = c.Backup
	}
	return events.Proposal{Stream: events.PolicyStream, Actor: actor, Type: events.PolicyLoaded, Data: data}
}

// Refused is the policy.refused event for a file at path that doesn't
// load, while kept stays in force.
func Refused(path string, kept *Policy, refused error) events.Proposal {
	return events.Proposal{Stream: events.PolicyStream, Actor: Agentry, Type: events.PolicyRefused,
		Data: map[string]any{"reason": refused.Error(), "path": path, "kept": kept.Hash()}}
}

// A Version is one entry of the policy's history.
type Version struct {
	Position int64        `json:"position"`
	At       time.Time    `json:"at"`
	Actor    events.Actor `json:"actor"`
	Change   string       `json:"change"` // what changed; for a refused file, why it was refused
	Refused  bool         `json:"refused,omitempty"`
	Hash     string       `json:"hash"`             // of the version loaded, or of the one kept over the refused file
	Diff     string       `json:"diff,omitempty"`   // from the version in force before
	Backup   string       `json:"backup,omitempty"` // the file before Agentry changed it
	// CanRevert is true for a version whose text the log holds.
	CanRevert bool `json:"canRevert"`
}

// History is the policy stream's versions, newest first.
func History(stream []events.Event) []Version {
	history := []Version{}
	for i := len(stream) - 1; i >= 0; i-- {
		e := stream[i]
		v := Version{Position: e.Position, At: e.At, Actor: e.Actor, Change: e.Text("change"), Hash: e.Text("hash"),
			Diff: e.Text("diff"), Backup: e.Text("backup"), CanRevert: e.Text("text") != ""}
		switch e.Type {
		case events.PolicyLoaded:
			if v.Change == "" {
				v.Change = "loaded" // recorded before changes were described
			}
		case events.PolicyRefused:
			v.Refused, v.Change, v.Hash = true, e.Text("reason"), e.Text("kept")
		default:
			continue
		}
		history = append(history, v)
	}
	return history
}

// ErrNoVersion is a position in the policy stream that holds no version.
var ErrNoVersion = errors.New("the policy has no such version")

// VersionAt is the text of the version recorded at position in the policy
// stream, and when it was put in force. A position holding no version is
// ErrNoVersion; any other error is a version whose text the log lacks.
func VersionAt(stream []events.Event, position int64) (string, time.Time, error) {
	for _, e := range stream {
		if e.Position != position {
			continue
		}
		if e.Type != events.PolicyLoaded || e.Text("text") == "" {
			return "", time.Time{}, fmt.Errorf("the log doesn't hold the text of version %d, so it can't be restored", position)
		}
		return e.Text("text"), e.At, nil
	}
	return "", time.Time{}, fmt.Errorf("version %d: %w", position, ErrNoVersion)
}
