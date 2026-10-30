// Command speccheck checks the spec folder, spec/ by default, so that every
// scenario in it can be found, counted and judged.
//
// It walks every .feature file under the folder and reports a scenario that
//
//   - lacks an ID tag with the prefix its file name gives it (@TASK-01 in
//     tasks-and-board.feature), or sits in a file whose name is not in the
//     prefix table;
//   - lacks exactly one of @test, @benchmark and @manual;
//   - sits under functional/ and lacks exactly one of the role tags @user,
//     @coordinator, @agent, @claude-code and @worker;
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
// It also reports a path that a feature file's "# Sources:" comment names
// and the repo doesn't have, in any file, proposals/ too.
//
// A file directly under the folder's proposals/ holds a feature that is
// agreed and not built. Any file name is allowed there, and its scenarios
// follow the rules above with these changes: the ID tag is @PROP-nn; each
// scenario has exactly one target tag, @to-<stem> naming a file of the
// prefix table without .feature, or @to-new-<stem> for a file that does not
// exist yet; the role tag rule is the target's (exactly one for a
// functional file, none for a non-functional one, one or none for a new
// one); and a @test scenario's citations are not looked up, as its test may
// not exist yet. A target tag on a scenario outside proposals/ is reported.
//
// Cited paths are read from the folder's parent, the repo root. A Scenario
// Outline is reported and not counted: this spec writes one Scenario per
// case. The checker also reports an em-dash anywhere in any file under the
// folder. Problems are printed as file:line: message; the scenarios per
// file, the totals per check tag, outside proposals/ and then under it, and
// how many cited paths and Go tests were checked are printed either way;
// the exit code is 1 on any problem.
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
// The proposals are totalled on a line of their own, so the baseline's
// totals stay what exists.
func printCounts(w io.Writer, result Result) {
	var files, scenarios [2]int // outside proposals/, then under it
	for _, file := range result.Files {
		fmt.Fprintf(w, "%s: %s\n", file.Path, plural(file.Scenarios, "scenario"))
		group := 0
		if file.Proposal {
			group = 1
		}
		files[group]++
		scenarios[group] += file.Scenarios
	}
	printTotals(w, "", files[0], scenarios[0], result.Tags)
	printTotals(w, "proposals: ", files[1], scenarios[1], result.ProposalTags)
	fmt.Fprintf(w, "cited by @test: %s, %s\n",
		plural(result.Paths, "path"), plural(result.Tests, "Go test"))
}

func printTotals(w io.Writer, label string, files, scenarios int, tags map[string]int) {
	fmt.Fprintf(w, "%s%s, %s: %d @test, %d @benchmark, %d @manual\n", label,
		plural(files, "file"), plural(scenarios, "scenario"),
		tags[tagTest], tags[tagBenchmark], tags[tagManual])
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
