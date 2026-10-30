package coordinator

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/platform/workspace"
	"gitlab.com/dev.chempa/agentry/internal/testkit/storetest"
)

// day is noon on the nth day of a fixed month, so commit and turn times
// fall unmistakably before or after each other.
func day(n int) time.Time {
	return time.Date(2026, time.March, n, 12, 0, 0, 0, time.UTC)
}

// twoCommits are the task branch's commits, newest first: the engineer's
// first commit on day 1 and its fix on day 3, around a review on day 2.
func twoCommits() []workspace.Commit {
	return []workspace.Commit{
		{Hash: "c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2", Subject: "fix: remove the last TODO", At: day(3),
			Files: []workspace.File{{Path: "src/b.go", Change: workspace.Modified}}},
		{Hash: "c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1c1", Subject: "fix: remove the TODOs", At: day(1),
			Files: []workspace.File{{Path: "src/b.go", Change: workspace.Added}, {Path: "src/a.go", Change: workspace.Modified}}},
	}
}

// firstReview is a task whose engineer has worked once and whose reviewer
// is about to check it for the first time.
func firstReview() history {
	return history{}.
		add(you, events.TaskCreated, "", map[string]any{"title": "Remove the TODOs", "description": "Every TODO in src/ goes.\n\nDone when `grep TODO src/` finds nothing.",
			"repo": "/r", "coordinator": "tech-lead"}).
		add(worker, events.WorktreeReady, "", map[string]any{"path": "/w/task-1", "branch": "agentry/task-1", "base": "b0b0b0b0", "port": 7901}).
		turn("turn-1", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Remove them."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove them."}).
		add(worker, events.TurnStarted, "turn-2", map[string]any{"agent": "engineer", "hash": "h"}).
		add(worker, events.ActionRequested, "push", map[string]any{"tool": "Bash", "command": "git push origin HEAD:dev"}).
		add(you, events.ActionDenied, "push", map[string]any{"reason": "agents don't push"}).
		add(worker, events.TurnEnded, "turn-2", map[string]any{"agent": "engineer", "report": "Removed 3."}).
		turn("turn-3", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Review it."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "reviewer", "instructions": "Review it."})
}

// trailLeaks is what a handoff must never carry: the coordinator's
// decisions, the trail's lines, earlier instructions and the denied push.
var trailLeaks = []string{`{"action": "assign"`, "you → tech-lead", "tech-lead → engineer", "engineer → tech-lead", "Remove them.",
	"git push origin HEAD:dev", "agents don't push", "## What has happened"}

func TestAFirstReviewsHandoffIsTheTaskTheRangeAndTheBuildersReport(t *testing.T) {
	roster := defaultRoster()
	task := firstReview().task()
	reviewer := agentNamed(t, roster, "reviewer")

	brief := handoffBrief(task, reviewer, roster, "Review it.", nil, twoCommits())
	for _, want := range []string{"# Agentry brief: reviewer, task-1, turn-4\n\n" + reviewer.Prompt, "## What you know from earlier tasks",
		"## The task\n\nTitle: Remove the TODOs\n\nEvery TODO in src/ goes.", "Done when `grep TODO src/` finds nothing.",
		"## Your instructions from the coordinator\n\nReview it.", "worktree, /w/task-1, on branch agentry/task-1, which started from commit b0b0b0b0",
		"## Shared files", "The task has no shared files yet.", ".agentry/turn-4-plan.md",
		"## What to review\n\nThe commit range is b0b0b0b0..c2c2c2c2: 2 commits on the branch since it started.",
		"It touches 2 files:\n- M src/a.go\n- M src/b.go\n", "git log --oneline b0b0b0b0..HEAD", "git diff b0b0b0b0..HEAD\n", "git diff b0b0b0b0..HEAD -- <file>",
		"never carries the diff itself", "## The builder's last report\n\nFrom engineer, in turn-2:\n\nRemoved 3.",
		"## When you finish", strings.TrimSpace(VerdictFormat)} {
		if !strings.Contains(brief, want) {
			t.Errorf("the handoff is missing %q:\n%s", want, brief)
		}
	}
	for _, left := range append(trailLeaks, "## Your earlier findings", "## Changed since your last verdict") {
		if strings.Contains(brief, left) {
			t.Errorf("the handoff should leave out %q:\n%s", left, brief)
		}
	}
	if order := []string{"## The task", "## Your instructions", "## Where the work is", "## Your plan", "## What to review",
		"## The builder's last report", "## When you finish"}; !inOrder(brief, order) {
		t.Errorf("the sections aren't in the order %v:\n%s", order, brief)
	}
}

