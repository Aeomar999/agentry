package policy

import (
	"embed"
	"fmt"
	"path"
	"reflect"
	"slices"
	"strings"
)

// shipped/ holds every default.toml whose profiles some version of Agentry
// shipped, oldest first, named NN-VERSION.toml (VERSION-dev for one that only
// ever reached people building from source). They tell a profile the user
// never touched, which can be updated, from one they changed, which can't.
//
// When a change to default.toml changes a profile, copy the new
// default.toml to the next NN with the version that will ship it:
//
//	cp internal/policy/default.toml internal/policy/shipped/05-v0.9.0.toml
//
// TestEveryShippedProfileIsKept fails until you do.
//
//go:embed shipped
var shippedFiles embed.FS

// A snapshot is the profiles one version of Agentry shipped.
type snapshot struct {
	version  string
	profiles map[string]profileFile
	comments map[string][]string // each profile's comments, as blockComments finds them
}

// snapshots reads shipped/, oldest first.
func snapshots() ([]snapshot, error) {
	entries, err := shippedFiles.ReadDir("shipped")
	if err != nil {
		return nil, err
	}
	var all []snapshot
	for _, entry := range entries { // ReadDir sorts by name, so by NN
		_, version, ok := strings.Cut(strings.TrimSuffix(entry.Name(), ".toml"), "-")
		if !ok {
			return nil, fmt.Errorf("shipped/%s isn't named NN-VERSION.toml", entry.Name())
		}
		text, err := shippedFiles.ReadFile(path.Join("shipped", entry.Name()))
		if err != nil {
			return nil, err
		}
		f, err := decode(text)
		if err != nil {
			return nil, fmt.Errorf("shipped/%s: %w", entry.Name(), err)
		}
		all = append(all, snapshot{version: version, profiles: f.Profiles, comments: blockComments(text)})
	}
	return all, nil
}

// ProfileState is how a profile of the user's file stands against the
// profile of the same name this version of Agentry ships.
type ProfileState string

const (
	ProfileMissing  ProfileState = "missing"  // the file has no such profile
	ProfileCurrent  ProfileState = "current"  // the same as this version's
	ProfileOutdated ProfileState = "outdated" // as an older version shipped it, untouched since: it can be updated
	ProfileEdited   ProfileState = "edited"   // the user changed it: Agentry never overwrites it
)

// DefaultProfileStatus is one default profile as the user's file has it.
type DefaultProfileStatus struct {
	Name  string       `json:"name"`
	State ProfileState `json:"state"`
	// ShippedIn is the newest version that shipped the user's profile
	// as it is, when it is outdated.
	ShippedIn string `json:"shippedIn,omitempty"`
}

// DefaultProfiles says, for each profile this version ships, in
// default.toml's order, how the policy file text has it. Profiles are
// compared by what they say, not how: layout doesn't count. Comments count
// only against an older version: a profile that says what this version's
// does is current whatever notes it has, but one that says what an older
// version's did is outdated only if its comments are that version's too,
// since updating it would drop a note of the user's.
func DefaultProfiles(text []byte) ([]DefaultProfileStatus, error) {
	f, err := decode(text)
	if err != nil {
		return nil, err
	}
	past, err := snapshots()
	if err != nil {
		return nil, err
	}
	shipped, err := shippedProfiles()
	if err != nil {
		return nil, err
	}
	yours := snapshot{profiles: f.Profiles, comments: blockComments(text)}
	var statuses []DefaultProfileStatus
	for _, name := range shipped.names {
		statuses = append(statuses, profileStatus(name, yours, shipped.profiles[name], past))
	}
	return statuses, nil
}

func profileStatus(name string, file snapshot, current profileFile, past []snapshot) DefaultProfileStatus {
	status := DefaultProfileStatus{Name: name, State: ProfileEdited}
	theirs, ok := file.profiles[name]
	switch {
	case !ok:
		status.State = ProfileMissing
	case reflect.DeepEqual(theirs, current):
		status.State = ProfileCurrent
	default:
		for _, s := range past {
			old, ok := s.profiles[name]
			if ok && reflect.DeepEqual(theirs, old) && slices.Equal(file.comments[name], s.comments[name]) {
				status.State, status.ShippedIn = ProfileOutdated, s.version
			}
		}
	}
	return status
}

// defaults are the profiles this version ships, from default.toml.
type defaults struct {
	names    []string // in default.toml's order
	profiles map[string]profileFile
	blocks   map[string][]byte // each profile as default.toml writes it
}

func shippedProfiles() (defaults, error) {
	f, err := decode(shippedPolicy)
	if err != nil {
		return defaults{}, err
	}
	blocks, err := profileBlocks(shippedPolicy)
	if err != nil {
		return defaults{}, err
	}
	d := defaults{profiles: f.Profiles, blocks: map[string][]byte{}}
	for _, b := range blocks {
		d.names = append(d.names, b.name)
		d.blocks[b.name] = b.text(shippedPolicy)
	}
	return d, nil
}

// ShippedProfile is the default profile name as this version writes it in
// default.toml, or false when it ships no such profile.
func ShippedProfile(name string) (string, bool) {
	d, err := shippedProfiles()
	if err != nil {
		return "", false
	}
	block, ok := d.blocks[name]
	return string(block), ok
}
