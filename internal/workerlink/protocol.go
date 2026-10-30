package workerlink

import (
	"gitlab.com/dev.chempa/agentry/internal/platform/workspace"
	"gitlab.com/dev.chempa/agentry/internal/platform/worktrees"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
)

// The server's requests, each a machines.Machine method of the same name.
const (
	methodPrepare     = "prepare"     // machines.WorktreeRequest → worktrees.Worktree
	methodHandOff     = "handOff"     // handOff → nothing
	methodRemove      = "remove"      // worktrees.Worktree → nothing
	methodPrepareTurn = "prepareTurn" // machines.Turn → nothing
	methodStartTurn   = "startTurn"   // machines.Turn → nothing
	methodStop        = "stop"        // taskOnly → nothing
	methodFiles       = "files"       // workspace.Workspace → []workspace.File
	methodCommits     = "commits"     // workspace.Workspace → []workspace.Commit
	methodStartHelper = "startHelper" // machines.Helper → nothing
	methodRead        = "read"        // read → workspace.Contents
	methodAttach      = "attach"      // attach → size; the worker dials the stream first
	methodAbout       = "about"       // nothing → machines.About

	// A task's shared files; the worker sends and gets them over HTTP
	// (sharedfiles.Hub), and these say which.
	methodSharedManifest = "sharedManifest" // sharedIn → []sharedfiles.File
	methodSharedDeliver  = "sharedDeliver"  // sharedIn → nothing
	methodSharedUpload   = "sharedUpload"   // sharedIn → []sharedfiles.File
	methodSharedFetch    = "sharedFetch"    // sharedFetch → []sharedfiles.File
)

// The worker's notices.
const (
	noticeHello  = "hello"  // workers.Machine, first, once per connection
	noticeStatus = "status" // status, every StatusEvery
)

type handOff struct {
	Worktree worktrees.Worktree `json:"worktree"`
	Message  string             `json:"message"` // for the commit of what was left uncommitted
}

type taskOnly struct {
	Task string `json:"task"`
}

type sharedIn struct {
	Worktree string             `json:"worktree"`
	Files    []sharedfiles.File `json:"files,omitempty"` // for deliver
	Names    []string           `json:"names,omitempty"` // for upload
}

type sharedFetch struct {
	Path     string `json:"path"`
	As       string `json:"as"`
	Anywhere bool   `json:"anywhere"`
}

type read struct {
	Workspace workspace.Workspace `json:"workspace"`
	Path      string              `json:"path"`
}

type attach struct {
	Task   string `json:"task"`
	Stream string `json:"stream"` // the id the worker dials the stream with
}

type size struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

// status is which tasks have a live agent process on the worker.
type status struct {
	Running []string `json:"running"`
}

// The paths the worker uses, under the server's address.
const (
	ConnectPath = "/api/worker/connect"
	LeavePath   = "/api/worker/leave"    // POST: the worker takes itself off the server
	StreamPath  = "/api/worker/streams/" // + the stream's id
)
