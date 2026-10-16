package main

import (
	"os"
	"path/filepath"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/policy"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

const policyFile = "policy.toml"

// openPolicy loads the policy agents follow, writing the default on first
// start, and records it in the log if it isn't the one recorded last. The
// log, the policy itself and the sign-in token are protected: no agent may
// touch them, whatever the policy says.
func openPolicy(dataDir string, log *store.Store) (*policy.Live, error) {
	path := filepath.Join(dataDir, policyFile)
	home, _ := os.UserHomeDir()
	protected := []string{filepath.Join(dataDir, "agentry.db"), path, filepath.Join(dataDir, "ui-token")}
	live, err := policy.OpenLive(path, home, protected)
	if err != nil {
		return nil, err
	}
	history, err := log.Stream(events.PolicyStream)
	if err != nil {
		return nil, err
	}
	if len(history) == 0 || history[len(history)-1].Text("hash") != live.Current().Hash() {
		recordPolicy(log, live, nil)
	}
	return live, nil
}

// recordPolicy writes policy.loaded for the policy in force, or
// policy.refused when the file on disk was refused.
func recordPolicy(log *store.Store, live *policy.Live, refused error) {
	agentry := events.Actor{Kind: events.System, ID: "policy"}
	p := events.Proposal{Stream: events.PolicyStream, Actor: agentry, Type: events.PolicyLoaded,
		Data: map[string]any{"hash": live.Current().Hash(), "path": live.Path(), "profiles": live.Current().ProfileNames()}}
	if refused != nil {
		p.Type, p.Data = events.PolicyRefused, map[string]any{"reason": refused.Error(), "path": live.Path(), "kept": live.Current().Hash()}
	}
	log.Propose(p)
}
