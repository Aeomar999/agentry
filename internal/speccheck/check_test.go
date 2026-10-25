package main

import (
	"bytes"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

// A scenario that passes every rule, to break one rule at a time.
const goodScenario = `Feature: Tasks
  What the person at the page does.

  @USER-01 @test
  Scenario: A new task goes to the coordinator
    Given a repo with a Software team
    When the user creates a task from the page
    Then the task is shown in To do
    And its coordinator is asked to decide
    # internal/web/api_test.go
`

// goodTestFile is the test file goodScenario cites, in the repo root.
const goodTestFile = "internal/web/api_test.go"

// writeSpec writes the files into a spec folder under a fresh temp folder,
// the repo root, and gives back that spec folder. A name starting with ../
// is a file of the repo outside the spec folder. goodTestFile is written
// too, unless the files give it.
func writeSpec(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "spec")
	if _, given := files["../"+goodTestFile]; !given {
		files = maps.Clone(files)
		files["../"+goodTestFile] = "package web\n"
	}
	for name, text := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// problemsOf runs the checker over the files and gives back each problem as
// "name:line: message", with the temp folder cut off.
func problemsOf(t *testing.T, files map[string]string) []string {
	t.Helper()
	dir := writeSpec(t, files)
	result, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	var lines []string
	for _, p := range result.Problems {
		rel, _ := filepath.Rel(dir, p.File)
		lines = append(lines, rel+":"+strconv.Itoa(p.Line)+": "+p.Message)
	}
	return lines
}

func expectProblems(t *testing.T, got []string, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d problems, want %d:\n%s", len(got), len(want), strings.Join(got, "\n"))
	}
	for i := range want {
		if !strings.Contains(got[i], want[i]) {
			t.Errorf("problem %d is %q, want one holding %q", i+1, got[i], want[i])
		}
	}
}

func TestAScenarioCarriesItsFilesID(t *testing.T) {
	for name, c := range map[string]struct{ tags, want string }{
		"no ID":          {"@test", "user.feature:5: no ID tag; this file's scenarios are tagged @USER-nn"},
		"another prefix": {"@COORD-01 @test", "user.feature:5: ID @COORD-01 does not match this file's prefix @USER-nn"},
		"one digit":      {"@USER-1 @test", "user.feature:5: no ID tag"},
		"two IDs":        {"@USER-01 @USER-02 @test", "user.feature:5: 2 ID tags; a scenario has one"},
	} {
		t.Run(name, func(t *testing.T) {
			text := strings.Replace(goodScenario, "@USER-01 @test", c.tags, 1)
			expectProblems(t, problemsOf(t, map[string]string{"functional/user.feature": text}), c.want)
		})
	}
}

func TestAScenarioCarriesExactlyOneCheckTag(t *testing.T) {
	for name, c := range map[string]struct{ tags, want string }{
		"none": {"@USER-01", "user.feature:5: no check tag; a scenario has exactly one of @test, @benchmark, @manual"},
		"two":  {"@USER-01 @test @manual", "user.feature:5: more than one check tag (@test, @manual); a scenario has exactly one"},
	} {
		t.Run(name, func(t *testing.T) {
			text := strings.Replace(goodScenario, "@USER-01 @test", c.tags, 1)
			expectProblems(t, problemsOf(t, map[string]string{"functional/user.feature": text}), c.want)
		})
	}
}

func TestAnIDIsUsedOnceInTheFolder(t *testing.T) {
	twice := goodScenario + "\n" + strings.Replace(goodScenario, "Feature: Tasks\n  What the person at the page does.\n\n", "", 1)
	expectProblems(t, problemsOf(t, map[string]string{"functional/user.feature": twice}),
		"user.feature:12: ID @USER-01 is already used at ")
}

func TestAThenStatesANumberOrAnOutcome(t *testing.T) {
	for name, c := range map[string]struct {
		then  string
		wrong bool
	}{
		"vague":                  {"Then the task shows up", true},
		"a number":               {"Then the board shows 1 task", false},
		"an outcome word":        {"Then the task is shown on the board", false},
		"the word in any case":   {"Then NOT a thing changes", false},
		"a contraction":          {"Then the task doesn't move", true},
		"the word inside a word": {"Then the task is noted", true},
		"runs says nothing":      {"Then the page runs", true},
		"does says nothing":      {"Then the task does its work", true},
		"a cannot case":          {"Then the agent cannot push", false},
		"nothing happens":        {"Then nothing moves", false},
	} {
		t.Run(name, func(t *testing.T) {
			text := strings.Replace(goodScenario, "Then the task is shown in To do", c.then, 1)
			got := problemsOf(t, map[string]string{"functional/user.feature": text})
			if c.wrong {
				expectProblems(t, got, "user.feature:8: Then has no number and no outcome word; the words are not, no, never")
			} else {
				expectProblems(t, got)
			}
		})
	}
}

