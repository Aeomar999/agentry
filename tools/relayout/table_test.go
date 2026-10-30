package main

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// Running the table again over its own new paths changes none of them,
// so a second run of any step rewrites nothing it already rewrote.
func TestNewPathsAreNotRewrittenAgain(t *testing.T) {
	for _, m := range moves {
		for _, text := range []string{m.new, "./" + m.new + "/...", module + "/" + m.new} {
			if got := rewriteText(text, moves); got != text {
				t.Errorf("%s is rewritten again, to %s", text, got)
			}
		}
	}
}

func TestEveryStepAndAreaHasMoves(t *testing.T) {
	for _, step := range []string{"4", "5", "6", "7", "8", "9"} {
		if len(movesOf(step, "")) == 0 {
			t.Errorf("step %s moves nothing", step)
		}
	}
	for _, area := range areas {
		if len(movesOf("7", area)) == 0 {
			t.Errorf("step 7's area %s moves nothing", area)
		}
	}
	for _, m := range moves {
		if (m.step == "7") != slices.Contains(areas, m.area) {
			t.Errorf("%s: step %s with area %q; only step 7's rows, and all of them, name an area", m.old, m.step, m.area)
		}
	}
}

// Each package moves once, and never into its own folder, which would
// read as a stray of itself.
func TestEachPackageMovesOnceAndOutOfItsFolder(t *testing.T) {
	seen := map[string]bool{}
	for _, m := range moves {
		if seen[m.old] {
			t.Errorf("%s moves twice", m.old)
		}
		seen[m.old] = true
		if strings.HasPrefix(m.new+"/", m.old+"/") {
			t.Errorf("%s moves into itself, to %s", m.old, m.new)
		}
	}
}

// An edit's new text holds neither its old text nor an old path, so a
// second run finds the edit done.
func TestEditsAreDoneOnce(t *testing.T) {
	for _, e := range edits {
		if strings.Contains(e.new, e.old) {
			t.Errorf("%s: %q holds %q, so a second run would edit it again", e.file, e.new, e.old)
		}
		if got := rewriteText(e.new, moves); got != e.new {
			t.Errorf("%s: the rewrite changes the edit's new text %q to %q", e.file, e.new, got)
		}
	}
}

// The files edits and allowed name are in the repo, today or where their
// step moved them, so the lists don't fall behind the code.
func TestListedFilesAreInTheRepo(t *testing.T) {
	var files []string
	for _, e := range edits {
		files = append(files, e.file)
	}
	for file := range allowed {
		files = append(files, file)
	}
	for _, file := range files {
		if !exists(filepath.Join("../..", file)) && !exists(filepath.Join("../..", rewriteText(file, moves))) {
			t.Errorf("%s is listed in table.go, but it is in the repo neither there nor at its new path", file)
		}
	}
}

func TestEditsAreInStepsThatMove(t *testing.T) {
	for _, e := range edits {
		if len(movesOf(e.step, e.area)) == 0 {
			t.Errorf("%s: an edit in step %s %s, which moves nothing", e.file, e.step, e.area)
		}
	}
}

// The repo's packages still to move have no relative path or Chdir that
// table.go neither edits nor allows, so a new one fails the tests the day
// it lands, not the day its step runs. It only reads: a package already
// moved is checked at its new folder.
func TestTheRepoHasNoUnknownRelativePaths(t *testing.T) {
	var runs []run
	for _, step := range []string{"4", "5", "6", "7", "8", "9"} {
		runs = append(runs, run{moves: movesOf(step, ""), edits: editsOf(step, "")})
	}
	for _, area := range areas {
		runs = append(runs, run{moves: movesOf("7", area), edits: editsOf("7", area)})
	}
	for _, r := range runs {
		r.root, r.allowed = "../..", allowed
		if err := r.checkRelativePaths(); err != nil {
			t.Errorf("step %s: %v", strings.TrimSpace(r.moves[0].step+" "+r.moves[0].area), err)
		}
	}
}
