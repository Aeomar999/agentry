package policy

import "testing"

// # matches a number in a command pattern; \ takes the next character as it is.
func TestACommandPatternCanEscapeItsWildcards(t *testing.T) {
	cases := []struct {
		pattern, command string
		want             bool
	}{
		{"sed -n #p *", "sed -n 12p f", true},
		{"sed -n #p *", "sed -n #p f", false},
		{`echo \#`, "echo #", true},
		{`echo \#`, "echo 1", false},
		{`echo \*`, "echo *", true},
		{`echo \*`, "echo x", false},
		{`echo a\?`, "echo a?", true},
		{`echo a\?`, "echo ab", false},
		{`echo \\`, `echo \`, true},
		{`echo \`, `echo \`, true},
	}
	for _, c := range cases {
		g, err := commandGlob(c.pattern)
		if err != nil {
			t.Fatal(err)
		}
		if got := g.matches(c.command); got != c.want {
			t.Errorf("%q matching %q: got %v, want %v", c.pattern, c.command, got, c.want)
		}
	}
}