func TestAnAndAfterAThenIsAThen(t *testing.T) {
	text := strings.Replace(goodScenario, "And its coordinator is asked to decide", "And its coordinator decides", 1)
	expectProblems(t, problemsOf(t, map[string]string{"functional/user.feature": text}),
		"user.feature:9: Then has no number and no outcome word")

	// An And after a Given or When is not a Then, and a When ends the Then.
	text = strings.Replace(goodScenario, "    When the user creates a task from the page\n",
		"    And the user is signed in\n    When the user creates a task from the page\n", 1)
	text = strings.Replace(text, "    # internal/web/api_test.go\n", "    When the user looks again\n    And the page is open\n", 1)
	expectProblems(t, problemsOf(t, map[string]string{"functional/user.feature": text}))
}

func TestVagueWordsAreRefused(t *testing.T) {
	for name, c := range map[string]struct{ old, new, want string }{
		"in a step":    {"Given a repo with a Software team", "Given the server starts quickly", `user.feature:6: the word "quickly"`},
		"in the title": {"Scenario: A new task goes to the coordinator", "Scenario: A task is handled Properly", `user.feature:5: the word "properly"`},
		"in a comment": {"# internal/web/api_test.go", "# checked appropriately", `user.feature:10: the word "appropriately"`},
	} {
		t.Run(name, func(t *testing.T) {
			text := strings.Replace(goodScenario, c.old, c.new, 1)
			expectProblems(t, problemsOf(t, map[string]string{"functional/user.feature": text}), c.want)
		})
	}
}

func TestAScenarioOutlineIsRefusedAndNotCounted(t *testing.T) {
	outline := goodScenario + `
  @USER-02 @test
  Scenario Outline: A task ends up <outcome>
    Given a task
    When the coordinator decides
    Then the task is <outcome>

    Examples:
      | outcome |
      | done    |
`
	dir := writeSpec(t, map[string]string{"functional/user.feature": outline})
	result, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := result.Files[0].Scenarios; got != 1 {
		t.Errorf("counted %d scenarios, want 1: the outline is not one", got)
	}
	if got := result.Tags[tagTest]; got != 1 {
		t.Errorf("counted %d @test, want 1: the outline's tags are not counted", got)
	}
	expectProblems(t, problemsOf(t, map[string]string{"functional/user.feature": outline}),
		"user.feature:13: Scenario Outline is not used in this spec: write one Scenario per case")
}

func TestABlankLineBetweenTheTagsAndTheScenarioIsExplained(t *testing.T) {
	text := strings.Replace(goodScenario, "@USER-01 @test\n", "@USER-01 @test\n\n", 1)
	expectProblems(t, problemsOf(t, map[string]string{"functional/user.feature": text}),
		"user.feature:6: no ID tag; this file's scenarios are tagged @USER-nn; the tags must be on the line directly above Scenario:",
		"user.feature:6: no check tag; a scenario has exactly one of @test, @benchmark, @manual; the tags must be on the line directly above Scenario:")

	// With no tag line above, the hint is not given.
	text = strings.Replace(goodScenario, "  @USER-01 @test\n", "", 1)
	got := problemsOf(t, map[string]string{"functional/user.feature": text})
	expectProblems(t, got, "user.feature:4: no ID tag", "user.feature:4: no check tag")
	for _, p := range got {
		if strings.Contains(p, "directly above") {
			t.Errorf("hint given with no tag line above: %s", p)
		}
	}
}

func TestACommentBetweenTheTagsAndTheScenarioIsExplained(t *testing.T) {
	text := strings.Replace(goodScenario, "@USER-01 @test\n", "@USER-01 @test\n  # internal/web/api_test.go\n", 1)
	expectProblems(t, problemsOf(t, map[string]string{"functional/user.feature": text}),
		"user.feature:6: no ID tag; this file's scenarios are tagged @USER-nn; the tags must be on the line directly above Scenario:",
		"user.feature:6: no check tag; a scenario has exactly one of @test, @benchmark, @manual; the tags must be on the line directly above Scenario:")
}

func TestARejectedScenarioIsNotCountedUnderAnyCheckTag(t *testing.T) {
	text := goodScenario + "\n" + strings.NewReplacer("Feature: Tasks\n  What the person at the page does.\n\n", "",
		"@USER-01 @test", "@USER-02 @test @manual").Replace(goodScenario)
	dir := writeSpec(t, map[string]string{"functional/user.feature": text})
	result, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Problems) == 0 {
		t.Error("a scenario with two check tags gives no problem, so the run would not fail")
	}
	if got := result.Files[0].Scenarios; got != 2 {
		t.Errorf("counted %d scenarios, want 2", got)
	}
	if test, manual := result.Tags[tagTest], result.Tags[tagManual]; test != 1 || manual != 0 {
		t.Errorf("counted %d @test and %d @manual, want 1 and 0: the rejected scenario counts under neither", test, manual)
	}
}

