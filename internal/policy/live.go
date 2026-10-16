package policy

import (
	"context"
	"crypto/sha256"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

// Live is the policy in force, reloaded when policy.toml changes. A file
// with an error never replaces a working policy: the one before it stays,
// and Problem says what is wrong until the file is fixed.
type Live struct {
	path      string
	home      string
	protected []string
	current   atomic.Pointer[Policy]

	mu       sync.Mutex
	problem  string
	lastSeen [32]byte // the content last loaded or refused, so each version is judged once
}

// OpenLive loads path, writing the default policy there first if there is
// none. It fails if that first load fails: Agentry never runs without a policy.
func OpenLive(path, home string, protected []string) (*Live, error) {
	if err := EnsureFile(path); err != nil {
		return nil, err
	}
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	first, err := Parse(text, home, protected)
	if err != nil {
		return nil, err
	}
	l := &Live{path: path, home: home, protected: protected, lastSeen: sha256.Sum256(text)}
	l.current.Store(first)
	return l, nil
}

func (l *Live) Decide(profile string, call Call) Verdict {
	return l.current.Load().Decide(profile, call)
}

func (l *Live) Current() *Policy { return l.current.Load() }

func (l *Live) Path() string { return l.path }

// Problem is why the file on disk isn't in force, or "" when it is.
func (l *Live) Problem() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.problem
}

// Reload reads the file once. It reports a newly loaded policy, or the
// error of a newly refused file; both are nil when the file hasn't changed.
func (l *Live) Reload() (*Policy, error) {
	text, err := os.ReadFile(l.path)
	l.mu.Lock()
	defer l.mu.Unlock()
	if err != nil {
		return nil, l.refuseLocked(err, [32]byte{})
	}
	sum := sha256.Sum256(text)
	if sum == l.lastSeen {
		return nil, nil
	}
	loaded, err := Parse(text, l.home, l.protected)
	if err != nil {
		return nil, l.refuseLocked(err, sum)
	}
	l.lastSeen, l.problem = sum, ""
	l.current.Store(loaded)
	return loaded, nil
}

func (l *Live) refuseLocked(err error, sum [32]byte) error {
	if l.problem == err.Error() {
		return nil // already reported
	}
	l.lastSeen, l.problem = sum, err.Error()
	return err
}

// Watch reloads every interval until ctx ends, and reports each change.
func (l *Live) Watch(ctx context.Context, interval time.Duration, report func(loaded *Policy, refused error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if loaded, refused := l.Reload(); loaded != nil || refused != nil {
				report(loaded, refused)
			}
		}
	}
}

// ProfileSummary is a profile as the page shows it.
type ProfileSummary struct {
	Name      string        `json:"name"`
	Why       string        `json:"why"`
	Otherwise Decision      `json:"otherwise"`
	Rules     []RuleSummary `json:"rules"`
}

type RuleSummary struct {
	Name     string   `json:"name"`
	Decision Decision `json:"decision"`
	Why      string   `json:"why"`
}

func (p *Policy) Summary() []ProfileSummary {
	var summaries []ProfileSummary
	for _, name := range p.ProfileNames() {
		prof := p.profiles[name]
		summary := ProfileSummary{Name: name, Why: prof.why, Otherwise: prof.otherwise, Rules: []RuleSummary{}}
		for _, r := range prof.rules {
			summary.Rules = append(summary.Rules, RuleSummary{Name: r.name, Decision: r.decision, Why: r.why})
		}
		summaries = append(summaries, summary)
	}
	return summaries
}
