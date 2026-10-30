package coordinator

import (
	"cmp"
	"fmt"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
	"gitlab.com/dev.chempa/agentry/internal/platform/workspace"
)

// A brief is everything an agent is given for one turn. In a fresh session
// it is all the agent knows besides the repository: its own prompt, then
// what Agentry knows about the task. A turn that goes on with the agent's
// last session on the task still has all that, so its brief is what has
// happened since and what every turn needs anew: its instructions or the
// decision to make, where the work is and the plan file, which is per turn.
// An agent that checks work starts fresh by default, and gets a handoff
// instead of the trail: the task, the commits to review and the builder's
// report, with its own earlier findings when it has checked before. All
// three are made from the same sections.

func coordinatorBrief(task *tasks.Task, coordinator agents.Agent, roster agents.Roster, f family, machines []machine, shared []sharedfiles.File,
	state extensions.Reader, list []extensions.Extension) string {
	hooks := hooksFor(list, coordinator, state)
	var b strings.Builder
	header(&b, coordinator, task)
	knowledge(&b, coordinator)
	section(&b, "The task", taskText(task))
	coordinatorWork(&b, task, coordinator, shared)

	var team strings.Builder
	for _, agent := range roster.Assignees(coordinator.TeamName()) {
		team.WriteString(about("- "+agent.Name, agent.Description) + "\n")
	}
	section(&b, "The agents you can hand work to", team.String())
	machinesSection(&b, task, coordinator, roster, machines)
	otherTeams(&b, task, coordinator, roster, basesOf(state))
	epicSection(&b, task, f)
	hookSections(&b, hooks, state, task)
	section(&b, "What has happened so far", trailText(task.Trail, 0)+"The last step is the newest. Decide what happens next.")
	yourDecision(&b, task, roster, hooks)
	return b.String()
}

// resumedCoordinatorBrief is the coordinator's brief for a turn that goes on
// with its last session, whose brief was made from the trail's first seen
// steps; or, when from is another story of the epic, with its session
// there (see goingOn). The machines online and the epic's stories change
// without a step on the trail, so it has them as they are now. Of the
// task's kind, only the epic it is or is a story of: how to split it was
// in its first brief.
func resumedCoordinatorBrief(task *tasks.Task, coordinator agents.Agent, roster agents.Roster, seen int, from string, f family,
	machines []machine, shared []sharedfiles.File, state extensions.Reader, list []extensions.Extension) string {
	hooks := hooksFor(list, coordinator, state)
	var b strings.Builder
	trailHeading := goingOn(&b, coordinator, task, from)
	section(&b, trailHeading, cmp.Or(trailText(since(task.Trail, seen), seen), "Nothing new on the trail.\n\n")+
		"The last step is the newest. Decide what happens next.")
	coordinatorWork(&b, task, coordinator, shared)
	machinesSection(&b, task, coordinator, roster, machines)
	familySection(&b, task, f)
	hookSections(&b, hooks, state, task)
	keepYourNotes(&b)
	yourDecision(&b, task, roster, hooks)
	return b.String()
}

// coordinatorWork is where the coordinator's work is and the plan it keeps.
func coordinatorWork(b *strings.Builder, task *tasks.Task, coordinator agents.Agent, shared []sharedfiles.File) {
	section(b, "Where the work is", whereText(task))
	section(b, "Shared files", sharedText(task, shared, true))
	plan(b, task, coordinator.Name, "your plan for the task as a whole: the stages it goes through, which agent does each, and what is done")
}

// machinesSection is the machines the coordinator's team may run on, when
// there is more than this one to choose from.
func machinesSection(b *strings.Builder, task *tasks.Task, coordinator agents.Agent, roster agents.Roster, machines []machine) {
	if text := machinesText(task, machines, roster.Assignees(coordinator.TeamName())); text != "" {
		section(b, "Machines", text)
	}
}

