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
		{"/t\\\nmp", []string{"/tmp"}},
		{"$\\\nx", nil},
		{"\"$\\\nx\"", nil},
		{"a$\\\nb", nil},
		{"$\\\nx{a,b}", nil},
		{`\$x`, []string{"$x"}},
		{`a$`, []string{"a$"}},
		{`a$/b`, []string{"a$/b"}}, // a $ that starts nothing is text
		{`"^$|x"`, []string{"^$|x"}},
		{`"%h$%s"`, []string{"%h$%s"}},
		{`"a$ b"`, []string{"a$ b"}},
		{`"a$"x`, []string{"a$x"}},
		{`a$$b`, nil},
		{`$'a'b`, []string{"ab"}},
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
		{`{a}b,c}`, []string{"a}b", "c"}},
		{`{a}{b,c}`, []string{"{a}b", "{a}c"}},
		{`{a}b,{c}`, []string{"{a}b,{c}"}},
		{`{},}1`, []string{"{},}1"}},
		{`x{},}1`, []string{"x}1", "x1"}},
		{`x{1..3}`, []string{"x1", "x2", "x3"}},
		{`{3..1}`, []string{"3", "2", "1"}},
		{`{a..c}`, []string{"a", "b", "c"}},
		{`{C..A}`, []string{"C", "B", "A"}},
		{`{1..10..4}`, []string{"1", "5", "9"}},
		{`{10..1..-4}`, []string{"10", "6", "2"}},
		{`{-2..1}`, []string{"-2", "-1", "0", "1"}},
		{`{01..03}`, []string{"01", "02", "03"}},
		{`{1..010..4}`, []string{"001", "005", "009"}},
		{`{5..5}`, []string{"5"}},
		{`{1..2}{a,b}`, []string{"1a", "1b", "2a", "2b"}},
		{`{1..65}`, nil},    // more than maxValues words
		{`{-01..2}`, nil},   // bash pads a negative number as -01 000
		{`{a..Z}`, nil},     // letters of mixed case
		{`{+1..3}`, nil},    // a sign
		{`{1..a}`, nil},     // bash leaves it as it is
		{`{1..{2,3}}`, nil}, // bash makes {1..2} {1..3}
		{`$x{y,}`, nil},
		{"{" + strings.Repeat("{a,b}", maxBraces) + "}", nil}, // too many braces
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

// Commands are judged as the shell runs them, quotes and backslashes gone.
func TestCommandsAreReadUnquoted(t *testing.T) {
	cases := map[string]string{
		`\tar c x`:         "tar c x",
		`t\ar c x`:         "tar c x",
		`'tar' c x`:        "tar c x",
		`"t"ar c x`:        "tar c x",
		`echo a\ b`:        "echo a b",
		`echo "a\nb\$c\\"`: `echo a\nb$c\`,
		`echo 'a\nb'`:      `echo a\nb`,
		"echo a\\\nb":      "echo ab",
		`echo $'\x74ar'`:   "echo $?",
		`echo $'tar'`:      "echo tar",
		`grep -E 'a\|b' x`: `grep -E a\|b x`,
		`grep -E a\\\|b x`: `grep -E a\|b x`,
	}
	for command, want := range cases {
		if got := inspectShell(command, "/work", "/home").commands; len(got) != 1 || got[0] != want {
			t.Errorf("%s: commands %q, want %q", command, got, want)
		}
	}
}

// A command handed to a shell as a string is unknown when the string is, or
// when it runs too deep inside others.
func TestInnerCommandsThatCantBeKnown(t *testing.T) {
	cases := []struct {
		command string
		opaque  bool
	}{
		{"bash -c 'echo hi'", false},
		{`bash -c "$x"`, true},
		{"sh -c 'echo $(id)'", true},
		{"eval eval eval eval echo hi", false},
		{"eval eval eval eval eval echo hi", true},
		// folders of the shell's stack
		{"cat ~-/x", true},
		{"ls ~-", true},
		{"pushd ~1", true},
		{"cat ~+2/x", true},
		{"cat ~+/x", false},
	}
	for _, c := range cases {
		if got := inspectShell(c.command, "/work", "/home").opaque; got != c.opaque {
			t.Errorf("%s: opaque %v, want %v", c.command, got, c.opaque)
		}
	}
}

// A command run as a string starts in the folders the shell may be in, and
// eval runs in this shell, so its cd moves the commands after it.
func TestInnerCommandsStartWhereTheShellIs(t *testing.T) {
	cases := []struct {
		command string
		want    string // a path the call names
	}{
		{"cd /a && bash -c 'cat x'", "/a/x"},
		{`eval "cd /a" && cat x`, "/a/x"},
		{`command eval "cd /a"; cat x`, "/a/x"},
		{"cat ~+/x", "/work/x"},
		{"cd /a; cat ~+/x", "/a/x"},
	}
	for _, c := range cases {
		if got := inspectShell(c.command, "/work", "/home").paths; !slices.Contains(got, c.want) {
			t.Errorf("%s: paths %q, want %s among them", c.command, got, c.want)
		}
	}
	if got := inspectShell("bash -c 'cd /a'; cat x", "/work", "/home").paths; slices.Contains(got, "/a/x") {
		t.Errorf("bash -c's cd moved the shell: paths %q", got)
	}
}
