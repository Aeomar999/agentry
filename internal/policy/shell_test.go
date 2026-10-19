package policy

import (
	"slices"
	"strings"
	"testing"

	"mvdan.cc/sh/v3/syntax"
)

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

// A word's values are what bash hands the command, checked against bash with
// `bash -c 'set -- <word>; printf "[%s] " "$@"'` (task-89, review round 3).
func TestWordsAreReadAsBashReadsThem(t *testing.T) {
	cases := []struct {
		word string
		want []string // nil: unknown
	}{
		{`a\ b`, []string{"a b"}},
		{`"a\$b"`, []string{"a$b"}},
		{`"a\x"`, []string{`a\x`}},
		{`\\`, []string{`\`}},
		{`"\\"`, []string{`\`}},
		{"a\\\nb", []string{"ab"}},
		{`'a\b'`, []string{`a\b`}},
		{`$'a\x41'`, nil},
		{`{a,b}"x"`, []string{"ax", "bx"}},
		{`{a,'b,c'}`, []string{"a", "b,c"}},
		{`{x{a,b}}`, []string{"{xa}", "{xb}"}},
		{`{a,b}{c,d}`, []string{"ac", "ad", "bc", "bd"}},
		{`{a,{b,c}d}`, []string{"a", "bd", "cd"}},
		{`{{a,b}`, []string{"{a", "{b"}},
		{`{a,}`, []string{"a"}},
		{`{,}`, []string{}},
		{`\{a,b}`, []string{"{a,b}"}},
		{`{a\,b}`, []string{"{a,b}"}},
		{`"{a,b}"`, []string{"{a,b}"}},
		{`{a}`, []string{"{a}"}},
		{`a{b,c`, []string{"a{b,c"}},
		{`x{1..3}`, nil},
		{`$x{y,}`, nil},
	}
	for _, c := range cases {
		file, err := syntax.NewParser().Parse(strings.NewReader("echo "+c.word), "")
		if err != nil {
			t.Fatal(err)
		}
		call := file.Stmts[0].Cmd.(*syntax.CallExpr)
		words, ok := braceWords(call.Args[1])
		got := []string{}
		for _, word := range words {
			values, known := (&shellInspector{}).values(word)
			ok = ok && known
			got = append(got, values...)
		}
		if !ok {
			if c.want != nil {
				t.Errorf("%s: unknown, want %q", c.word, c.want)
			}
			continue
		}
		if c.want == nil || !slices.Equal(got, c.want) {
			t.Errorf("%s: %q, want %q", c.word, got, c.want)
		}
	}
}
