// Command speccheck checks the spec folder, spec/ by default, so that every
// scenario in it can be found, counted and judged.
//
// It walks every .feature file under the folder and reports a scenario that
//
//   - lacks an ID tag with the prefix its file name gives it (@USER-01 in
//     user.feature), or sits in a file whose name is not in the prefix table;
//   - lacks exactly one of @test, @benchmark and @manual;
//   - repeats an ID used anywhere else in the folder;
//   - has a Then line, or an And/But line continuing a Then, with no digit
//     and no outcome word;
//   - uses the words quickly, properly or appropriately;
//   - is tagged @test and cites, in a comment line, a repo path that does
//     not exist, or a Go test that is not defined in the test file cited
//     with it;
//   - is numbered above its prefix's entry in lastIDBeforeLineRule and
//     cites "line N" or "lines N".
//
// Cited paths are read from the folder's parent, the repo root. A Scenario
// Outline is reported and not counted: this spec writes one Scenario per
// case. The checker also reports an em-dash anywhere in any file under the
// folder. Problems are printed as file:line: message; the scenarios per
// file, the totals per check tag and how many cited paths and Go tests were
// checked are printed either way; the exit code is 1 on any problem.
package main

import (
	"fmt"
	"io"
	"os"
)

func main() {
	folder := "spec"
	if len(os.Args) > 1 {
		folder = os.Args[1]
	}
	result, err := Check(folder)
	if err != nil {
		fmt.Fprintln(os.Stderr, "spec/check:", err)
		os.Exit(2)
	}
	printCounts(os.Stdout, result)
	if len(result.Problems) > 0 {
		printProblems(os.Stderr, result.Problems)
		os.Exit(1)
	}
}

// printCounts writes the scenarios per feature file and the totals per
// check tag, the part of the output a passing run shows too.
func printCounts(w io.Writer, result Result) {
	scenarios := 0
	for _, file := range result.Files {
		fmt.Fprintf(w, "%s: %s\n", file.Path, plural(file.Scenarios, "scenario"))
		scenarios += file.Scenarios
	}
	fmt.Fprintf(w, "%s, %s: %d @test, %d @benchmark, %d @manual\n",
		plural(len(result.Files), "file"), plural(scenarios, "scenario"),
		result.Tags[tagTest], result.Tags[tagBenchmark], result.Tags[tagManual])
	fmt.Fprintf(w, "cited by @test: %s, %s\n",
		plural(result.Paths, "path"), plural(result.Tests, "Go test"))
}

func printProblems(w io.Writer, problems []Problem) {
	for _, p := range problems {
		fmt.Fprintf(w, "%s:%d: %s\n", p.File, p.Line, p.Message)
	}
	fmt.Fprintf(w, "spec/check: %s\n", plural(len(problems), "problem"))
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("1 %s", noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
