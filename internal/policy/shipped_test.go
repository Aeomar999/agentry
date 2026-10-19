package policy

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"reflect"
	"strings"
	"testing"
)

func shippedFile(t *testing.T, name string) []byte {
	t.Helper()
	text, err := shippedFiles.ReadFile(path.Join("shipped", name))
	if err != nil {
		t.Fatal(err)
	}
	return text
}

// A profile changed in default.toml must be kept in shipped/ too, or the
// installs that still have the old one would see it as edited, never updated.
func TestEveryShippedProfileIsKept(t *testing.T) {
	all, err := snapshots()
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := shippedFiles.ReadDir("shipped")
	newest := entries[len(entries)-1].Name()
	current, _ := decode(Default())
	if !reflect.DeepEqual(all[len(all)-1].profiles, current.Profiles) {
		t.Errorf("default.toml's profiles aren't the ones in shipped/%s: copy it to the next one, "+
			"as in cp internal/policy/default.toml internal/policy/shipped/NN-vX.Y.Z.toml, with the version that ships it", newest)
	}
	for i, entry := range entries {
		if want := fmt.Sprintf("%02d-", i+1); !strings.HasPrefix(entry.Name(), want) {
			t.Errorf("shipped/%s should start with %q", entry.Name(), want)
		}
	}
}

func TestDefaultProfilesSaysWhatEachFileHas(t *testing.T) {
	statesOf := func(text []byte) map[string]string {
		t.Helper()
		statuses, err := DefaultProfiles(text)
		if err != nil {
			t.Fatal(err)
		}
		states := map[string]string{}
		for _, s := range statuses {
			states[s.Name] = string(s.State) + " " + s.ShippedIn
		}
		return states
	}
	firstRelease := shippedFile(t, "02-v0.1.0.toml")
	beforeIt := shippedFile(t, "01-v0.1.0-dev.toml")
	cases := map[string]struct {
		text []byte
		want map[string]string
	}{
		"this version's": {Default(), map[string]string{"developer": "current ", "read-only": "current ",
			"read-mostly": "current ", "autonomous": "current ", "allow-everything": "current "}},
		"v0.1.0, before read-mostly": {firstRelease, map[string]string{"developer": "current ", "read-only": "current ",
			"read-mostly": "missing ", "autonomous": "current ", "allow-everything": "current "}},
		"before v0.1.0": {beforeIt, map[string]string{"developer": "current ", "read-only": "outdated v0.1.0-dev",
			"read-mostly": "missing ", "autonomous": "current ", "allow-everything": "current "}},
		"a why changed": {replaceOnce(t, beforeIt, `why = "For agents that judge work`, `why = "Mine: for agents that judge work`),
			map[string]string{"developer": "current ", "read-only": "edited ",
				"read-mostly": "missing ", "autonomous": "current ", "allow-everything": "current "}},
		"only comments and spacing changed": {replaceOnce(t, Default(), "[profiles.read-only]\n", "[profiles.read-only]   # mine\n\n# my notes\n"),
			map[string]string{"developer": "current ", "read-only": "current ",
				"read-mostly": "current ", "autonomous": "current ", "allow-everything": "current "}},
	}
	for name, c := range cases {
		if got := statesOf(c.text); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %v, want %v", name, got, c.want)
		}
	}
}

func replaceOnce(t *testing.T, text []byte, old, new string) []byte {
	t.Helper()
	if n := bytes.Count(text, []byte(old)); n != 1 {
		t.Fatalf("%q is in the text %d times, want once", old, n)
	}
	return bytes.Replace(text, []byte(old), []byte(new), 1)
}

func TestAddProfileAppendsTheDefaultAndLeavesTheRest(t *testing.T) {
	text := shippedFile(t, "02-v0.1.0.toml")
	added, err := AddProfile(text, "read-mostly", "v0.8.2")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(added, text) {
		t.Error("the file before the new profile should be as it was, byte for byte")
	}
	block, _ := ShippedProfile("read-mostly")
	if want := "\n\n# Added by Agentry v0.8.2, the default profile of that version.\n" + block; string(added[len(text):]) != want {
		t.Errorf("appended:\n%s\nwant:\n%s", added[len(text):], want)
	}
	if state := stateOf(t, added, "read-mostly"); state != ProfileCurrent {
		t.Errorf("read-mostly is %s after adding it", state)
	}
	if _, err := AddProfile(added, "read-mostly", "v0.8.2"); err == nil || !strings.Contains(err.Error(), "already has") {
		t.Errorf("adding it twice: %v", err)
	}
	if _, err := AddProfile(text, "dreamer", "v0.8.2"); err == nil || !strings.Contains(err.Error(), "ships no profile") {
		t.Errorf("adding a profile Agentry doesn't ship: %v", err)
	}
	noNewline := bytes.TrimSuffix(text, []byte("\n"))
	if added, err := AddProfile(noNewline, "read-mostly", "v0.8.2"); err != nil || !bytes.HasPrefix(added, append(noNewline, '\n')) {
		t.Errorf("a file without a last newline: %v", err)
	}
}

