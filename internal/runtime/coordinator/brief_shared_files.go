package coordinator

import (
	"fmt"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

// sharedText is what the task's shared files are, and how they work.
func sharedText(task *tasks.Task, shared []sharedfiles.File, coordinator bool) string {
	var b strings.Builder
	b.WriteString("Files that aren't in git, such as data, builds, documents or images, are shared with the task in " +
		"the folder .agentry/files of this worktree (git ignores it). They follow the task to whichever machine works on it " +
		"next: what you add, change or remove there is kept when your turn ends, and the next agent finds it there. Put what " +
		"the next agent or the user needs there, and keep code in git.")
	if task.Epic != "" {
		b.WriteString(" Your epic's files are in .agentry/files/" + sharedfiles.EpicFolder + ", to read: changes there aren't kept.")
	}
	if coordinator {
		b.WriteString("\n\nTo bring a file or folder from a machine (one in \"Machines\", or this one, \"local\") into the " +
			"shared files, decide \"fetch\" with its machine and path: from inside that machine's worktrees it comes at once; " +
			"from anywhere else the user is asked first. You hear when it has come.")
	}
	if len(shared) == 0 {
		b.WriteString("\n\nThe task has no shared files yet.")
		return b.String()
	}
	b.WriteString("\n\nIts shared files now:\n")
	for _, f := range shared {
		by := ""
		if kept, ok := task.Files[f.Name]; ok {
			by = ", from " + kept.By
			if kept.From != "" {
				by = ", copied from " + kept.From
			}
		}
		fmt.Fprintf(&b, "- %s (%s%s)\n", f.Name, humanSize(f.Size), by)
	}
	return b.String()
}

func humanSize(n int64) string {
	switch {
	case n >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(n)/(1<<30))
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(n)/(1<<10))
	}
	return fmt.Sprintf("%d bytes", n)
}
