package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A Problem is one thing wrong, at a line of a file under the spec folder.
type Problem struct {
	File    string
	Line    int
	Message string
}

// A FileCount is how many scenarios one feature file holds.
type FileCount struct {
	Path      string
	Scenarios int
}

// Result is what one run of the checker found.
type Result struct {
	Files    []FileCount    // every .feature file, in path order
	Tags     map[string]int // scenarios per check tag
	Paths    int            // paths cited by @test scenarios, checked
	Tests    int            // Go tests cited by @test scenarios, checked
	Problems []Problem
}

// Check walks the spec folder and judges every feature file in it, then
// every file for an em-dash. Cited paths are read from the folder's parent,
// the repo root. The error is for a folder that cannot be read, not for a
// spec that fails: that is in the problems.
func Check(folder string) (Result, error) {
	result := Result{Tags: map[string]int{}}
	seen := map[string]string{} // ID -> where it was first used
	cites := newCiter(filepath.Dir(filepath.Clean(folder)))
	err := filepath.WalkDir(folder, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		text, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result.Problems = append(result.Problems, emDashes(path, string(text))...)
		if filepath.Ext(path) == ".feature" {
			checkFeature(&result, seen, cites, path, string(text))
		}
		return nil
	})
	result.Paths, result.Tests = cites.paths, cites.tests
	return result, err
}

// checkFeature counts one feature file's scenarios and adds what is wrong
// with them to the result.
func checkFeature(result *Result, seen map[string]string, cites *citer, path, text string) {
	scenarios, outlines := parseFeature(path, text)
	result.Files = append(result.Files, FileCount{Path: path, Scenarios: len(scenarios)})
	prefix, known := prefixes[filepath.Base(path)]
	functional := filepath.Base(filepath.Dir(path)) == "functional"
	if !known {
		result.Problems = append(result.Problems, Problem{path, 1, "unknown feature file name; the known names are " + knownNames()})
	}
	for _, l := range outlines {
		result.Problems = append(result.Problems, Problem{path, l.number, "Scenario Outline is not used in this spec: write one Scenario per case"})
	}
	for _, s := range scenarios {
		// A scenario rejected for its check tags counts under none of them.
		if checkTagProblem(s) == "" {
			for _, t := range s.tags {
				result.Tags[t.name]++
			}
		}
		result.Problems = append(result.Problems, scenarioProblems(s, prefix, seen)...)
		if functional {
			if message := roleTagProblem(s); message != "" {
				result.Problems = append(result.Problems, Problem{s.file, s.line, message + tagHint(s)})
			}
		}
		if hasTag(s, tagTest) {
			result.Problems = append(result.Problems, cites.problems(s)...)
		}
	}
}

// scenarioProblems runs every rule over one scenario. An empty prefix, for
// a file whose name gives it none, skips the ID rule: the file is the problem.
func scenarioProblems(s scenario, prefix string, seen map[string]string) []Problem {
	var problems []Problem
	at := func(line int, message string) {
		problems = append(problems, Problem{s.file, line, message})
	}
	if prefix != "" {
		if message := idProblem(s, prefix); message != "" {
			at(s.line, message+tagHint(s))
		}
	}
	if message := checkTagProblem(s); message != "" {
		at(s.line, message+tagHint(s))
	}
	for _, id := range idTags(s) {
		where := fmt.Sprintf("%s:%d", s.file, id.line)
		if first, used := seen[id.name]; used {
			at(id.line, fmt.Sprintf("ID %s is already used at %s", id.name, first))
		} else {
			seen[id.name] = where
		}
	}
	for _, l := range thenLines(s.body) {
		if !hasOutcome(l.text) {
			at(l.number, "Then has no number and no outcome word; the words are "+strings.Join(outcomeWords, ", "))
		}
	}
	for _, l := range append([]line{{s.line, s.title}}, s.body...) {
		if word := bannedWordIn(l.text); word != "" {
			at(l.number, fmt.Sprintf("the word %q says nothing a check can judge; say what happens", word))
		}
	}
	for _, l := range lineReferences(s, prefix) {
		at(l.number, "a line number goes stale as dev moves; cite a function or a test by name instead")
	}
	return problems
}

// emDashes finds every line of a file that holds an em-dash.
func emDashes(path, text string) []Problem {
	var problems []Problem
	for i, l := range strings.Split(text, "\n") {
		if strings.Contains(l, emDash) {
			problems = append(problems, Problem{path, i + 1, "em-dash; write a comma, a colon or a full stop"})
		}
	}
	return problems
}

func knownNames() string {
	names := make([]string, 0, len(prefixes))
	for name := range prefixes {
		names = append(names, name)
	}
	slices.Sort(names)
	return strings.Join(names, ", ")
}
