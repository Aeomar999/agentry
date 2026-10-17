// Package machines is where agents work: the machine Agentry runs on, or a
// worker that has joined. Everything Agentry does to a task's worktree, its
// session or its terminal goes through a Machine, so the coordinator loop,
// the watchdog and the page never care which machine it is.
package machines

import (
	"context"

	"gitlab.com/dev.chempa/agentry/internal/terminal"
	"gitlab.com/dev.chempa/agentry/internal/workspace"
	"gitlab.com/dev.chempa/agentry/internal/worktrees"
)

type Machine interface {
	Name() string // workers.Local, or the worker's name

	// Prepare makes the task's worktree on this machine: from the repo on
	// the first visit, or from the hub's copy of the task's branch.
	Prepare(ctx context.Context, request WorktreeRequest) (worktrees.Worktree, error)
	// HandOff gets the work ready to leave: what was left uncommitted is
	// committed, the hub gets the branch, and this machine's worktree goes.
	HandOff(ctx context.Context, w worktrees.Worktree, message string) error
	// Remove takes the worktree away once its branch is merged (worktrees.ErrNotMerged otherwise).
	Remove(ctx context.Context, w worktrees.Worktree) error

	// PrepareTurn writes a turn's files and the engine's hooks; StartTurn
	// starts its session. Between them, the turn is recorded as started, so
	// the session's first hook call finds it open.
	PrepareTurn(ctx context.Context, turn Turn) error
	StartTurn(ctx context.Context, turn Turn) error
	Stop(task string)           // ends the task's session
	IsRunning(task string) bool // the task's session exists and its process is alive

	// For the page's peek into the worktree.
	Files(ctx context.Context, w workspace.Workspace) ([]workspace.File, error)
	Read(ctx context.Context, w workspace.Workspace, path string) (workspace.Contents, error)
	// Attach watches the task's session, for the page's terminal.
	Attach(task string, onOutput func([]byte)) (terminal.Pane, error)
	// About reads the machine's details now, for the Workers page.
	About(ctx context.Context) (About, error)
}

// WorktreeRequest is what a machine needs to make a task's worktree.
type WorktreeRequest struct {
	Task  string `json:"task"`
	Repo  string `json:"repo"`  // the repo the task works on, on the machine Agentry runs on: the hub
	Base  string `json:"base"`  // where the task started; "" on the first visit
	Ports []int  `json:"ports"` // taken on this machine by other tasks

	// For a worker: the repo's own remote, so agents there push where they
	// would here, and a key naming the repo, for its copy of it.
	Origin  string `json:"origin"`
	RepoKey string `json:"repoKey"`
}

// Turn is one agent turn, as a machine starts it.
type Turn struct {
	Task      string            `json:"task"`
	ID        string            `json:"id"`
	Worktree  string            `json:"worktree"` // the task's worktree on this machine
	Engine    string            `json:"engine"`
	Model     string            `json:"model"`
	Theme     string            `json:"theme"`
	Brief     string            `json:"brief"`     // what the agent is to do, written to BriefFile
	BriefFile string            `json:"briefFile"` // relative to the worktree
	Knowledge string            `json:"knowledge"` // what the agent knows, written to its knowledge file
	Prompt    string            `json:"prompt"`    // the one line the engine starts on
	Env       map[string]string `json:"env"`       // the session's environment; the machine adds where its hooks post
}