func stateOf(t *testing.T, text []byte, name string) ProfileState {
	t.Helper()
	statuses, err := DefaultProfiles(text)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range statuses {
		if s.Name == name {
			return s.State
		}
	}
	t.Fatalf("no status for %s", name)
	return ""
}

func TestUpdateProfileReplacesOnlyAnUntouchedProfile(t *testing.T) {
	// The user's own notes, above the profile and in the next one, and a
	// profile of their own must all survive the update.
	text := replaceOnce(t, shippedFile(t, "01-v0.1.0-dev.toml"), "\n[profiles.read-only]\n", "\n# my note on read-only\n[profiles.read-only]\n")
	text = replaceOnce(t, text, "\n[profiles.autonomous]\n", "\n# about autonomous\n[profiles.autonomous]\n# inside it\n")
	text = append(text, "\n[profiles.mine]\notherwise = \"deny\" # strict\n"...)

	updated, err := UpdateProfile(text, "read-only")
	if err != nil {
		t.Fatal(err)
	}
	old, _, _ := findBlock(text, "read-only")
	block, _ := ShippedProfile("read-only")
	want := string(text[:old.start]) + block + string(text[old.end:])
	if string(updated) != want {
		t.Errorf("only the read-only block should change:\n%s", LineDiff("want", "got", want, string(updated)))
	}
	for _, kept := range []string{"# my note on read-only\n[profiles.read-only]", "# about autonomous\n[profiles.autonomous]\n# inside it\n", "otherwise = \"deny\" # strict"} {
		if !bytes.Contains(updated, []byte(kept)) {
			t.Errorf("lost %q", kept)
		}
	}
	if state := stateOf(t, updated, "read-only"); state != ProfileCurrent {
		t.Errorf("read-only is %s after the update", state)
	}

	edited := replaceOnce(t, text, `why = "For agents that judge work`, `why = "Mine: for agents that judge work`)
	if _, err := UpdateProfile(edited, "read-only"); !errors.Is(err, ErrProfileEdited) {
		t.Errorf("updating an edited profile: got %v, want ErrProfileEdited", err)
	}
	if _, err := UpdateProfile(updated, "read-only"); err == nil || !strings.Contains(err.Error(), "already") {
		t.Errorf("updating a current profile: %v", err)
	}
	if _, err := UpdateProfile(text, "read-mostly"); err == nil || !strings.Contains(err.Error(), "no default profile") {
		t.Errorf("updating a missing profile: %v", err)
	}
}

func TestProfileDiffShowsYoursAgainstTheDefault(t *testing.T) {
	text := shippedFile(t, "01-v0.1.0-dev.toml")
	diff, err := ProfileDiff(text, "read-only")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"--- your read-only\n+++ Agentry's read-only\n@@ ", "\n+  name = \"keeps its own notes\"\n"} {
		if !strings.Contains(diff, want) {
			t.Errorf("the diff lacks %q:\n%s", want, diff)
		}
	}
	if diff, err := ProfileDiff(Default(), "read-only"); diff != "" || err != nil {
		t.Errorf("a current profile: got %q, %v", diff, err)
	}
	missing, err := ProfileDiff(text, "read-mostly")
	if err != nil || !strings.Contains(missing, "\n+[profiles.read-mostly]\n") || strings.Contains(missing, "\n-") {
		t.Errorf("a missing profile should show all added: %v\n%s", err, missing)
	}
}

func TestTurnOffUseForEveryoneCommentsOutItsLine(t *testing.T) {
	off, err := TurnOffUseForEveryone(Default())
	if err != nil {
		t.Fatal(err)
	}
	if want := replaceOnce(t, Default(), override, "\n# use_for_everyone = \"allow-everything\"\n"); !bytes.Equal(off, want) {
		t.Errorf("only that line should change:\n%s", LineDiff("want", "got", string(want), string(off)))
	}
	p, err := Parse(off, home, protected)
	if err != nil || p.UseForEveryone() != "" {
		t.Errorf("turned off: %v, use_for_everyone %q", err, p.UseForEveryone())
	}
	if _, err := TurnOffUseForEveryone(off); err == nil || !strings.Contains(err.Error(), "isn't set") {
		t.Errorf("turning it off twice: %v", err)
	}
}