func hookSections(b *strings.Builder, hooks []extensions.Extension, state extensions.Reader, task *tasks.Task) {
	for _, more := range briefSections(hooks, state, task) {
		section(b, more.Heading, more.Text)
	}
}

// YouChoseToShip tells an agent that ships work that the user moved the task
// to Shipping, so it lands as it is, checked or not.
const YouChoseToShip = "The user chose to ship this as it is, so no earlier review or test is needed. " +
	"The checks you run after bringing it up to date still stand: if they fail, don't push; report why."

func workerBrief(task *tasks.Task, agent agents.Agent, roster agents.Roster, instructions string, shared []sharedfiles.File) string {
	var b strings.Builder
	header(&b, agent, task)
	knowledge(&b, agent)
	section(&b, "The task", taskText(task))
	workerWork(&b, task, agent, roster, instructions, shared)
	finish(&b, agent)
	return b.String()
}

// resumedWorkerBrief is the agent's brief for a turn that goes on with its
// last session, whose brief was made from the trail's first seen steps; or,
// when from is another story of the epic, with its session there (see
// goingOn). The newest step, the handoff giving these instructions, is
// left to them.
func resumedWorkerBrief(task *tasks.Task, agent agents.Agent, roster agents.Roster, instructions string, seen int, from string,
	shared []sharedfiles.File) string {
	steps := since(task.Trail, seen)
	if last := len(steps) - 1; last >= 0 && steps[last].To == agent.Name {
		steps = steps[:last]
	}
	var b strings.Builder
	trailHeading := goingOn(&b, agent, task, from)
	section(&b, trailHeading, cmp.Or(trailText(steps, seen), "Nothing but your new instructions."))
	workerWork(&b, task, agent, roster, instructions, shared)
	keepYourNotes(&b)
	finish(&b, agent)
	return b.String()
}

// workerWork is the agent's instructions, where the work is and the plan
// it keeps.
func workerWork(b *strings.Builder, task *tasks.Task, agent agents.Agent, roster agents.Roster, instructions string, shared []sharedfiles.File) {
	if agent.Column == agents.Ships && yourShip(task, roster) {
		instructions += "\n\n" + YouChoseToShip
	}
	section(b, "Your instructions from the coordinator", instructions)
	section(b, "Where the work is", whereText(task))
	section(b, "Shared files", sharedText(task, shared, false))
	plan(b, task, agent.Name, "your plan for this turn: the steps you will take to do your instructions")
}

// handoffBrief is the brief for an agent that checks work, in a fresh
// session: what a review needs and nothing of the trail. It has the task,
// the instructions and the work's place as workerBrief does; then the
// commits to review, as a range and the files it touches, with the git
// commands that show them, never the diff itself; the builder's last
// report; and, when the agent has checked this task before, its own
// earlier findings and what changed since. commits are the task branch's,
// newest first, as the machine the work is on lists them; nil when it
// couldn't.
func handoffBrief(task *tasks.Task, agent agents.Agent, roster agents.Roster, instructions string, shared []sharedfiles.File,
	commits []workspace.Commit) string {
	var b strings.Builder
	header(&b, agent, task)
	knowledge(&b, agent)
	section(&b, "The task", taskText(task))
	workerWork(&b, task, agent, roster, instructions, shared)
	section(&b, "What to review", reviewText(task.Worktree.Base, commits))
	section(&b, "The builder's last report", builderReport(task, roster))
	if last := lastReported(task, agent.Name); last != nil {
		section(&b, "Your earlier findings", "From your last turn on this task, "+last.ID+":\n\n"+quoted(last.Report))
		section(&b, "Changed since your last verdict", changedSince(commits, last.StartedAt))
	}
	finish(&b, agent)
	return b.String()
}

func section(b *strings.Builder, title, body string) {
	fmt.Fprintf(b, "## %s\n\n%s\n\n", title, strings.TrimSpace(body))
}
