package web

import (
	"errors"
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/workers"
	"gitlab.com/dev.chempa/agentry/internal/workspace"
)

// The peek into a task's worktree: its files and what changed, and one
// file at a time. Read only.

// workspaceOf is the task's worktree and the machine it is on, which reads it.
func (s *Server) workspaceOf(w http.ResponseWriter, r *http.Request) (workspace.Workspace, machines.Machine, bool) {
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil {
		http.Error(w, "no such task", http.StatusNotFound)
		return workspace.Workspace{}, nil, false
	}
	if task.Worktree == nil {
		http.Error(w, "the task has no workspace yet", http.StatusNotFound)
		return workspace.Workspace{}, nil, false
	}
	machine := machines.Machine(&machines.Local{Label: workers.Local})
	if s.config.Fleet != nil {
		machine = s.config.Fleet.Of(task.Worktree.Worker)
	}
	return workspace.Workspace{Dir: task.Worktree.Path, Base: task.Worktree.Base}, machine, true
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	ws, machine, ok := s.workspaceOf(w, r)
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
	reply(w, map[string]any{"base": ws.Base, "files": files})
}

func (s *Server) readFile(w http.ResponseWriter, r *http.Request) {
	ws, machine, ok := s.workspaceOf(w, r)
	if !ok {
		return
	}
	contents, err := machine.Read(r.Context(), ws, r.URL.Query().Get("path"))
	switch {
	case errors.Is(err, workspace.ErrOutside):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case err != nil:
		failed(w, err)
	default:
		reply(w, contents)
	}
}
