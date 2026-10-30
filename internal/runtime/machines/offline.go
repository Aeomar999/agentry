package machines

import (
	"context"
	"fmt"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/platform/terminal"
	"gitlab.com/dev.chempa/agentry/internal/platform/workspace"
	"gitlab.com/dev.chempa/agentry/internal/platform/worktrees"
)

// Offline is a worker that isn't connected. Everything fails with why,
// except IsRunning: for OfflineGrace after it went, what ran there is taken
// to run still; after that, nothing runs there that Agentry can see.
type Offline struct {
	Worker string
	Since  time.Time // when it went offline; zero if it hasn't been online since Agentry started
}

func (o Offline) Name() string { return o.Worker }

func (o Offline) err() error { return fmt.Errorf("%s is offline", o.Worker) }

func (o Offline) Prepare(context.Context, WorktreeRequest) (worktrees.Worktree, error) {
	return worktrees.Worktree{}, o.err()
}
func (o Offline) HandOff(context.Context, worktrees.Worktree, string) error { return o.err() }
func (o Offline) Remove(context.Context, worktrees.Worktree) error          { return o.err() }
func (o Offline) PrepareTurn(context.Context, Turn) error                   { return o.err() }
func (o Offline) StartTurn(context.Context, Turn) error                     { return o.err() }
func (o Offline) Stop(string)                                               {}
func (o Offline) IsRunning(string) bool                                     { return time.Since(o.Since) < OfflineGrace }
func (o Offline) Files(context.Context, workspace.Workspace) ([]workspace.File, error) {
	return nil, o.err()
}
func (o Offline) Commits(context.Context, workspace.Workspace) ([]workspace.Commit, error) {
	return nil, o.err()
}
func (o Offline) StartHelper(context.Context, Helper) error { return o.err() }
func (o Offline) Read(context.Context, workspace.Workspace, string) (workspace.Contents, error) {
	return workspace.Contents{}, o.err()
}
func (o Offline) Attach(string, func([]byte)) (terminal.Pane, error) { return nil, o.err() }

func (o Offline) SharedManifest(context.Context, string) ([]sharedfiles.File, error) {
	return nil, o.err()
}
func (o Offline) SharedDeliver(context.Context, string, []sharedfiles.File) error { return o.err() }
func (o Offline) SharedUpload(context.Context, string, []string) ([]sharedfiles.File, error) {
	return nil, o.err()
}
func (o Offline) SharedFetch(context.Context, string, string, bool) ([]sharedfiles.File, error) {
	return nil, o.err()
}
func (o Offline) About(context.Context) (About, error) { return About{}, o.err() }
