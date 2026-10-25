package policy

import (
	"strings"
	"testing"
)

// A shell command Agentry can't read gets the profile's unreadable, not its
// otherwise, so a profile can refuse what it can't check.
func TestAnUnreadableCommandGetsTheProfilesUnreadable(t *testing.T) {
	const unreadable = "Agentry couldn't read this shell command"
	refusing := strings.Replace(minimal, `otherwise = "ask"`, "otherwise = \"ask\"\nunreadable = \"deny\"", 1)
	asking := strings.Replace(minimal, `otherwise = "ask"`, "otherwise = \"ask\"\nunreadable = \"ask\"", 1)
	cases := []struct {
		name, policy, command string
		want                  Decision
		rule                  string
	}{
		{"unreadable is deny", refusing, "echo 'unterminated", Deny, unreadable},
		{"a readable command no rule matches", refusing, "frobnicate", Ask, "otherwise"},
		{"unreadable is ask", asking, "echo 'unterminated", Ask, unreadable},
	}
	for _, c := range cases {
		p, err := Parse([]byte(c.policy), home, nil)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := p.Decide("p", bash(c.command)); got.Decision != c.want || got.Rule != c.rule {
			t.Errorf("%s: got %s by %q, want %s by %q", c.name, got.Decision, got.Rule, c.want, c.rule)
		}
	}
}
