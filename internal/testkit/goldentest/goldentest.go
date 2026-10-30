// Package goldentest checks a table a test builds against the copy
// committed in testdata, so a change to it shows as a diff to review.
package goldentest

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Check fails t when the file at path doesn't hold got, listing the lines
// it lacks (+) and those it has that got doesn't (-). With update it
// writes got there first. rerun is the command that rewrites the file,
// which the failure names.
func Check(t testing.TB, path, got string, update bool, rerun string) {
	t.Helper()
	if update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v; run `%s` to write it", err, rerun)
	}
	if string(want) == got {
		return
	}
	t.Errorf("%s differs from what the code gives now:\n%s\nIf that's meant, run `%s` and read the diff of %s.",
		path, Diff(string(want), got), rerun, path)
}

// Diff is the lines of got that want lacks (+) and the lines of want that
// got lacks (-), each in its own order. Files with the same lines in
// another order differ by none, which it says.
func Diff(want, got string) string {
	wantLines, gotLines := strings.Split(want, "\n"), strings.Split(got, "\n")
	var diff strings.Builder
	for _, line := range wantLines {
		if !slices.Contains(gotLines, line) {
			diff.WriteString("- " + line + "\n")
		}
	}
	for _, line := range gotLines {
		if !slices.Contains(wantLines, line) {
			diff.WriteString("+ " + line + "\n")
		}
	}
	if diff.Len() == 0 && want != got {
		return "(only the order of lines differs)\n"
	}
	return diff.String()
}
