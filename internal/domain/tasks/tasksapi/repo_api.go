package tasksapi

import (
	"net/http"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/platform/worktrees"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
)

// changeRepo moves a task's work to another repo: when the turn in progress
// ends, or at once between turns. A finished task gets its worktree of the
// new repo here and now, its whole history counted as the task's work, so
// its Workspace shows what was done there.
func (a API) changeRepo(w http.ResponseWriter, r *http.Request) {
	var request struct{ Repo string }
	if !httpapi.Decode(w, r, &request) {
		return
	}
	repo, reason := a.RepoFor(strings.TrimSpace(request.Repo))
	if reason != "" {
		http.Error(w, reason, http.StatusBadRequest)
		return
	}
	task, err := a.State.Task(r.PathValue("task"))
	if err != nil || task == nil {
		httpapi.NoTask(w, a.State, r.PathValue("task"))
		return
	}
	moved := events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.RepoChanged, Data: map[string]any{"repo": repo}}
	if !task.IsDone || a.Fleet == nil {
		httpapi.Propose(w, a.Log, moved)
		return
	}
	if !httpapi.ProposeOrRefuse(w, a.Log, moved) {
		return
	}
	here := a.Fleet.Of(workers.Local)
	made, err := here.Prepare(r.Context(), machines.WorktreeRequest{Task: task.ID, Repo: repo})
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	httpapi.Propose(w, a.Log, events.Proposal{Stream: task.ID, Actor: events.Actor{Kind: events.Worker, ID: here.Name()}, Type: events.WorktreeReady,
		Data: map[string]any{"repo": made.Repo, "worker": here.Name(), "path": made.Path, "branch": made.Branch, "base": "", "port": made.Port}})
}

// RepoFor is the folder a task's work goes in, for a repo named by its
// folder on this machine or by a git address, which is cloned here; or why
// it can't be worked in.
func (a API) RepoFor(repo string) (string, string) {
	if worktrees.IsAddress(repo) {
		folder, err := worktrees.Clone(a.Clones, repo)
		if err != nil {
			return "", err.Error()
		}
		return folder, ""
	}
	return repo, worktrees.IsRepo(repo)
}