// inOrder says whether each heading comes after the one before it.
func inOrder(text string, headings []string) bool {
	at := -1
	for _, heading := range headings {
		next := strings.Index(text, heading)
		if next < at {
			return false
		}
		at = next
	}
	return true
}

// A re-review adds the reviewer's own earlier findings and the commits
// since its last turn; the builder's report is the latest builder's.
func TestAReReviewsHandoffHasItsEarlierFindingsAndWhatChangedSince(t *testing.T) {
	roster := defaultRoster()
	task := fixRound().
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove the one in src/b.go."}).
		turn("turn-6", "engineer", "Removed it.").
		turn("turn-7", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Review the fix."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "reviewer", "instructions": "Review the fix."}).task()
	task.Turn("turn-4").StartedAt = day(2) // the review, between the two commits
	task.Worktree.Base = "b0b0b0b0"

	brief := handoffBrief(task, agentNamed(t, roster, "reviewer"), roster, "Review the fix.", nil, twoCommits())
	t.Logf("a re-review handoff:\n%s", brief)
	for _, want := range []string{"# Agentry brief: reviewer, task-1, turn-8\n", "## Your instructions from the coordinator\n\nReview the fix.",
		"## Your earlier findings\n\nFrom your last turn on this task, turn-4:\n\nOne is left in src/b.go.\n\nVerdict: changes requested",
		"## Changed since your last verdict\n\n1 commit since your last turn, newest first:\n- c2c2c2c2 fix: remove the last TODO\n",
		"It touches 1 file:\n- M src/b.go", "git diff c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2c2^..HEAD",
		"## The builder's last report\n\nFrom engineer, in turn-6:\n\nRemoved it.", "The commit range is b0b0b0b0..c2c2c2c2"} {
		if !strings.Contains(brief, want) {
			t.Errorf("the re-review handoff is missing %q:\n%s", want, brief)
		}
	}
	for _, left := range append(trailLeaks, "Removed 3.", "- c1c1c1c1 fix: remove the TODOs") {
		if strings.Contains(brief, left) {
			t.Errorf("the re-review handoff should leave out %q:\n%s", left, brief)
		}
	}
	if !strings.HasSuffix(strings.TrimSpace(brief), strings.TrimSpace(VerdictFormat)) {
		t.Errorf("a reviewer's handoff ends with the verdict format:\n%s", brief)
	}
}

// A report the handoff quotes keeps its own headings below the brief's
// sections, so the lines that start with "## " are the handoff's alone and
// a reader can tell where the quoted report ends. Code in a fence stays.
func TestAQuotedReportsHeadingsStayBelowTheHandoffsSections(t *testing.T) {
	roster := defaultRoster()
	const report = "## Report for the coordinator\n\nDone.\n\n```sh\n# comment\ngrep TODO src/\n```\n\n# Left\n\nNothing."
	task := firstReview().
		turn("turn-4", "reviewer", report+"\n\nVerdict: changes requested").
		turn("turn-5", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Remove the one in src/b.go."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove the one in src/b.go."}).
		turn("turn-6", "engineer", report).
		turn("turn-7", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Review the fix."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "reviewer", "instructions": "Review the fix."}).task()
	task.Turn("turn-4").StartedAt = day(2)

	brief := handoffBrief(task, agentNamed(t, roster, "reviewer"), roster, "Review the fix.", nil, twoCommits())
	var sections []string
	for _, line := range strings.Split(brief, "\n") {
		if strings.HasPrefix(line, "## ") {
			sections = append(sections, line)
		}
	}
	want := []string{"## What you know from earlier tasks", "## The task", "## Your instructions from the coordinator", "## Where the work is",
		"## Shared files", "## Your plan", "## What to review", "## The builder's last report", "## Your earlier findings",
		"## Changed since your last verdict", "## When you finish"}
	if fmt.Sprint(sections) != fmt.Sprint(want) {
		t.Errorf("the handoff's sections are %q, want %q:\n%s", sections, want, brief)
	}
	for _, want := range []string{"From engineer, in turn-6:\n\n#### Report for the coordinator\n\nDone.",
		"From your last turn on this task, turn-4:\n\n#### Report for the coordinator\n\nDone.", "\n### Left\n"} {
		if !strings.Contains(brief, want) {
			t.Errorf("the handoff is missing %q:\n%s", want, brief)
		}
	}
	if n := strings.Count(brief, "#### Report for the coordinator"); n != 2 {
		t.Errorf("the quoted heading appears %d times, want 2:\n%s", n, brief)
	}
	if n := strings.Count(brief, "```sh\n# comment\ngrep TODO src/\n```"); n != 2 {
		t.Errorf("the fenced comment should stay as it is in both reports, found %d:\n%s", n, brief)
	}
}

// With no commit newer than the reviewer's last turn, the handoff says so:
// the coordinator may have asked for a re-check all the same.
func TestAReReviewWithNothingNewerSaysSo(t *testing.T) {
	roster := defaultRoster()
	task := fixRound().
		turn("turn-6", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Check again."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "reviewer", "instructions": "Check again."}).task()
	task.Turn("turn-4").StartedAt = day(5) // after both commits

	brief := handoffBrief(task, agentNamed(t, roster, "reviewer"), roster, "Check again.", nil, twoCommits())
	if !strings.Contains(brief, "## Changed since your last verdict\n\nNo commits since your last turn.") || strings.Contains(brief, "^..HEAD") {
		t.Errorf("the handoff should say no commit is newer:\n%s", brief)
	}
}

// cutShort is a turn of the agent's that ended with an issue, so it left
// no report.
func (h history) cutShort(id, agent, reason string) history {
	return h.add(worker, events.TurnStarted, id, map[string]any{"agent": agent, "hash": "h"}).
		add(watchdog, events.IssueRaised, "", map[string]any{"reason": reason})
}

// A reviewer whose only earlier turn was cut short has no findings to be
// given back, so its retry is briefed as a first review: neither section
// is there to suggest the range was checked.
func TestAReviewerWhoseOnlyTurnWasCutShortGetsAFirstReview(t *testing.T) {
	roster := defaultRoster()
	task := firstReview().
		cutShort("turn-4", "reviewer", "the agent's process exited").
		turn("turn-5", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Review it."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "reviewer", "instructions": "Review it."}).task()

	brief := handoffBrief(task, agentNamed(t, roster, "reviewer"), roster, "Review it.", nil, twoCommits())
	for _, left := range []string{"## Your earlier findings", "## Changed since your last verdict", "(It left no report.)", "No commits since your last turn"} {
		if strings.Contains(brief, left) {
			t.Errorf("a retry after a cut-short turn should leave out %q:\n%s", left, brief)
		}
	}
	if !strings.Contains(brief, "## The builder's last report\n\nFrom engineer, in turn-2:\n\nRemoved 3.") {
		t.Errorf("the retry still has the builder's report:\n%s", brief)
	}
}

// A reviewer that reported once and was then cut short on a retry is given
// back the report it did write, and what changed since that turn.
func TestAReviewerCutShortOnARetryIsGivenItsRealFindings(t *testing.T) {
	roster := defaultRoster()
	task := fixRound().
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove the one in src/b.go."}).
		turn("turn-6", "engineer", "Removed it.").
		turn("turn-7", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Review the fix."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "reviewer", "instructions": "Review the fix."}).
		cutShort("turn-8", "reviewer", "the engine crashed").
		turn("turn-9", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Review the fix."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "reviewer", "instructions": "Review the fix."}).task()
	task.Turn("turn-4").StartedAt = day(2) // the real review, between the two commits
	task.Turn("turn-8").StartedAt = day(5) // the cut-short retry, after both

	brief := handoffBrief(task, agentNamed(t, roster, "reviewer"), roster, "Review the fix.", nil, twoCommits())
	for _, want := range []string{"## Your earlier findings\n\nFrom your last turn on this task, turn-4:\n\nOne is left in src/b.go.",
		"## Changed since your last verdict\n\n1 commit since your last turn, newest first:\n- c2c2c2c2 fix: remove the last TODO\n"} {
		if !strings.Contains(brief, want) {
			t.Errorf("the handoff is missing %q:\n%s", want, brief)
		}
	}
	for _, left := range []string{"turn-8", "(It left no report.)", "No commits since your last turn"} {
		if strings.Contains(brief, left) {
			t.Errorf("the handoff should leave out %q:\n%s", left, brief)
		}
	}
}

// The builder's report is the last one written, not the last turn: a turn
// cut short after it is noted under the report instead of hiding it. A
// shipper's report counts as a builder's, since it too may have changed
// the branch.
func TestTheBuildersReportSurvivesACutShortTurnAfterIt(t *testing.T) {
	roster := defaultRoster()
	task := firstReview().
		turn("turn-4", "reviewer", "One is left.\n\nVerdict: changes requested").
		turn("turn-5", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Remove it."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "engineer", "instructions": "Remove it."}).
		cutShort("turn-6", "engineer", "the agent's process exited").
		turn("turn-7", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Review what is there."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "reviewer", "instructions": "Review what is there."}).task()

	brief := handoffBrief(task, agentNamed(t, roster, "reviewer"), roster, "Review what is there.", nil, twoCommits())
	want := "## The builder's last report\n\nFrom engineer, in turn-2:\n\nRemoved 3.\n\nIts latest turn, turn-6, was cut short: the agent's process exited\n"
	if !strings.Contains(brief, want) || strings.Contains(brief, "(It left no report.)") {
		t.Errorf("the handoff is missing %q:\n%s", want, brief)
	}

	shipped := firstReview().
		turn("turn-4", "deployer", "Rebased onto dev; the tests pass.").
		turn("turn-5", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Review it."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "reviewer", "instructions": "Review it."}).task()
	brief = handoffBrief(shipped, agentNamed(t, roster, "reviewer"), roster, "Review it.", nil, twoCommits())
	if want := "## The builder's last report\n\nFrom deployer, in turn-4:\n\nRebased onto dev; the tests pass."; !strings.Contains(brief, want) {
		t.Errorf("a shipper's report counts as the builder's; missing %q:\n%s", want, brief)
	}
}

// With no commits listed, nil or none, the handoff says so and still names
// the commands; with no builder's turn, it says that too.
func TestAHandoffWithNoCommitsNamesTheCommands(t *testing.T) {
	roster := defaultRoster()
	task := history{}.
		add(you, events.TaskCreated, "", map[string]any{"title": "Check the docs", "repo": "/r", "coordinator": "tech-lead"}).
		add(worker, events.WorktreeReady, "", map[string]any{"path": "/w/task-1", "branch": "agentry/task-1", "base": "b0b0b0b0", "port": 7901}).
		turn("turn-1", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Read the docs."}`).
		add(coordinator, events.TaskAssigned, "", map[string]any{"agent": "reviewer", "instructions": "Read the docs."}).task()
	for name, commits := range map[string][]workspace.Commit{"nil": nil, "none": {}} {
		brief := handoffBrief(task, agentNamed(t, roster, "reviewer"), roster, "Read the docs.", nil, commits)
		for _, want := range []string{"## What to review\n\nNothing is listed here: the branch has no commits since b0b0b0b0 yet, or Agentry couldn't read them.",
			"git log --oneline b0b0b0b0..HEAD", "git diff b0b0b0b0..HEAD\n",
			"## The builder's last report\n\nNo agent that builds the work has reported on this task yet."} {
			if !strings.Contains(brief, want) {
				t.Errorf("%s commits: the handoff is missing %q:\n%s", name, want, brief)
			}
		}
		if strings.Contains(brief, "The commit range is") || strings.Contains(brief, "It touches") {
			t.Errorf("%s commits: the handoff names a range:\n%s", name, brief)
		}
	}
}

// A long list of files is replaced by their count and the command that
// lists them.
func TestAHandoffCapsALongFileList(t *testing.T) {
	roster := defaultRoster()
	task := firstReview().task()
	wide := workspace.Commit{Hash: "c3c3c3c3c3c3c3c3", Subject: "chore: rename everything", At: day(1)}
	for i := range mostFilesListed + 1 {
		wide.Files = append(wide.Files, workspace.File{Path: fmt.Sprintf("src/f%02d.go", i), Change: workspace.Modified})
	}
	brief := handoffBrief(task, agentNamed(t, roster, "reviewer"), roster, "Review it.", nil, []workspace.Commit{wide})
	if want := fmt.Sprintf("It touches %d files; list them with `git diff --stat b0b0b0b0..HEAD`.", mostFilesListed+1); !strings.Contains(brief, want) {
		t.Errorf("the handoff is missing %q:\n%s", want, brief)
	}
	if strings.Contains(brief, "- M src/f00.go") {
		t.Errorf("the handoff lists the files one by one:\n%s", brief)
	}
	wide.Files = wide.Files[:mostFilesListed]
	if brief := handoffBrief(task, agentNamed(t, roster, "reviewer"), roster, "Review it.", nil, []workspace.Commit{wide}); !strings.Contains(brief, "- M src/f39.go") {
		t.Errorf("%d files are listed one by one:\n%s", mostFilesListed, brief)
	}
}

// The loop briefs a fresh reviewer with the handoff, a fresh engineer with
// the whole brief, and a reviewer set to resume with the resumed brief.
func TestTheLoopHandsAFreshReviewerTheHandoff(t *testing.T) {
	fakeClaudeOnPath(t)
	repo := gitRepo(t)
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	const id = "task-1"
	run(t, newLoop(s, state, t.TempDir()), state)
	propose(t, s, events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Remove the TODOs", "description": "Every TODO in src/ goes.", "repo": repo, "coordinator": "tech-lead"}})

	waitForTurn(t, s, id, "turn-1", "tech-lead")
	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "assign", "agent": "engineer", "instructions": "Remove every TODO."}`)
	task := waitForTurn(t, s, id, "turn-2", "engineer")
	if brief := readBrief(t, task, "turn-2"); strings.Contains(brief, "## What to review") || !strings.Contains(brief, "## The task") {
		t.Errorf("a fresh engineer gets the whole worker brief:\n%s", brief)
	}
	if err := os.WriteFile(filepath.Join(task.Worktree.Path, "src.go"), []byte("package src\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, task.Worktree.Path, "add", "src.go")
	commitIn(t, task.Worktree.Path)
	endTurn(t, s, id, "turn-2", "engineer", "Removed 3.")
	waitForTurn(t, s, id, "turn-3", "tech-lead")
	endTurn(t, s, id, "turn-3", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Review it."}`)
	task = waitForTurn(t, s, id, "turn-4", "reviewer")
	brief := readBrief(t, task, "turn-4")
	for _, want := range []string{"## What to review\n\nThe commit range is " + shortHash(task.Worktree.Base) + "..", "git log --oneline " + task.Worktree.Base + "..HEAD",
		"1 commit on the branch", "It touches 1 file:\n- A src.go\n",
		"## The builder's last report\n\nFrom engineer, in turn-2:\n\nRemoved 3.", "## Your instructions from the coordinator\n\nReview it."} {
		if !strings.Contains(brief, want) {
			t.Errorf("the fresh reviewer's brief is missing %q:\n%s", want, brief)
		}
	}
	if strings.Contains(brief, "engineer → tech-lead") || task.Turn("turn-4").FreshBecause != "reviewers start fresh" {
		t.Errorf("the fresh reviewer (fresh because %q) got the trail:\n%s", task.Turn("turn-4").FreshBecause, brief)
	}

	// Set to resume, the reviewer's next turn goes on with its session and
	// gets the resumed brief, not the handoff.
	reviewer, _ := state.Roster().Get("reviewer")
	reviewer.Resume = agents.ResumesSession
	propose(t, s, reviewer.Proposal(you, true))
	propose(t, s, events.Proposal{Stream: id, Actor: worker, Type: events.SessionStarted, Correlation: "turn-4", Data: map[string]any{"engineSession": "s-4"}})
	endTurn(t, s, id, "turn-4", "reviewer", "One is left.\n\nVerdict: changes needed")
	waitForTurn(t, s, id, "turn-5", "tech-lead")
	endTurn(t, s, id, "turn-5", "tech-lead", `{"action": "assign", "agent": "reviewer", "instructions": "Look again."}`)
	task = waitForTurn(t, s, id, "turn-6", "reviewer")
	if brief := readBrief(t, task, "turn-6"); !strings.Contains(brief, ", going on\n") || strings.Contains(brief, "## What to review") {
		t.Errorf("a reviewer set to resume gets the resumed brief:\n%s", brief)
	}
	if turn := task.Turn("turn-6"); turn.FromSession != "s-4" {
		t.Errorf("turn-6 went on with %q, want s-4", turn.FromSession)
	}
}
