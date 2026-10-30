package tasks

import (
	"cmp"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
)

func (t *Task) applyWorktree(e events.Event) {
	switch e.Type {
	case events.RepoChanged:
		// Its worktree is made again, of the new repo, before the next turn:
		// at once between turns, or when the turn in progress ends.
		repo, when := e.Text("repo"), " (it moves when the turn in progress ends)"
		t.MovingTo = repo
		if t.OpenTurn() == nil {
			t.moveRepo()
			when = ""
		}
		from, moved := You, "I moved this task"
		if e.Actor.Kind == events.Agent { // the coordinator, to the repo you named
			from, moved = t.Coordinator, "You moved this task, as the user asked,"
		}
		t.step(e, from, t.Coordinator, moved+" to the repo at "+repo+when+". From now on its work goes there: "+
			"the task has a new worktree of that repo, on its own branch, and every agent works in it.")

	case events.WorktreeReady:
		t.Worktree = &Worktree{Repo: e.Text("repo"), Worker: cmp.Or(e.Text("worker"), workers.Local), Path: e.Text("path"),
			Branch: e.Text("branch"), Base: e.Text("base"), Port: e.Number("port")}

	case events.WorktreeMoved:
		t.Worktree.Worker, t.Worktree.Path, t.Worktree.Port = e.Text("worker"), e.Text("path"), e.Number("port")
	}
}

// moveRepo makes the move you asked for: the next turn works in a new
// worktree, of the new repo.
func (t *Task) moveRepo() {
	if t.MovingTo != "" {
		t.Repo, t.Worktree, t.MovingTo = t.MovingTo, nil, ""
	}
}
