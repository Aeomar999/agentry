package main

import (
	"os"
	"path/filepath"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/store"
	"gitlab.com/dev.chempa/agentry/internal/domain/policy"
	"gitlab.com/dev.chempa/agentry/internal/platform/tlsca"
)

const policyFile = "policy.toml"

// settingsFile holds extensions' settings and secrets, beside the policy.
const settingsFile = "features.toml"

// openPolicy loads the policy agents follow, writing the default on first
// start, and records it in the log if it isn't the one recorded last. The
// log, the policy itself, the settings file with its secrets, the tokens and
// the HTTPS CA's folder are protected: no agent may touch them, whatever the
// policy says. The CA's key is named on its own too, though its folder
// covers it, so a path that can't be fully known is still refused for
// spelling out its name.
func openPolicy(dataDir string, log *store.Store) (*policy.Live, error) {
	path := filepath.Join(dataDir, policyFile)
	home, _ := os.UserHomeDir()
	protected := []string{filepath.Join(dataDir, "agentry.db"), path, filepath.Join(dataDir, settingsFile),
		filepath.Join(dataDir, "ui-token"), filepath.Join(dataDir, hookTokenFile), filepath.Join(dataDir, deskTokenFile), filepath.Join(dataDir, helperTokenFile),
		tlsca.Dir(dataDir), tlsca.KeyPath(dataDir)}
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
	loaded := &policy.Change{Policy: live.Current()}
	if last == nil {
		log.Propose(policy.Loaded(live.Path(), loaded, "first loaded", policy.Agentry))
		return
	}
	if before := last.Text("text"); before != "" {
		loaded.Diff = policy.LineDiff("before", "after", before, loaded.Policy.Text())
	}
	log.Propose(policy.NoticedOnDisk(live.Path(), loaded))
}

// recordOnDisk records a change to the file the watcher noticed: the
// policy it put in force, or the file it refused.
func recordOnDisk(log *store.Store, live *policy.Live, loaded *policy.Change, refused error) {
	if refused != nil {
		log.Propose(policy.Refused(live.Path(), live.Current(), refused))
		return
	}
	log.Propose(policy.NoticedOnDisk(live.Path(), loaded))
}
