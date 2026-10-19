package policy

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
)

func TestWriteFileKeepsABackupFirst(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "policy.toml")
	now := time.Date(2026, 10, 19, 8, 30, 0, 123, time.UTC)

	backup, err := WriteFile(path, Default(), now)
	if err != nil || backup != "" {
		t.Fatalf("a first write has nothing to back up: %q, %v", backup, err)
	}
	off, _ := TurnOffUseForEveryone(Default())
	backup, err = WriteFile(path, off, now)
	if err != nil {
		t.Fatal(err)
	}
	if want := path + "-backup-20261019T083000.000000123Z"; backup != want {
		t.Errorf("backup %s, want %s", backup, want)
	}
	if saved, _ := os.ReadFile(backup); string(saved) != string(Default()) {
		t.Error("the backup should hold the file as it was")
	}
	if written, _ := os.ReadFile(path); string(written) != string(off) {
		t.Error("the file should hold the new text")
	}
	if _, err := WriteFile(path, off, now); err == nil {
		t.Error("a second backup at the same time must not overwrite the first")
	}

	if _, err := WriteFile(path, []byte("version = "), now.Add(time.Second)); err == nil {
		t.Error("a policy that doesn't load must not be written")
	}
	if written, _ := os.ReadFile(path); string(written) != string(off) {
		t.Error("a refused write must leave the file as it was")
	}
	names, _ := filepath.Glob(filepath.Join(dir, "*"))
	if want := []string{path, backup}; !slices.Equal(names, want) {
		t.Errorf("files %v, want only %v: no temporary file or extra backup left", names, want)
	}

	// The backup is Agentry's own file, as the policy is.
	p, _ := Parse(off, home, []string{path})
	if v := p.Decide("developer", file("Edit", backup)); v.Decision != Deny || v.Rule != "Agentry's own files are off limits" {
		t.Errorf("editing the backup: %s (%s), want it denied as Agentry's own file", v.Decision, v.Rule)
	}
}
