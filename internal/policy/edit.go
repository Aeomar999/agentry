package policy

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// The edits Agentry makes to a policy file for you. Each takes the file's
// text and returns the new text, touching nothing outside the part it
// changes, and refuses rather than return a policy that doesn't load.

// ErrProfileEdited refuses to update a profile the user changed.
var ErrProfileEdited = errors.New("you changed this profile, so Agentry won't overwrite it")

// AddProfile appends this version's default profile name to the file, under
// a comment saying which version of Agentry added it.
func AddProfile(text []byte, name, version string) ([]byte, error) {
	f, err := decode(text)
	if err != nil {
		return nil, err
	}
	if _, ok := f.Profiles[name]; ok {
		return nil, fmt.Errorf("the file already has a profile called %q", name)
	}
	block, ok := ShippedProfile(name)
	if !ok {
		return nil, fmt.Errorf("Agentry ships no profile called %q", name)
	}
	var added bytes.Buffer
	added.Write(text)
	if len(text) > 0 && !bytes.HasSuffix(text, []byte("\n")) {
		added.WriteByte('\n')
	}
	fmt.Fprintf(&added, "\n\n# Added by Agentry %s, the default profile of that version.\n%s", version, withNewline(block))
	return checkEdit(text, added.Bytes(), name)
}

// UpdateProfile replaces an outdated profile with this version's default.
// It refuses with ErrProfileEdited for a profile the user changed: compare
// them with ProfileDiff and edit the file yourself.
func UpdateProfile(text []byte, name string) ([]byte, error) {
	statuses, err := DefaultProfiles(text)
	if err != nil {
		return nil, err
	}
	state := ProfileMissing
	for _, s := range statuses {
		if s.Name == name {
			state = s.State
		}
	}
	switch state {
	case ProfileMissing:
		return nil, fmt.Errorf("the file has no default profile called %q to update", name)
	case ProfileCurrent:
		return nil, fmt.Errorf("profile %q is already this version's", name)
	case ProfileEdited:
		return nil, fmt.Errorf("profile %q: %w", name, ErrProfileEdited)
	}
	old, ok, err := findBlock(text, name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("profile %q isn't written as a [profiles.%s] table, so Agentry can't replace it", name, name)
	}
	block, _ := ShippedProfile(name)
	updated := slices.Concat(text[:old.start], []byte(withNewline(block)), text[old.end:])
	return checkEdit(text, updated, name)
}

// ProfileDiff shows how the file's profile name differs from this version's
// default, as a unified diff; "" when they are written the same.
func ProfileDiff(text []byte, name string) (string, error) {
	block, ok := ShippedProfile(name)
	if !ok {
		return "", fmt.Errorf("Agentry ships no profile called %q", name)
	}
	theirs, found, err := findBlock(text, name)
	if err != nil {
		return "", err
	}
	yours := ""
	if found {
		yours = withNewline(string(theirs.text(text)))
	}
	return LineDiff("your "+name, "Agentry's "+name, yours, withNewline(block)), nil
}

// TurnOffUseForEveryone comments out the use_for_everyone line, so each
// agent uses its own profile again.
func TurnOffUseForEveryone(text []byte) ([]byte, error) {
	line, err := keyLine(text, "use_for_everyone")
	if err != nil {
		return nil, err
	}
	if line < 0 {
		return nil, errors.New("use_for_everyone isn't set")
	}
	off := slices.Concat(text[:line], []byte("# "), text[line:])
	result, err := checkEdit(text, off, "")
	if err != nil {
		return nil, err
	}
	if f, _ := decode(result); f.UseForEveryone != "" {
		return nil, errors.New("use_for_everyone is written over more than one line; comment it out yourself")
	}
	return result, nil
}

// checkEdit is the edited text if it loads and every profile but changed
// says what it said before.
func checkEdit(before, after []byte, changed string) ([]byte, error) {
	// home only expands ~ in patterns: it can't make a policy load or not.
	if _, err := Parse(after, "", nil); err != nil {
		return nil, fmt.Errorf("the edited policy wouldn't load: %w", err)
	}
	old, _ := decode(before)
	edited, _ := decode(after)
	for name, p := range old.Profiles {
		if name != changed && !reflect.DeepEqual(edited.Profiles[name], p) {
			return nil, fmt.Errorf("the edit would also change profile %q", name)
		}
	}
	return after, nil
}

// withNewline ends a block with a newline even when it was the last line
// of a file that had none.
func withNewline(block string) string {
	if block == "" || strings.HasSuffix(block, "\n") {
		return block
	}
	return block + "\n"
}
