package coordinator

import (
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// A task that names its repo by a git address: the coordinator moves it
// there, Agentry clones it, and the work goes on in a worktree of the clone.
// A repo the user didn't name, the coordinator can't move it to, and one
// that can't be cloned, the coordinator hears why.
func TestTheCoordinatorMovesATaskToTheRepoYouNamed(t *testing.T) {
	fakeClaudeOnPath(t)
	root := t.TempDir()
	s, state := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	for _, p := range agents.SeedProposals(nil) {
		s.Propose(p)
	}
	const id = "task-1"
	loop := newLoop(s, state, root)
	loop.Clones = filepath.Join(root, ".repos")
	remote := gitRepo(t)
	address := "file://" + filepath.ToSlash(remote)
	missing := "file://" + filepath.ToSlash(filepath.Join(t.TempDir(), "gone.git"))
	repo := gitRepo(t)
	run(t, loop, state)

	s.Propose(events.Proposal{Stream: id, Actor: you, Type: events.TaskCreated, Data: map[string]any{
		"title": "Make it installable", "description": "Clone " + missing + ", or else " + address + ", and make the app a PWA.",
		"repo": repo, "coordinator": "tech-lead"}})
	waitForTurn(t, s, id, "turn-1", "tech-lead")

	endTurn(t, s, id, "turn-1", "tech-lead", `{"action": "move", "repo": "`+filepath.ToSlash(t.TempDir())+`"}`)
	waitUntil(t, s, id, "the refusal", func(task *tasks.Task) bool { return said(task, "only to a repo the user named") })
	task := waitForTurn(t, s, id, "turn-2", "tech-lead")
	if filepath.Dir(task.Repo) == loop.Clones {
		t.Errorf("a refused move shouldn't move it: %s", task.Repo)
	}

	endTurn(t, s, id, "turn-2", "tech-lead", `{"action": "move", "repo": "`+missing+`"}`)
	waitUntil(t, s, id, "the failed clone", func(task *tasks.Task) bool { return said(task, "couldn't move to "+missing+": ") })
	task = waitForTurn(t, s, id, "turn-3", "tech-lead")
	if filepath.Dir(task.Repo) == loop.Clones {
		t.Errorf("a failed clone shouldn't move it: %s", task.Repo)
	}

	endTurn(t, s, id, "turn-3", "tech-lead", `{"action": "move", "repo": "`+address+`"}`)
	task = waitForTurn(t, s, id, "turn-4", "tech-lead")
	if filepath.Dir(task.Repo) != loop.Clones || !strings.HasPrefix(task.Worktree.Repo, loop.Clones) {
		t.Errorf("the task should work in a clone of %s: repo %s, worktree %+v", address, task.Repo, task.Worktree)
	}
	if brief := readBrief(t, task, "turn-4"); !strings.Contains(brief, "You moved this task, as the user asked,") {
		t.Errorf("the coordinator should hear it moved:\n%s", brief)
	}
}

func said(task *tasks.Task, text string) bool {
	for _, step := range task.Trail {
		if strings.Contains(step.Text, text) {
			return true
		}
	}
	return false
}

// The user names a repo whole: one that only starts the same, as a shorter
// address or the folder above, isn't one they named.
func TestARepoIsNamedOnlyWhole(t *testing.T) {
	for _, c := range []struct {
		said, repo string
		named      bool
	}{
		{"Work in https://github.com/team/app-private.git please", "https://github.com/team/app", false},
		{"Work in https://github.com/team/app-private.git please", "https://github.com/team/app-private.git", true},
		{"It is in /home/u/projects/app-v2.", "/home/u/projects/app", false},
		{"It is in /home/u/projects/app.v2 now", "/home/u/projects/app", false},
		{"It is in /home/u/projects/app-v2.", "/home/u/projects/app-v2", true},
		{"Clone `git@github.com:team/app.git` first", "git@github.com:team/app.git", true},
		{`Clone "git@github.com:team/app.git" first`, "git@github.com:team/app.git", true},
		{"Clone 'git@github.com:team/app.git' first", "git@github.com:team/app.git", true},
		{"Clone <https://github.com/team/app.git>, then (https://x.io/a.zip).", "https://x.io/a.zip", true},
		{"Use /srv/app; then test", "/srv/app", true},
		{"/srv/app", "/srv/app", true},
		{"Use x/srv/app", "/srv/app", false},
		{"Use /srv/app", "", false},
	} {
		task := &tasks.Task{Title: "A task", Trail: []tasks.Step{{From: tasks.You, Text: c.said}}}
		if got := namedByYou(task, c.repo); got != c.named {
			t.Errorf("%q named in %q: %v, want %v", c.repo, c.said, got, c.named)
		}
	}
}
