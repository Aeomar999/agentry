package goldentest

import "testing"

func TestDiffSaysWhatChanged(t *testing.T) {
	for _, c := range []struct{ want, got, diff string }{
		{"a\nb\n", "a\nc\n", "- b\n+ c\n"},
		{"a\nb\n", "b\na\n", "(only the order of lines differs)\n"},
		{"a\n", "a\n", ""},
	} {
		if diff := Diff(c.want, c.got); diff != c.diff {
			t.Errorf("Diff(%q, %q) = %q, want %q", c.want, c.got, diff, c.diff)
		}
	}
}
