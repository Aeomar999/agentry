package policy

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

	// mu is held from reading the file to storing what it holds, so a
	// change Agentry makes and a reload never see each other half done.
	mu       sync.Mutex
	problem  string
	lastSeen [32]byte // the content last loaded or refused, so each version is judged once
}

// A Change is a new version of the policy put in force.
type Change struct {
	Policy *Policy
	Before *Policy // the version in force until then
	Diff   string  // from Before's text to Policy's
	Backup string  // the file as it was, when Agentry wrote the change; "" when it didn't
}

// ErrStale refuses a change made to a version of the policy that is no
// longer the file on disk.
var ErrStale = errors.New("the policy file changed since you loaded it; reload and try again")

// A Refusal is a change Agentry won't make, and why: the version seen is
// stale, the file doesn't load, the edit doesn't apply or wouldn't load,
// its check refused it, or the file is a link. Any other error from Change
// is a failure to read or write the file.
type Refusal struct{ Err error }

func (r *Refusal) Error() string { return r.Err.Error() }

func (r *Refusal) Unwrap() error { return r.Err }

// keepBackups is how many of its backups Agentry keeps beside the policy:
// the newest, as each is named by its time. Older ones go after each
// change it writes, so the folder doesn't fill with them.
const keepBackups = 20

// OpenLive loads path, writing the default policy there first if there is
// none. It fails if that first load fails: Agentry never runs without a policy.
// A file that doesn't load is named in the error, with its newest backup.
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
		return nil, refusedAtStart(path, err)
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
func (l *Live) Reload() (*Change, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.reloadLocked()
}

func (l *Live) reloadLocked() (*Change, error) {
	text, err := os.ReadFile(l.path)
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
	return l.storeLocked(loaded, ""), nil
}

// An Edit is a change to make to the policy file.
type Edit struct {
	Seen   string                            // the hash of the version it was made against
	Apply  func(text []byte) ([]byte, error) // the file's text, changed; an error refuses the edit
	Check  func(next *Policy) error          // may refuse the new version before it is written; nil for none
	Record func(*Change)                     // logs it before any other change or reload can start; nil for none
}

// Change makes the edit to the policy file and puts the result in force.
// When the version e.Seen isn't the one in force, or the file on disk
// isn't it (changed but not reloaded yet), it refuses with ErrStale. Each
// refusal is a *Refusal. The old file is backed up first, as WriteFile does.
func (l *Live) Change(e Edit, now time.Time) (*Change, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	before := l.current.Load()
	text, err := os.ReadFile(l.path)
	if err != nil {
		return nil, err
	}
	if l.problem != "" {
		return nil, &Refusal{fmt.Errorf("policy.toml doesn't load, so Agentry won't change it until you fix it: %s", l.problem)}
	}
	if e.Seen != before.Hash() || string(text) != before.Text() {
		return nil, &Refusal{ErrStale}
	}
	edited, err := e.Apply(text)
	if err != nil {
		return nil, &Refusal{err}
	}
	next, err := Parse(edited, l.home, l.protected)
	if err != nil {
		return nil, &Refusal{fmt.Errorf("the changed policy wouldn't load: %w", err)}
	}
	if e.Check != nil {
		if err := e.Check(next); err != nil {
			return nil, &Refusal{err}
		}
	}
	backup, err := WriteFile(l.path, edited, now)
	if err != nil {
		return nil, err
	}
	pruneBackups(l.path, keepBackups)
	change := l.storeLocked(next, backup)
	if e.Record != nil {
		e.Record(change)
	}
	return change, nil
}

// storeLocked puts loaded in force: the watcher won't report it again.
func (l *Live) storeLocked(loaded *Policy, backup string) *Change {
	before := l.current.Load()
	l.lastSeen, l.problem = sha256.Sum256([]byte(loaded.Text())), ""
	l.current.Store(loaded)
	return &Change{Policy: loaded, Before: before, Backup: backup,
		Diff: LineDiff("before", "after", before.Text(), loaded.Text())}
}

// refusedAtStart says how to recover from a policy file that doesn't load
// when Agentry starts: there is no page yet to show what's wrong.
func refusedAtStart(path string, err error) error {
	backups := listBackups(path)
	if len(backups) == 0 {
		return fmt.Errorf("policy %s is refused: %w; there is no backup of it", path, err)
	}
	newest := backups[len(backups)-1]
	return fmt.Errorf("policy %s is refused: %w; to go back to its newest backup, cp '%s' '%s'", path, err, newest, path)
}

// listBackups is the backups of the policy at path, oldest first.
func listBackups(path string) []string {
	entries, _ := os.ReadDir(filepath.Dir(path)) // sorted by name, and the names end in the time
	var backups []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), filepath.Base(path)+"-backup-") {
			backups = append(backups, filepath.Join(filepath.Dir(path), e.Name()))
		}
	}
	return backups
}

// pruneBackups deletes all but the newest keep backups of the policy at
// path. A backup it can't delete stays; the next change tries again.
func pruneBackups(path string, keep int) {
	backups := listBackups(path)
	for _, old := range backups[:max(len(backups)-keep, 0)] {
		os.Remove(old)
	}
}

func (l *Live) refuseLocked(err error, sum [32]byte) error {
	if l.problem == err.Error() {
		return nil // already reported
	}
	l.lastSeen, l.problem = sum, err.Error()
	return err
}

// Watch reloads every interval until ctx ends, and reports each change
// made on disk, before any other change or reload can start.
func (l *Live) Watch(ctx context.Context, interval time.Duration, report func(loaded *Change, refused error)) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.mu.Lock()
			if loaded, refused := l.reloadLocked(); loaded != nil || refused != nil {
				report(loaded, refused)
			}
			l.mu.Unlock()
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
