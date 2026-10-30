package coordinator

import (
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

func taskText(task *tasks.Task) string {
	if task.Description == "" {
		return "Title: " + task.Title
	}
	return "Title: " + task.Title + "\n\n" + task.Description
}

func whereText(task *tasks.Task) string {
	w := task.Worktree
	return fmt.Sprintf("You are in the task's own git worktree, %s, on branch %s, which started from commit %s. "+
		"Other tasks have their own worktrees. If you run a server, use port %d (also in the PORT environment variable).\n\n"+
		"All of the task's work happens here, in this worktree, and is committed on its branch. The next agent may work "+
		"on another machine: only what is committed on this branch goes there with the task, so never create or change "+
		"repos or folders outside the worktree. If the user asks for the work to be in another repo, a folder on this machine, "+
		"a git address such as git@github.com:team/app.git, or a link to a .zip or .tar.gz of it, the coordinator moves the task there with a \"move\" decision, "+
		"and Agentry clones or downloads it if it needs to; for any other place, such as a new repo at some path, the coordinator asks the "+
		"user to move the task (the task's Details have Change repo, which can make a new repo). No one works outside the "+
		"worktree.\n\n"+
		"To show the user a document, a design or a diagram, write it as a file in the worktree (Markdown, with SVG "+
		"diagrams beside it), commit it, and link it where they read you: in your report, a reply or a summary, as a "+
		"Markdown link to its path from the worktree's root, such as [the design](docs/design.md). They open it from the "+
		"chat straight into the task's Workspace, rendered, with its diagrams.",
		w.Path, w.Branch, w.Base, w.Port)
}
