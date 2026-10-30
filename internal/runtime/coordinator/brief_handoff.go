package coordinator

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/platform/workspace"
)

// lastReported is the agent's latest ended turn on the task that left a
// report; nil when none did. A turn cut short or stopped leaves none, and
// a handoff that quoted it would read as if the range had been checked.
func lastReported(task *tasks.Task, agent string) *tasks.Turn {
	for i := len(task.Turns) - 1; i >= 0; i-- {
		if turn := task.Turns[i]; turn.Agent == agent && reported(turn) {
			return turn
		}
	}
	return nil
}

// reported says whether the turn ended with a report.
func reported(turn *tasks.Turn) bool {
	return turn.Ended && strings.TrimSpace(turn.Report) != ""
}

// longestReport caps a report quoted whole in a handoff: a review needs
// all of a builder's report, a runaway one still shouldn't swamp the brief.
const longestReport = 3 * longestStep

// quoted is a report as a handoff quotes it: capped at longestReport, with
// its Markdown headings two levels down. Agents head their reports
// themselves ("## Report for the coordinator"), and quoted as they are,
// those read as sections of the brief, so where the report ends is lost.
// Headings inside fenced code are code and stay as they are.
func quoted(report string) string {
	lines := strings.Split(tasks.Shorten(report, longestReport), "\n")
	fenced := false
	for i, line := range lines {
		switch {
		case strings.HasPrefix(strings.TrimSpace(line), "```"):
			fenced = !fenced
		case !fenced && isHeading(line):
			lines[i] = strings.Repeat("#", min(headingLevel(line)+2, 6)) + line[headingLevel(line):]
		}
	}
	return strings.Join(lines, "\n")
}

// isHeading says whether the line is a Markdown heading: one to six "#"
// then a space, or nothing more.
func isHeading(line string) bool {
	level := headingLevel(line)
	return level >= 1 && level <= 6 && (len(line) == level || line[level] == ' ')
}

// headingLevel counts the "#" the line starts with.
func headingLevel(line string) int {
	return len(line) - len(strings.TrimLeft(line, "#"))
}

// mostFilesListed is how many touched files a handoff lists by name;
// beyond that it gives the count and the command that lists them.
const mostFilesListed = 40

// reviewText is the range to review, the files it touches and the git
// commands that show them. With no commits listed it says so and still
// gives the commands, so the agent can look for itself.
func reviewText(base string, commits []workspace.Commit) string {
	var b strings.Builder
	if len(commits) == 0 {
		b.WriteString("Nothing is listed here: the branch has no commits since " + shortHash(base) + " yet, or Agentry couldn't read them. " +
			"Check for yourself with the commands below; if there is nothing to review, say so in your report.\n\n")
	} else {
		fmt.Fprintf(&b, "The commit range is %s..%s: %s on the branch since it started.\n\n%s\n\n",
			shortHash(base), shortHash(commits[0].Hash), count(len(commits), "commit"), filesText(touched(commits), base))
	}
	b.WriteString("Read the change with git, in the worktree; this brief never carries the diff itself:\n\n" +
		"    git log --oneline " + base + "..HEAD\n" +
		"    git diff " + base + "..HEAD\n" +
		"    git diff " + base + "..HEAD -- <file>\n")
	return b.String()
}

// touched are the files the commits change, each once, sorted by path,
// with its change in the newest commit that touched it.
func touched(commits []workspace.Commit) []workspace.File {
	seen := map[string]bool{}
	var files []workspace.File
	for _, commit := range commits { // newest first
		for _, f := range commit.Files {
			if !seen[f.Path] {
				seen[f.Path] = true
				files = append(files, f)
			}
		}
	}
	slices.SortFunc(files, func(a, b workspace.File) int { return strings.Compare(a.Path, b.Path) })
	return files
}

// filesText lists the files by name while there are few enough to read;
// past that it gives the count and the command that lists them.
func filesText(files []workspace.File, base string) string {
	if len(files) > mostFilesListed {
		return fmt.Sprintf("It touches %s; list them with `git diff --stat %s..HEAD`.", count(len(files), "file"), base)
	}
	if len(files) == 0 {
		return "It touches no files."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "It touches %s:\n", count(len(files), "file"))
	for _, f := range files {
		fmt.Fprintf(&b, "- %s %s\n", cmp.Or(string(f.Change), "-"), f.Path)
	}
	return strings.TrimRight(b.String(), "\n")
}

// builderReport is the latest report on the task by an agent that builds or
// ships the work, with who wrote it; a note when there is none yet. When
// that agent's turn since was cut short, a line says so: the reviewer then
// knows the report may not be the last word on the branch.
func builderReport(task *tasks.Task, roster agents.Roster) string {
	for i := len(task.Turns) - 1; i >= 0; i-- {
		turn := task.Turns[i]
		agent, ok := roster.Get(turn.Agent)
		if !reported(turn) || !ok || agent.Coordinates || agent.Column == agents.Reviews {
			continue
		}
		text := "From " + turn.Agent + ", in " + turn.ID + ":\n\n" + quoted(turn.Report)
		if latest := task.LastTurnOf(turn.Agent); latest.ID != turn.ID && latest.Ended && latest.Issue != "" {
			text += "\n\nIts latest turn, " + latest.ID + ", was cut short: " + latest.Issue
		}
		return text
	}
	return "No agent that builds the work has reported on this task yet."
}

// changedSince is what the branch gained after the agent's last turn began,
// at since: the newer commits, the files they touch, and the command that
// shows them together. Commit times are git's committer dates, so after a
// rebase every commit looks new; that errs towards reviewing more.
func changedSince(commits []workspace.Commit, since time.Time) string {
	var newer []workspace.Commit
	for _, commit := range commits { // newest first
		if commit.At.After(since) {
			newer = append(newer, commit)
		}
	}
	if len(newer) == 0 {
		return "No commits since your last turn. The coordinator may still have asked you to check again; your instructions say what."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s since your last turn, newest first:\n", count(len(newer), "commit"))
	for _, commit := range newer {
		fmt.Fprintf(&b, "- %s %s\n", shortHash(commit.Hash), commit.Subject)
	}
	oldest := newer[len(newer)-1].Hash
	fmt.Fprintf(&b, "\n%s\n\nSee them together with `git diff %s^..HEAD`.", filesText(touched(newer), oldest+"^"), oldest)
	return b.String()
}

// shortHash is the hash as git shows it in a one-line log.
func shortHash(hash string) string {
	if len(hash) > 8 {
		return hash[:8]
	}
	return hash
}

// count is "1 commit" or "3 commits".
func count(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