func TestATestScenarioCitesPathsAndTestsThatExist(t *testing.T) {
	repo := map[string]string{
		"../" + goodTestFile:            "package web\n\nfunc TestCreate(t *testing.T) {}\n\nfunc TestList(t *testing.T) {}\n",
		"../internal/web/api.go":        "package web\n",
		"../internal/web/empty_test.go": "",
		"../ui/src/Badge.test.tsx":      "",
		"../README.md":                  "# Agentry\n",
	}
	for name, c := range map[string]struct{ tags, comment, want string }{
		"a path and its tests":       {"@USER-01 @test", "# internal/web/api_test.go TestCreate and TestList", ""},
		"a missing path":             {"@USER-01 @test", "# internal/web/gone_test.go", "user.feature:10: cited path internal/web/gone_test.go does not exist"},
		"a renamed test":             {"@USER-01 @test", "# internal/web/api_test.go TestMake", "user.feature:10: TestMake is not defined in internal/web/api_test.go"},
		"a test with no path":        {"@USER-01 @test", "# the list: TestList", "user.feature:10: TestList is cited with no Go test file before it"},
		"in the same file":           {"@USER-01 @test", "# internal/web/api_test.go TestCreate; listing: TestList in the same file", ""},
		"gone from the file":         {"@USER-01 @test", "# internal/web/api_test.go TestCreate; listing: TestGone in the same file", "user.feature:10: TestGone is not defined in internal/web/api_test.go"},
		"on the next line":           {"@USER-01 @test", "# internal/web/api_test.go TestCreate\n    # the list: TestGone", "user.feature:11: TestGone is not defined in internal/web/api_test.go"},
		"a test before its path":     {"@USER-01 @test", "# TestCreate in internal/web/api_test.go", ""},
		"a non-test file between":    {"@USER-01 @test", "# internal/web/api_test.go TestCreate; the handler: internal/web/api.go\n    # TestList", ""},
		"a path with a line":         {"@USER-01 @test", "# internal/web/api.go:42 and internal/web/api.go:3-5", ""},
		"a missing path with a line": {"@USER-01 @test", "# internal/web/gone.go:42", "user.feature:10: cited path internal/web/gone.go does not exist"},
		"an empty test file":         {"@USER-01 @test", "# internal/web/empty_test.go TestCreate", "user.feature:10: TestCreate is not defined in internal/web/empty_test.go"},
		"a UI path":                  {"@USER-01 @test", "# ui/src/Badge.test.tsx, the badge's colour", ""},
		"a missing UI path":          {"@USER-01 @test", "# ui/src/Gone.test.tsx", "user.feature:10: cited path ui/src/Gone.test.tsx does not exist"},
		"a glob":                     {"@USER-01 @test", "# every page: ui/src/**/*.tsx", ""},
		"a root file":                {"@USER-01 @test", "# README.md, Use it; PLAN.md T-013", "user.feature:10: cited path PLAN.md does not exist"},
		"a manual scenario":          {"@USER-01 @manual", "# internal/web/gone_test.go TestGone", ""},
	} {
		t.Run(name, func(t *testing.T) {
			files := maps.Clone(repo)
			files["functional/user.feature"] = strings.NewReplacer("@USER-01 @test", c.tags, "# internal/web/api_test.go", c.comment).Replace(goodScenario)
			got := problemsOf(t, files)
			if c.want == "" {
				expectProblems(t, got)
			} else {
				expectProblems(t, got, c.want)
			}
		})
	}
}

