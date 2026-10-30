package policy

import "testing"

func TestLineDiff(t *testing.T) {
	numbers := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\n15\n16\n"
	cases := map[string]struct{ old, new, want string }{
		"the same": {numbers, numbers, ""},
		"one line changed": {"a\nb\nc\n", "a\nB\nc\n",
			"--- old\n+++ new\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n"},
		"added at the start, removed at the end": {"1\n2\n3\n4\n5\n", "0\n1\n2\n3\n4\n",
			"--- old\n+++ new\n@@ -1,5 +1,5 @@\n+0\n 1\n 2\n 3\n 4\n-5\n"},
		"far apart changes make two hunks": {numbers, "1\nTWO\n3\n4\n5\n6\n7\n8\n9\n10\n11\n12\n13\n14\nFIFTEEN\n16\n",
			"--- old\n+++ new\n@@ -1,5 +1,5 @@\n 1\n-2\n+TWO\n 3\n 4\n 5\n@@ -12,5 +12,5 @@\n 12\n 13\n 14\n-15\n+FIFTEEN\n 16\n"},
		"into an empty file": {"", "a\n", "--- old\n+++ new\n@@ -0,0 +1 @@\n+a\n"},
		"a last line without a newline": {"a\nb\n", "a\nb",
			"--- old\n+++ new\n@@ -1,2 +1,2 @@\n a\n-b\n+b\n\\ No newline at end of file\n"},
	}
	for name, c := range cases {
		if got := LineDiff("old", "new", c.old, c.new); got != c.want {
			t.Errorf("%s: got\n%s\nwant\n%s", name, got, c.want)
		}
	}
}
