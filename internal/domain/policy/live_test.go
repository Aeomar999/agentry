package policy

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func openTestLive(t *testing.T) (*Live, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "policy.toml")
	live, err := OpenLive(path, home, nil)
	if err != nil {
		t.Fatal(err)
	}
	return live, path
}

var at = time.Date(2026, 10, 19, 8, 30, 0, 0, time.UTC)

func TestAChangeIsWrittenBackedUpAndPutInForce(t *testing.T) {
	live, path := openTestLive(t)
	before := live.Current()
	var recorded []*Change
	change, err := live.Change(Edit{Seen: before.Hash(), Apply: TurnOffUseForEveryone, Record: func(c *Change) { recorded = append(recorded, c) }}, at)
	if err != nil {
		t.Fatal(err)
	}
	if len(recorded) != 1 || recorded[0] != change {
		t.Errorf("the change should be recorded once: %v", recorded)
	}
	if live.Current() != change.Policy || change.Before != before || change.Policy.UseForEveryone() != "" {
		t.Error("the change should be the policy in force, and say what it replaced")
	}
	if written, _ := os.ReadFile(path); string(written) != change.Policy.Text() {
		t.Error("the file should hold the change")
	}
	if saved, _ := os.ReadFile(change.Backup); string(saved) != before.Text() {
		t.Errorf("the backup %q should hold the file as it was", change.Backup)
	}
	if !strings.Contains(change.Diff, "\n-use_for_everyone = \"allow-everything\"\n+# use_for_everyone = \"allow-everything\"\n") {
		t.Errorf("diff:\n%s", change.Diff)
	}
	if reloaded, refused := live.Reload(); reloaded != nil || refused != nil {
		t.Errorf("the watcher must not report Agentry's own change again as one on disk: %+v %v", reloaded, refused)
	}
}

func TestAChangeToAnOldVersionIsRefused(t *testing.T) {
	live, path := openTestLive(t)
	seen := live.Current().Hash()
	if _, err := live.Change(Edit{Apply: TurnOffUseForEveryone}, at); !errors.Is(err, ErrStale) {
		t.Errorf("no hash: %v", err)
	}

	// Changed on disk, not reloaded yet.
	onDisk := append(Default(), "# mine\n"...)
	os.WriteFile(path, onDisk, 0o600)
	if _, err := live.Change(Edit{Seen: seen, Apply: TurnOffUseForEveryone}, at); !errors.Is(err, ErrStale) {
		t.Errorf("a file changed on disk: %v", err)
	}
	if text, _ := os.ReadFile(path); string(text) != string(onDisk) {
		t.Error("a refused change must leave the file as it was")
	}
	live.Reload()
	if _, err := live.Change(Edit{Seen: seen, Apply: TurnOffUseForEveryone}, at); !errors.Is(err, ErrStale) {
		t.Errorf("a hash from before the reload: %v", err)
	}
	if _, err := live.Change(Edit{Seen: live.Current().Hash(), Apply: TurnOffUseForEveryone}, at); err != nil {
		t.Errorf("the hash in force: %v", err)
	}

	os.WriteFile(path, []byte("version = "), 0o600)
	live.Reload()
	if _, err := live.Change(Edit{Seen: live.Current().Hash(), Apply: TurnOffUseForEveryone}, at); err == nil || !strings.Contains(err.Error(), "doesn't load") {
		t.Errorf("a broken file on disk: %v", err)
	}
}

func TestAChangeItsCheckRefusesIsNotWritten(t *testing.T) {
	live, path := openTestLive(t)
	no := errors.New("no")
	if _, err := live.Change(Edit{Seen: live.Current().Hash(), Apply: TurnOffUseForEveryone, Check: func(*Policy) error { return no }}, at); !errors.Is(err, no) {
		t.Errorf("got %v, want the check's error", err)
	}
	if text, _ := os.ReadFile(path); string(text) != string(Default()) || live.Current().UseForEveryone() == "" {
		t.Error("the file and the policy in force should be as they were")
	}
	if names, _ := filepath.Glob(path + "-*"); len(names) != 0 {
		t.Errorf("no backup for a change not made: %v", names)
	}
}

