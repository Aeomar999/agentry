package policy

import "testing"

// A variable set in a way that can't be read leaves the command using it
// unknown. No profile both allows wait and asks on an unknown word, so these
// are checked here rather than through a decision.
func TestVariablesThatCantBeKnown(t *testing.T) {
	cases := []struct {
		command string
		opaque  bool
	}{
		{"c=status; wait -p c; git $c", true},
		{"c=status; wait -n -p c; git $c", true},
		{"c=status; wait -np c; git $c", true},
		{"c=status; wait -n; git $c", false},
		// its values never stop growing
		{"c=status; c=${c}x; git $c", true},
		{"a=log; c=${a}; git $c", false},
	}
	for _, c := range cases {
		if got := inspectShell(c.command, "/work", "/home").opaque; got != c.opaque {
			t.Errorf("%s: opaque %v, want %v", c.command, got, c.opaque)
		}
	}
}
