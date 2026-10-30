package web

import (
	"io"
	"net/http"
	"path"
	"strconv"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
	"gitlab.com/dev.chempa/agentry/internal/core/httpapi"
	"gitlab.com/dev.chempa/agentry/internal/domain/workers"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
)

// A task's shared files on the page, from Claude Code, and to the workers
// (package sharedfiles). You can add one, download any, and take one away.

type sharedFile struct {
	Name string `json:"name"`
	Size int64  `json:"size"`
	By   string `json:"by"`
	At   string `json:"at"`
	From string `json:"from,omitempty"` // machine:path, when it was fetched
}

func (s *Server) listShared(w http.ResponseWriter, r *http.Request) {
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil {
		s.noTask(w, r.PathValue("task"))
		return
	}
	list := []sharedFile{}
	for _, f := range task.SharedFiles() {
		list = append(list, sharedFile{Name: f.Name, Size: f.Size, By: f.By, At: f.At.Format(time.RFC3339), From: f.From})
	}
	httpapi.Reply(w, list)
}

// uploadShared shares the request's body with the task as ?name=.
func (s *Server) uploadShared(w http.ResponseWriter, r *http.Request) {
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil {
		s.noTask(w, r.PathValue("task"))
		return
	}
	name, err := sharedfiles.CleanName(r.URL.Query().Get("name"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	hash, size, err := s.config.Blobs.Put(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpapi.Propose(w, s.store, events.Proposal{Stream: task.ID, Actor: httpapi.You, Type: events.FileAdded,
		Data: map[string]any{"name": name, "hash": hash, "size": size, "by": "you"}})
}

func (s *Server) downloadShared(w http.ResponseWriter, r *http.Request) {
	task, err := s.task(r.PathValue("task"))
	if err != nil || task == nil {
		s.noTask(w, r.PathValue("task"))
		return
	}
	f, ok := task.Files[r.URL.Query().Get("name")]
	if !ok {
		http.Error(w, "the task has no shared file "+r.URL.Query().Get("name"), http.StatusNotFound)
		return
	}
	body, err := s.config.Blobs.Open(f.Hash)
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

func (s *Server) removeShared(w http.ResponseWriter, r *http.Request) {
	if s.isNone(r.PathValue("task")) {
		s.noTask(w, r.PathValue("task"))
		return
	}
	httpapi.Propose(w, s.store, events.Proposal{Stream: r.PathValue("task"), Actor: httpapi.You, Type: events.FileRemoved,
		Data: map[string]any{"name": r.URL.Query().Get("name"), "by": "you"}})
}

// blobFor sends a worker a file it needs, by its hash.
func (s *Server) blobFor(w http.ResponseWriter, r *http.Request, _ workers.Worker) {
	if !s.config.Blobs.Has(r.PathValue("hash")) {
		http.Error(w, "no such file", http.StatusNotFound)
		return
	}
	if r.Method == http.MethodHead {
		return
	}
	body, err := s.config.Blobs.Open(r.PathValue("hash"))
	if err != nil {
		httpapi.Failed(w, err)
		return
	}
	defer body.Close()
	w.Header().Set("Content-Type", "application/octet-stream")
	io.Copy(w, body)
}

// blobFrom keeps a file a worker sends, and says its hash.
func (s *Server) blobFrom(w http.ResponseWriter, r *http.Request, _ workers.Worker) {
	hash, size, err := s.config.Blobs.Put(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	httpapi.Reply(w, map[string]any{"hash": hash, "size": size})
}