func TestTheCitationsCheckedAreCounted(t *testing.T) {
	dir := writeSpec(t, map[string]string{
		"../" + goodTestFile:      "package web\n\nfunc TestCreate(t *testing.T) {}\n",
		"functional/user.feature": strings.Replace(goodScenario, "# internal/web/api_test.go", "# internal/web/api_test.go TestCreate and TestGone", 1),
	})
	result, err := Check(dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.Paths != 1 || result.Tests != 2 {
		t.Errorf("counted %d paths and %d Go tests, want 1 and 2", result.Paths, result.Tests)
	}
}

func TestANewScenarioCitesNoLineNumber(t *testing.T) {
	for name, c := range map[string]struct{ tags, comment, want string }{
		"a new ID with line":   {"@USER-134 @test", "# internal/web/api_test.go, line 12", "user.feature:10: a line number goes stale as dev moves; cite a function or a test by name instead"},
		"a new ID with lines":  {"@USER-134 @test", "# internal/web/api_test.go, lines 12 and 13", "user.feature:10: a line number goes stale"},
		"a new manual ID":      {"@USER-134 @manual", "# internal/web/api.go line 12", "user.feature:10: a line number goes stale"},
		"an old ID with line":  {"@USER-133 @test", "# internal/web/api_test.go, line 12", ""},
		"a new ID with a name": {"@USER-134 @test", "# internal/web/api_test.go, the timeline 5 times", ""},
		"a capital Line":       {"@USER-134 @test", "# internal/web/api_test.go, Line 12", "user.feature:10: a line number goes stale"},
		"a capital Lines":      {"@USER-134 @test", "# internal/web/api_test.go, Lines 3 to 5", "user.feature:10: a line number goes stale"},
	} {
		t.Run(name, func(t *testing.T) {
			text := strings.NewReplacer("@USER-01 @test", c.tags, "# internal/web/api_test.go", c.comment).Replace(goodScenario)
			got := problemsOf(t, map[string]string{"functional/user.feature": text})
			if c.want == "" {
				expectProblems(t, got)
			} else {
				expectProblems(t, got, c.want)
			}
		})
	}
}

func TestAStepMaySayLine(t *testing.T) {
	text := strings.NewReplacer("@USER-01 @test", "@USER-134 @test",
		"Then the task is shown in To do", "Then line 1 of the report is the verdict").Replace(goodScenario)
	expectProblems(t, problemsOf(t, map[string]string{"functional/user.feature": text}))

	text = strings.Replace(text, "# internal/web/api_test.go", "# internal/web/api_test.go, line 1 of the report is the verdict", 1)
	expectProblems(t, problemsOf(t, map[string]string{"functional/user.feature": text}),
		"user.feature:10: a line number goes stale")
}

// notInTheRootLists are the repo root's entries that are not cited: git's
// own and what .gitignore leaves out. A name starting with a dot that is a
// folder is skipped too, as .agentry is.
var notInTheRootLists = []string{".git", "bin", "release", "agentry", "node_modules", "__pycache__", "*.db", "*.db-wal", "*.db-shm"}

func TestTheRootListsNameEveryTopFolderAndRootFile(t *testing.T) {
	entries, err := os.ReadDir(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		name := entry.Name()
		if ignored(name) || entry.IsDir() && strings.HasPrefix(name, ".") {
			continue
		}
		if entry.IsDir() && !slices.Contains(rootFolders, name) {
			t.Errorf("the top folder %s is not in rootFolders, so a path under it is not checked", name)
		}
		if !entry.IsDir() && !slices.Contains(rootFiles, name) {
			t.Errorf("the root file %s is not in rootFiles, so citing it is not checked", name)
		}
	}
}

func ignored(name string) bool {
	for _, pattern := range notInTheRootLists {
		if matched, _ := filepath.Match(pattern, name); matched {
			return true
		}
	}
	return false
}

func TestAnEmDashAnywhereUnderTheFolderFails(t *testing.T) {
	expectProblems(t, problemsOf(t, map[string]string{
		"functional/user.feature": goodScenario,
		"README.md":               "# The spec\n\nPlain prose \u2014 with a dash.\n",
	}), "README.md:3: em-dash")
}

func TestAnUnknownFeatureFileNameFails(t *testing.T) {
	expectProblems(t, problemsOf(t, map[string]string{"functional/users.feature": goodScenario}),
		"users.feature:1: unknown feature file name; the known names are agent.feature, claude-code.feature, compatibility.feature")
}

func TestAGoodSpecPassesAndIsCounted(t *testing.T) {
	coordinator := `Feature: The coordinator

  @COORD-01 @manual
  Scenario: A decision hands the task to one agent
    Given a task whose coordinator has decided
    When the loop reads the decision
    Then exactly one agent gets a turn

  @COORD-02 @benchmark
  Scenario: Decisions are read fast
    Given 1000 decisions
    When they are read
    Then it takes under 1 second
`
	dir := writeSpec(t, map[string]string{
		"README.md":                      "# The spec\n",
		"functional/user.feature":        goodScenario,
		"functional/coordinator.feature": coordinator,
		"non-functional/limits.feature":  strings.NewReplacer("USER", "LIM", "Tasks", "Limits").Replace(goodScenario),
	})
	t.Chdir(filepath.Dir(dir))
	if _, err := Check("nowhere"); err == nil {
		t.Error("a folder that is not there should be an error")
	}
	result, err := Check("spec")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Problems) != 0 {
		t.Fatalf("a good spec has problems: %+v", result.Problems)
	}
	var out bytes.Buffer
	printCounts(&out, result)
	want := `spec/functional/coordinator.feature: 2 scenarios
spec/functional/user.feature: 1 scenario
spec/non-functional/limits.feature: 1 scenario
3 files, 4 scenarios: 2 @test, 1 @benchmark, 1 @manual
cited by @test: 2 paths, 0 Go tests
`
	if out.String() != want {
		t.Errorf("counts:\n%s\nwant:\n%s", out.String(), want)
	}
}
