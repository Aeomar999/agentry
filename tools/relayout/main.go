// Command relayout makes the package moves of steps 4 to 9 of
// docs/design/repo-organization.md, section 7, from section 4's table,
// which table.go holds:
//
//	go run ./tools/relayout [-n] <step> [area]
//
// For each package of the step it git mvs the folder, makes the edits the
// step needs beyond its paths (relative paths whose depth changes), and
// rewrites the old path, by whole path segments, in every Go file, spec/,
// the build files and the UI's sources; then gofmt -w sorts the imports
// again. It first checks each moved package for a relative path or Chdir
// it neither edits nor allows, and stops on one, before changing anything.
//
// Running a step again is safe: on a moved tree it changes nothing, and on
// a branch rebased over the move it moves the files still in an old folder
// and rewrites the old paths they or new lines brought.
package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"strings"
)

const usage = `usage: go run ./tools/relayout [-n] <step> [area]

Makes one step of the package moves in docs/design/repo-organization.md,
section 7, in the repo it runs in. Run it from the repo root.

  4   testkit and tools: storetest, deeptest, speccheck, packaging
  5   platform
  6   core
  7   domain, one area at a time: %s
      (7 alone moves every area)
  8   extensions/ideas, extensions/schedules
  9   runtime

  -n  say what would change, and change nothing

Running a step again changes only what is still at its old path.
`

func main() {
	flag.Usage = func() { fmt.Fprintf(os.Stderr, usage, strings.Join(areas, " ")) }
	dry := flag.Bool("n", false, "say what would change, and change nothing")
	flag.Parse()
	step, area, err := parseStep(flag.Args())
	if err != nil {
		fmt.Fprintln(os.Stderr, "relayout:", err)
		flag.Usage()
		os.Exit(2)
	}
	root, err := git(".", "rev-parse", "--show-toplevel")
	if err != nil {
		fmt.Fprintln(os.Stderr, "relayout:", err)
		os.Exit(1)
	}
	r := run{
		root:    strings.TrimSpace(root),
		moves:   movesOf(step, area),
		edits:   editsOf(step, area),
		allowed: allowed,
		dry:     *dry,
		out:     os.Stdout,
	}
	if err := r.do(); err != nil {
		fmt.Fprintln(os.Stderr, "relayout:", err)
		os.Exit(1)
	}
}

// parseStep reads the step and, for step 7, the optional area.
func parseStep(args []string) (step, area string, err error) {
	switch {
	case len(args) == 0 || len(args) > 2:
		return "", "", fmt.Errorf("give a step, and for step 7 an area if you like")
	case len(movesOf(args[0], "")) == 0:
		return "", "", fmt.Errorf("no step %q: the steps are 4 to 9", args[0])
	case len(args) == 1:
		return args[0], "", nil
	case args[0] != "7":
		return "", "", fmt.Errorf("only step 7 has areas")
	case !slices.Contains(areas, args[1]):
		return "", "", fmt.Errorf("no area %q in step 7", args[1])
	}
	return args[0], args[1], nil
}
