package coordinator

import (
	"fmt"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// A brief is everything an agent is given for one turn: its own prompt, then
// what Agentry knows about the task. Each turn is a fresh session, so the
// brief is all the agent knows besides the repository.

// longestStep caps each trail entry in the coordinator's brief.
const longestStep = 4000

func coordinatorBrief(task *tasks.Task, coordinator agents.Agent, roster agents.Roster) string {
	var b strings.Builder
	header(&b, coordinator, task)
	section(&b, "The task", taskText(task))
	section(&b, "Where the work is", whereText(task))

	var team strings.Builder
	for _, agent := range roster.Workers() {
		fmt.Fprintf(&team, "- %s: %s\n", agent.Name, agent.Description)
	}
	section(&b, "The agents you can hand work to", team.String())

	var trail strings.Builder
	for i, step := range task.Trail {
		fmt.Fprintf(&trail, "%d. %s → %s: %s\n\n", i+1, step.From, step.To, shorten(step.Text, longestStep))
	}
	section(&b, "What has happened so far", trail.String()+"The last step is the newest. Decide what happens next.")

	if refused := refusedActions(task); refused != "" {
		section(&b, "Actions Agentry refused", refused)
	}
	section(&b, "Your decision", DecisionFormat)
	return b.String()
}

func workerBrief(task *tasks.Task, agent agents.Agent, instructions string) string {
	var b strings.Builder
	header(&b, agent, task)
	section(&b, "The task", taskText(task))
	section(&b, "Your instructions from the coordinator", instructions)
	section(&b, "Where the work is", whereText(task))
	section(&b, "When you finish",
		"End with a short report for the coordinator: what you did, what you found, and anything left or blocking you. "+
			"The coordinator reads only this report, so make it complete. Don't stop to ask questions: put them in the report.")
	return b.String()
}

func header(b *strings.Builder, agent agents.Agent, task *tasks.Task) {
	fmt.Fprintf(b, "# Agentry brief: %s, %s, %s\n\n%s\n\n", agent.Name, task.ID, task.NextTurnID(), agent.Prompt)
}

func section(b *strings.Builder, title, body string) {
	fmt.Fprintf(b, "## %s\n\n%s\n\n", title, strings.TrimSpace(body))
}

func taskText(task *tasks.Task) string {
	if task.Description == "" {
		return "Title: " + task.Title
	}
	return "Title: " + task.Title + "\n\n" + task.Description
}

func whereText(task *tasks.Task) string {
	w := task.Worktree
	return fmt.Sprintf("You are in the task's own git worktree, %s, on branch %s, which started from commit %s. "+
		"Other tasks have their own worktrees. If you run a server, use port %d (also in the PORT environment variable).",
		w.Path, w.Branch, w.Base, w.Port)
}

func refusedActions(task *tasks.Task) string {
	var refused strings.Builder
	for _, action := range task.Actions {
		if action.Decision == tasks.Denied {
			fmt.Fprintf(&refused, "- %s (%s, %s): `%s`: %s\n", action.Agent, action.Turn, action.Tool, shorten(action.Command, 200), action.Reason)
		}
	}
	return refused.String()
}

func shorten(text string, limit int) string {
	if len(text) <= limit {
		return text
	}
	return text[:limit] + " [...]"
}
