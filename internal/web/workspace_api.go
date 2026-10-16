package web

import (
	"errors"
	"net/http"

	"gitlab.com/dev.chempa/agentry/internal/workspace"
)

// The peek into a task's worktree: its files and what changed, and one
// file at a time. Read only.

func (s *Server) workspaceOf(w http.ResponseWriter, r *http.Request) (workspace.Workspace, bool) {
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil {
		http.Error(w, "no such task", http.StatusNotFound)
		return workspace.Workspace{}, false
	}
	if task.Worktree == nil {
		http.Error(w, "the task has no workspace yet", http.StatusNotFound)
		return workspace.Workspace{}, false
	}
	return workspace.Workspace{Dir: task.Worktree.Path, Base: task.Worktree.Base}, true
}

func (s *Server) listFiles(w http.ResponseWriter, r *http.Request) {
	ws, ok := s.workspaceOf(w, r)
	if !ok {
		return
	}
	files, err := ws.Files()
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
	ws, ok := s.workspaceOf(w, r)
	if !ok {
		return
	}
	contents, err := ws.Read(r.URL.Query().Get("path"))
	switch {
	case errors.Is(err, workspace.ErrOutside):
		http.Error(w, err.Error(), http.StatusBadRequest)
	case err != nil:
		failed(w, err)
	default:
		reply(w, contents)
	}
}
