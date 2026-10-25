package main

import (
	"bytes"
	"os"
	"path/filepath"
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

// writeSpec writes the files into a spec folder under a fresh temp folder
// and gives back that spec folder.
func writeSpec(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "spec")
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
`
	if out.String() != want {
		t.Errorf("counts:\n%s\nwant:\n%s", out.String(), want)
	}
}
