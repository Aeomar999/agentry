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
// log, the policy itself and the tokens are protected: no agent may
// touch them, whatever the policy says.
func openPolicy(dataDir string, log *store.Store) (*policy.Live, error) {
	path := filepath.Join(dataDir, policyFile)
	home, _ := os.UserHomeDir()
	protected := []string{filepath.Join(dataDir, "agentry.db"), path, filepath.Join(dataDir, "ui-token"),
		filepath.Join(dataDir, hookTokenFile), filepath.Join(dataDir, deskTokenFile), filepath.Join(dataDir, helperTokenFile)}
	live, err := policy.OpenLive(path, home, protected)
	if err != nil {
		return nil, err
	}
	history, err := log.Stream(events.PolicyStream)
	if err != nil {
		return nil, err
	}
	if last := lastLoaded(history); last == nil || last.Text("hash") != live.Current().Hash() {
		recordAtStart(log, live, last)
	}
	return live, nil
}

func lastLoaded(history []events.Event) *events.Event {
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Type == events.PolicyLoaded {
			return &history[i]
		}
	}
	return nil
}

// recordAtStart records the policy Agentry starts with, which is new to the
// log: the first, or one changed while Agentry wasn't running. Its diff is
// from the last version recorded, when the log holds that one's text.
func recordAtStart(log *store.Store, live *policy.Live, last *events.Event) {
	loaded, change := &policy.Change{Policy: live.Current()}, "first loaded"
	if last != nil {
		change = "changed on disk"
		if before := last.Text("text"); before != "" {
			loaded.Diff = policy.LineDiff("before", "after", before, loaded.Policy.Text())
		}
	}
	log.Propose(policy.Loaded(live.Path(), loaded, change, policy.Agentry))
}

// recordOnDisk records a change to the file the watcher noticed: the
// policy it put in force, or the file it refused.
func recordOnDisk(log *store.Store, live *policy.Live, loaded *policy.Change, refused error) {
	if refused != nil {
		log.Propose(policy.Refused(live.Path(), live.Current(), refused))
		return
	}
	log.Propose(policy.Loaded(live.Path(), loaded, "changed on disk", policy.Agentry))
}
