// Package sharedfilesapi is a task's shared files over HTTP and MCP: on the
// page, from the desk and from Claude Code you can add one, download any,
// and take one away. Web mounts its Routes and agentry mcp lists its Tools
// as they do an extension's. The workers' blob routes stay in web, as no
// extensions.Auth takes a worker's credential.
package sharedfilesapi

import (
	"errors"
	"io"
	"net/http"
	"path"
	"strconv"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/current"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// State is what the routes read: the task, and when it was erased.
type State interface {
	Task(id string) (*tasks.Task, error)
	Erased(id string) (tasks.Erasure, bool)
}

// API holds what the shared files' routes need: the log to write their
// changes to, the state to read the task from, and where the files are kept.
type API struct {
	Log   extensions.Proposer
	State State
	Blobs sharedfiles.Disk
}

// Routes are the shared files' endpoints, for the page, and for the desk
// too except taking one away.
func (a API) Routes() []extensions.Route {
	return []extensions.Route{
		{Pattern: "GET /api/tasks/{task}/shared", Auth: extensions.Desk, Handler: a.listShared},
		{Pattern: "GET /api/tasks/{task}/shared/file", Auth: extensions.Desk, Handler: a.downloadShared},
		{Pattern: "POST /api/tasks/{task}/shared", Auth: extensions.Desk, Handler: a.uploadShared},
		{Pattern: "DELETE /api/tasks/{task}/shared", Auth: extensions.UI, Handler: a.removeShared},
	}
}

// File is one of a task's shared files, as the page shows it in the list
// and in the task's detail.
type File struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	By   string `json:"by"`
	At   string `json:"at"`
	From string `json:"from,omitempty"` // machine:path, when it was fetched
}

// FilesOf are the task's shared files as the page shows them, [] for none.
func FilesOf(t *tasks.Task) []File {
	list := []File{}
	for _, f := range t.SharedFiles() {
		list = append(list, File{Name: f.Name, Size: f.Size, By: f.By, At: f.At.Format(time.RFC3339), From: f.From})
	}
	return list
}

func (a API) listShared(w http.ResponseWriter, r *http.Request) {
	task, err := a.State.Task(r.PathValue("task"))
	if err != nil || task == nil {
		a.noTask(w, r.PathValue("task"))
		return
	}
	httpapi.Reply(w, FilesOf(task))
}

// uploadShared shares the request's body with the task as ?name=.
func (a API) uploadShared(w http.ResponseWriter, r *http.Request) {
	task, err := a.State.Task(r.PathValue("task"))
	if err != nil || task == nil {
		a.noTask(w, r.PathValue("task"))
		return
	}
	name, err := sharedfiles.CleanName(r.URL.Query().Get("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	hash, size, err := a.Blobs.Put(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpapi.Propose(w, a.Log, events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.FileAdded,
		Data: map[string]any{"name": name, "hash": hash, "size": size, "by": "you"}})
}

func (a API) downloadShared(w http.ResponseWriter, r *http.Request) {
	task, err := a.State.Task(r.PathValue("task"))
	if err != nil || task == nil {
		a.noTask(w, r.PathValue("task"))
		return
	}
	f, ok := task.Files[r.URL.Query().Get("name")]
	if !ok {
		http.Error(w, "the task has no shared file "+r.URL.Query().Get("name"), http.StatusNotFound)
		return
	}
	body, err := a.Blobs.Open(f.Hash)
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", strconv.FormatInt(f.Size, 10))
	w.Header().Set("Content-Disposition", `attachment; filename="`+path.Base(f.Name)+`"`)
	io.Copy(w, body)
}

func (a API) removeShared(w http.ResponseWriter, r *http.Request) {
	if a.isNone(r.PathValue("task")) {
		a.noTask(w, r.PathValue("task"))
		return
	}
	httpapi.Propose(w, a.Log, events.Proposal{Stream: r.PathValue("task"), Actor: httpapi.You, Type: events.FileRemoved,
		Data: map[string]any{"name": r.URL.Query().Get("name"), "by": "you"}})
}

// isNone is whether there is no task id, never made or erased, as web's
// isNone says.
func (a API) isNone(id string) bool {
	task, err := a.State.Task(id)
	return errors.Is(err, current.ErrErased) || err == nil && task == nil
}

// noTask answers as tasksapi's noTask does: that an erased task is gone for
// good, and when it went, 410, rather than that there was never such a
// task, 404.
func (a API) noTask(w http.ResponseWriter, id string) {
	if erasure, erased := a.State.Erased(id); erased {
		http.Error(w, id+" was erased on "+erasure.At.Format("2 January 2006"), http.StatusGone)
		return
	}
	http.Error(w, "no such task "+id, http.StatusNotFound)
}
