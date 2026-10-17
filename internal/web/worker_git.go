package web

import (
	"io"
	"net/http"
	"net/http/cgi"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/tasks"
	"gitlab.com/dev.chempa/agentry/internal/workerlink"
	"gitlab.com/dev.chempa/agentry/internal/workers"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

// A task's repo, the hub its work moves through (package worktrees), as
// workers reach it: git can fetch from it, and the worker the work is on
// sends its branch back as a bundle. Nothing can push to it: a bundle only
// ever sets the task's own branch.

// gitFetch serves git's fetch over HTTP for the task's repo, with git's own
// http-backend, which never runs a push here.
func (s *Server) gitFetch(w http.ResponseWriter, r *http.Request, _ workers.Worker) {
	task, ok := s.hubTask(w, r)
	if !ok {
		return
	}
	if strings.Contains(r.URL.Path, "receive-pack") || r.URL.Query().Get("service") == "git-receive-pack" {
		http.Error(w, "workers send work back as a bundle, not a push", http.StatusForbidden)
		return
	}
	common, err := exec.Command("git", "-C", task.Repo, "rev-parse", "--path-format=absolute", "--git-common-dir").Output()
	if err != nil {
		http.Error(w, task.Repo+" is not a git repository", http.StatusNotFound)
		return
	}
	gitDir := strings.TrimSpace(string(common))
	git, err := exec.LookPath("git")
	if err != nil {
		failed(w, err)
		return
	}
	handler := &cgi.Handler{Path: git, Args: []string{"http-backend"}, Root: workerlink.GitPath + task.ID,
		Env: []string{"GIT_PROJECT_ROOT=" + filepath.Dir(gitDir), "GIT_HTTP_EXPORT_ALL=1"}}
	// http-backend finds the repo from the path: the root's folder, then the repo's.
	r2 := r.Clone(r.Context())
	r2.URL.Path = workerlink.GitPath + task.ID + "/" + filepath.Base(gitDir) + strings.TrimPrefix(r.URL.Path, workerlink.GitPath+task.ID)
	handler.ServeHTTP(w, r2)
}

// receiveBundle sets the task's branch in the hub to the worker's, once the
// work leaves it. Only the worker the work is on may.
func (s *Server) receiveBundle(w http.ResponseWriter, r *http.Request, worker workers.Worker) {
	task, ok := s.hubTask(w, r)
	if !ok {
		return
	}
	if task.Worktree.Worker != worker.Name {
		http.Error(w, "the task's work isn't on "+worker.Name, http.StatusForbidden)
		return
	}
	file, err := os.CreateTemp("", "agentry-*.bundle")
	if err != nil {
		failed(w, err)
		return
	}
	defer os.Remove(file.Name())
	_, err = io.Copy(file, http.MaxBytesReader(w, r.Body, 4<<30))
	file.Close()
	if err != nil {
		http.Error(w, "the work didn't arrive: "+err.Error(), http.StatusBadRequest)
		return
	}
	if err := worktrees.ReceiveBundle(task.Repo, task.Worktree.Branch, file.Name()); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	reply(w, map[string]string{"branch": task.Worktree.Branch})
}

func (s *Server) hubTask(w http.ResponseWriter, r *http.Request) (*tasks.Task, bool) {
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil || task.Worktree == nil {
		http.Error(w, "no such task, or it has no work yet", http.StatusNotFound)
		return nil, false
	}
	return task, true
}