// A refusal says the change won't be made; failing to write the file isn't one.
func TestOnlyAChangeAgentryWontMakeIsARefusal(t *testing.T) {
	live, path := openTestLive(t)
	var refusal *Refusal
	for name, e := range map[string]Edit{
		"stale":         {Seen: "0ld", Apply: TurnOffUseForEveryone},
		"doesn't apply": {Seen: live.Current().Hash(), Apply: func([]byte) ([]byte, error) { return nil, errors.New("no") }},
		"won't load":    {Seen: live.Current().Hash(), Apply: func([]byte) ([]byte, error) { return []byte("version = "), nil }},
		"checked":       {Seen: live.Current().Hash(), Apply: TurnOffUseForEveryone, Check: func(*Policy) error { return errors.New("no") }},
	} {
		if _, err := live.Change(e, at); !errors.As(err, &refusal) {
			t.Errorf("%s: %v should be a refusal", name, err)
		}
	}
	// A folder where the backup goes can't be written over, by root either.
	os.Mkdir(path+"-backup-"+at.Format(backupTime), 0o700)
	if _, err := live.Change(Edit{Seen: live.Current().Hash(), Apply: TurnOffUseForEveryone}, at); err == nil || errors.As(err, &refusal) {
		t.Errorf("a backup Agentry can't write: %v should be a failure, not a refusal", err)
	}
	if text, _ := os.ReadFile(path); string(text) != string(Default()) || live.Current().UseForEveryone() == "" {
		t.Error("the file and the policy in force should be as they were")
	}
}

// Two changes at once: each lands, or is refused as stale; none is lost
// by being written over.
func TestChangesAtOnceNeverLoseOne(t *testing.T) {
	live, path := openTestLive(t)
	seen := live.Current().Hash()
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for i, note := range []string{"# first\n", "# second\n"} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = live.Change(Edit{Seen: seen, Apply: func(text []byte) ([]byte, error) {
				time.Sleep(10 * time.Millisecond) // both have read the file before either writes
				return append(text, note...), nil
			}}, at.Add(time.Duration(i)))
		}()
	}
	wg.Wait()
	landed := 0
	for _, err := range errs {
		switch {
		case err == nil:
			landed++
		case !errors.Is(err, ErrStale):
			t.Errorf("a change refused for another reason: %v", err)
		}
	}
	text, _ := os.ReadFile(path)
	if landed != 1 || strings.Count(string(text), "# first\n")+strings.Count(string(text), "# second\n") != 1 {
		t.Errorf("%d landed, want exactly one, and the file should hold it:\n%s", landed, text[len(Default())-20:])
	}
}

func TestAgentryKeepsItsNewestBackups(t *testing.T) {
	live, path := openTestLive(t)
	for i := range keepBackups + 5 {
		_, err := live.Change(Edit{Seen: live.Current().Hash(), Apply: func(text []byte) ([]byte, error) {
			return append(text, "# again\n"...), nil
		}}, at.Add(time.Duration(i)*time.Second))
		if err != nil {
			t.Fatal(err)
		}
	}
	backups, _ := filepath.Glob(path + "-backup-*")
	if len(backups) != keepBackups {
		t.Fatalf("%d backups, want %d", len(backups), keepBackups)
	}
	if oldest := path + "-backup-" + at.Add(5*time.Second).Format(backupTime); backups[0] != oldest {
		t.Errorf("oldest kept %s, want %s", backups[0], oldest)
	}
	os.WriteFile(path+"-mine", nil, 0o600)
	pruneBackups(path, 0)
	if names, _ := filepath.Glob(path + "-*"); len(names) != 1 {
		t.Errorf("only backups go: %v", names)
	}
}

// The backup a start-up error names is one Agentry itself wrote, the
// newest: the file from before its last change.
func TestAFileThatDoesntLoadAtStartNamesTheBackupOfAgentrysLastChange(t *testing.T) {
	live, path := openTestLive(t)
	older, _ := live.Change(Edit{Seen: live.Current().Hash(), Apply: TurnOffUseForEveryone}, at)
	newer, _ := live.Change(Edit{Seen: live.Current().Hash(), Apply: func(text []byte) ([]byte, error) {
		return append(text, "# mine\n"...), nil
	}}, at.Add(time.Minute))
	if older == nil || newer == nil || older.Backup == "" || newer.Backup == older.Backup {
		t.Fatalf("two changes, two backups: %+v %+v", older, newer)
	}
	os.WriteFile(path, []byte("version = "), 0o600)
	_, err := OpenLive(path, home, nil)
	if err == nil || !strings.Contains(err.Error(), "cp '"+newer.Backup+"'") || strings.Contains(err.Error(), older.Backup) {
		t.Errorf("names the newest backup, %s: %v", newer.Backup, err)
	}
}
