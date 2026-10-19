package policy

import (
	"reflect"
	"strings"
	"testing"
)

func TestProfileBlocksFindEachProfileAsWritten(t *testing.T) {
	text := []byte(`version = 1
default_profile = "a"

[profiles.a]
why = """
[profiles.not-a-header]
"""
otherwise = "ask"
  [[profiles.a.rules]]
  name = "x"
  decision = "allow"
# the end of a

# this introduces b
[profiles.b]
otherwise = "deny"`)
	blocks, err := profileBlocks(text)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, b := range blocks {
		got = append(got, b.name+": "+string(b.text(text)))
	}
	want := []string{
		"a: [profiles.a]\nwhy = \"\"\"\n[profiles.not-a-header]\n\"\"\"\notherwise = \"ask\"\n  [[profiles.a.rules]]\n  name = \"x\"\n  decision = \"allow\"\n# the end of a\n",
		"b: [profiles.b]\notherwise = \"deny\"",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}

	split := []byte("version = 1\n[profiles.a]\notherwise = \"ask\"\n[profiles.b]\notherwise = \"ask\"\n[[profiles.a.rules]]\nname = \"x\"\ndecision = \"allow\"\n")
	if _, err := profileBlocks(split); err == nil || !strings.Contains(err.Error(), "more than one place") {
		t.Errorf("a profile written in two places: %v", err)
	}
}

// Each block of the shipped policy, read alone, is that whole profile.
func TestTheShippedBlocksHoldTheirProfiles(t *testing.T) {
	d, err := shippedProfiles()
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"developer", "read-only", "read-mostly", "autonomous", "allow-everything"}; !reflect.DeepEqual(d.names, want) {
		t.Errorf("profiles %v, want %v", d.names, want)
	}
	for _, name := range d.names {
		alone, err := decode(d.blocks[name])
		if err != nil {
			t.Fatalf("%s alone: %v", name, err)
		}
		if !reflect.DeepEqual(alone.Profiles, map[string]profileFile{name: d.profiles[name]}) {
			t.Errorf("%s's block isn't the whole profile", name)
		}
	}
}
