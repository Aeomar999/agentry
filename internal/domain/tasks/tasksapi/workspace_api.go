package tasksapi

import (
	"errors"
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/platform/workspace"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
)

// A task's Workspace page, as an editor's git view shows it: its
// files with their git status, the branch's commits since the task
// started, and one file at a time, now or in a commit (?commit=). Read only.

// workspaceOf is the task's worktree and the machine it is on, which reads it.
func (a API) workspaceOf(w http.ResponseWriter, r *http.Request) (workspace.Workspace, machines.Machine, bool) {
	task, err := a.State.Task(r.PathValue("task"))
	if err != nil || task == nil {
		httpapi.NoTask(w, a.State, r.PathValue("task"))
		return workspace.Workspace{}, nil, false
	}
	if task.Worktree == nil {
		http.Error(w, "the task has no workspace yet", http.StatusNotFound)
		return workspace.Workspace{}, nil, false
	}
	machine := machines.Machine(&machines.Local{Label: workers.Local})
	if a.Fleet != nil {
		machine = a.Fleet.Of(task.Worktree.Worker)
	}
	return workspace.Workspace{Dir: task.Worktree.Path, Base: task.Worktree.Base, Commit: r.URL.Query().Get("commit")}, machine, true
}

func (a API) listFiles(w http.ResponseWriter, r *http.Request) {
	ws, machine, ok := a.workspaceOf(w, r)
	if !ok {
		return
	}
	files, err := machine.Files(r.Context(), ws)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound) // most often: removed once the task was merged
		return
	}
	if files == nil {
		files = []workspace.File{}
	}
	httpapi.Reply(w, map[string]any{"base": ws.Base, "files": files})
}

func (a API) listCommits(w http.ResponseWriter, r *http.Request) {
	ws, machine, ok := a.workspaceOf(w, r)
	if !ok {
		return
	}
	commits, err := machine.Commits(r.Context(), ws)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	if commits == nil {
		commits = []workspace.Commit{}
	}
	httpapi.Reply(w, commits)
}

func (a API) readFile(w http.ResponseWriter, r *http.Request) {
	ws, machine, ok := a.workspaceOf(w, r)
	if !ok {
		return
	}
	contents, err := machine.Read(r.Context(), ws, r.URL.Query().Get("path"))
	switch {
	case errors.Is(err, workspace.ErrOutside):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case err != nil:
		httpapi.Failed(w, err)
	default:
		httpapi.Reply(w, contents)
	}
}
