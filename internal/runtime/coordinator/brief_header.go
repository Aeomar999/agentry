package coordinator

import (
	"fmt"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
)

func header(b *strings.Builder, agent agents.Agent, task *tasks.Task) {
	fmt.Fprintf(b, "# Agentry brief: %s, %s, %s\n\n%s\n\n", agent.Name, task.ID, task.NextTurnID(), agent.Prompt)
}

// resumedHeader tells the agent this turn goes on from its last one on the
// task, in the same session.
func resumedHeader(b *strings.Builder, agent agents.Agent, task *tasks.Task) {
	fmt.Fprintf(b, "# Agentry brief: %s, %s, %s, going on\n\n", agent.Name, task.ID, task.NextTurnID())
	b.WriteString("This turn goes on from your last turn on this task, in the same session. You still have what you knew " +
		"then: your role, your notes, the task, its earlier trail, the files you read and the work you did. This brief is " +
		"only what has happened since and what this turn needs.\n\n")
}

// goingOn begins a resumed brief and is the heading of its trail. A session
// from another story, from, knows that story's task, worktree and trail,
// not this one's: so it says this is a new task and what it is, and the
// trail, from its start (seen is 0), is this task's own.
func goingOn(b *strings.Builder, agent agents.Agent, task *tasks.Task, from string) (trailHeading string) {
	if from == "" || from == task.ID {
		resumedHeader(b, agent, task)
		return "What has happened since your last turn"
	}
	fmt.Fprintf(b, "# Agentry brief: %s, %s, %s, going on from %s\n\n", agent.Name, task.ID, task.NextTurnID(), from)
	fmt.Fprintf(b, "This turn goes on with your session from %s, another story of the same epic. You still have what you knew "+
		"there: your role, your notes, and the code you read. But this is a new task, %s, with its own worktree and branch, "+
		"below. The files you read in that session may differ on this branch, so read them again before you edit them. "+
		"Nothing of %s's trail or instructions carries over: this brief is what this task is and what has happened on it.\n\n",
		from, task.ID, from)
	section(b, "The task", taskText(task))
	return "What has happened on this task so far"
}
