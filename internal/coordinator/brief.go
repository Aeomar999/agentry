package coordinator

import (
	"cmp"
	"fmt"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/machines"
	"gitlab.com/dev.chempa/agentry/internal/sharedfiles"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// A brief is everything an agent is given for one turn. In a fresh session
// it is all the agent knows besides the repository: its own prompt, then
// what Agentry knows about the task. A turn that goes on with the agent's
// last session on the task still has all that, so its brief is what has
// happened since and what every turn needs anew: its instructions or the
// decision to make, where the work is and the plan file, which is per turn.
// Both are made from the same sections.

// longestStep caps each trail entry in the coordinator's brief.
const longestStep = 4000

func coordinatorBrief(task *tasks.Task, coordinator agents.Agent, roster agents.Roster, f family, machines []machine, shared []sharedfiles.File,
	state features.Reader, list []features.Feature) string {
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
// steps. The machines online and the epic's stories change without a step
// on the trail, so it has them as they are now.
func resumedCoordinatorBrief(task *tasks.Task, coordinator agents.Agent, roster agents.Roster, seen int, f family, machines []machine,
	shared []sharedfiles.File, state features.Reader, list []features.Feature) string {
	hooks := hooksFor(list, coordinator, state)
	var b strings.Builder
	resumedHeader(&b, coordinator, task)
	section(&b, "What has happened since your last turn", cmp.Or(trailText(since(task.Trail, seen), seen), "Nothing new on the trail.\n\n")+
		"The last step is the newest. Decide what happens next.")
	coordinatorWork(&b, task, coordinator, shared)
	machinesSection(&b, task, coordinator, roster, machines)
	epicSection(&b, task, f)
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

func hookSections(b *strings.Builder, hooks []features.Feature, state features.Reader, task *tasks.Task) {
	for _, more := range briefSections(hooks, state, task) {
		section(b, more.Heading, more.Text)
	}
}

// yourDecision is what the coordinator's brief ends with: the actions Agentry
// refused, what finishing waits on, and the decision to make.
func yourDecision(b *strings.Builder, task *tasks.Task, roster agents.Roster, hooks []features.Feature) {
	if refused := refusedActions(task); refused != "" {
		section(b, "Actions Agentry refused", refused)
	}
	if task.WrapUp != nil {
		section(b, "Wrap up", wrapUpText(task))
		section(b, "Your decision", WrapUpFormat)
		return
	}
	checks(b, task, roster)
	decision := DecisionFormat
	for _, hook := range hooks {
		if hook.Coordinator.Decision != "" {
			decision += "\n" + hook.Coordinator.Decision
		}
	}
	section(b, "Your decision", decision)
}

// trailText numbers the steps, the first being step first+1 of the trail.
func trailText(steps []tasks.Step, first int) string {
	var trail strings.Builder
	for i, step := range steps {
		fmt.Fprintf(&trail, "%d. %s → %s: %s\n\n", first+i+1, step.From, step.To, tasks.Shorten(step.Text, longestStep))
	}
	return trail.String()
}

// since is the trail after its first seen steps.
func since(trail []tasks.Step, seen int) []tasks.Step {
	return trail[min(seen, len(trail)):]
}

// wrapUpText tells the coordinator the user moved the task themselves, and
// what its one last turn may do.
func wrapUpText(task *tasks.Task) string {
	moved := "The user moved this task to Done themselves: it is finished as it is, and it stays done."
	if task.WrapUp.To != "done" {
		moved = "The user moved this task to " + tasks.PlaceName(task.WrapUp.To) + " themselves: it waits there until they move it on."
	}
	return moved + " Any turn in progress was stopped; its work so far is in the worktree.\n\n" +
		"This is your last turn on the task until they move it again: wrap it up. You may write its summary for the user, " +
		"commit or tidy the work in progress on its branch (never push), and note what was left undone and where the work is. " +
		"Start no new work: don't hand the task to an agent, ask, split, move it or pass it to another team, and don't reopen " +
		"it. Agentry keeps your summary and refuses any other decision."
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
// last session, whose brief was made from the trail's first seen steps. The
// newest step, the handoff giving these instructions, is left to them.
func resumedWorkerBrief(task *tasks.Task, agent agents.Agent, roster agents.Roster, instructions string, seen int,
	shared []sharedfiles.File) string {
	steps := since(task.Trail, seen)
	if last := len(steps) - 1; last >= 0 && steps[last].To == agent.Name {
		steps = steps[:last]
	}
	var b strings.Builder
	resumedHeader(&b, agent, task)
	section(&b, "What has happened since your last turn", cmp.Or(trailText(steps, seen), "Nothing but your new instructions."))
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

// finish is how the agent ends its turn: with a report, and a reviewer's
// verdict last.
func finish(b *strings.Builder, agent agents.Agent) {
	text := "End with a short report for the coordinator: what you did, what you found, and anything left or blocking you. " +
		"The coordinator reads only this report, so make it complete. Don't stop to ask questions: put them in the report."
	if agent.Column == agents.Reviews {
		text += "\n\n" + VerdictFormat
	}
	section(b, "When you finish", text)
}

// checks tells the coordinator what finishing the task waits on: its
// checkers' approval of the last change (unchecked).
func checks(b *strings.Builder, task *tasks.Task, roster agents.Roster) {
	rule := "Before you finish, the work must be checked. Since the last turn that changed it, every agent of your team " +
		"that checks work and has checked it must approve it again, in the verdict its report ends with. When a check asks for " +
		"changes, hand the findings to the agent that builds it; once they are made, hand it back for another check. Agentry " +
		"refuses \"done\" until then."
	if why := unchecked(task, roster); why != "" {
		section(b, "Before you finish", rule+"\n\nNow: "+why+".")
	} else if lead, _ := roster.Get(task.Coordinator); hasCheckers(roster, lead.TeamName()) {
		section(b, "Before you finish", rule+"\n\nNow: the work as it is has been checked, or nothing has changed it yet.")
	}
}

func hasCheckers(roster agents.Roster, team string) bool {
	for _, agent := range roster.Assignees(team) {
		if agent.Column == agents.Reviews {
			return true
		}
	}
	return false
}

// keepYourNotes reminds an agent going on with its session of the notes its
// first brief asked it to keep.
func keepYourNotes(b *strings.Builder) {
	section(b, "Your notes", "Before you finish, update your notes in "+machines.KnowledgeFile+", as your first brief on this task said.")
}

// knowledge is the agent's notes from earlier tasks, and how to keep them.
func knowledge(b *strings.Builder, agent agents.Agent) {
	notes := agent.Knowledge
	if notes == "" {
		notes = "(Nothing yet.)"
	}
	section(b, "What you know from earlier tasks", notes+"\n\n"+
		"These notes are yours, kept across tasks, in "+machines.KnowledgeFile+". Before you finish, update that file: add what "+
		"you learned that will help on a later task (how this codebase does things, a trap you hit, what the user prefers), "+
		"fix what turned out wrong, and drop what no longer matters. Keep it short: a few lines per point. Leave it as it is "+
		"if you learned nothing new.")
}

// plan asks the agent to write down its plan before it starts, and keep it
// true as it works: the person watching the task reads it as it changes.
// When its last turn left a plan unfinished, that plan is already in the
// file (turns.Runner), for it to continue or replace.
func plan(b *strings.Builder, task *tasks.Task, agent, what string) {
	file := machines.PlanFile(task.NextTurnID())
	keep := "As you finish a step, tick it: `- [x] step`. Plans change as you learn: when yours does, update the file at once, " +
		"adding, dropping or rewording steps, so it always says what you are really doing. The person watching this task " +
		"follows it there as you work."
	if _, from := task.PlanToContinue(agent); from != "" {
		section(b, "Your plan", "Your plan from "+from+" isn't finished, and it is already in "+file+". Before anything else, "+
			"read it and bring it up to date: tick what is done, drop what no longer applies, and add what your instructions "+
			"now need. If your instructions are different work, replace it with a new plan for that work, as a Markdown "+
			"checklist, one line per step, `- [ ] step`. "+keep)
		return
	}
	section(b, "Your plan", "Before anything else, write "+what+" to "+file+", as a Markdown checklist: one line per step, "+
		"`- [ ] step`. Keep it short and concrete. "+keep)
}

// basesOf are the bases of the templates in state; none without a state,
// so built-in links still resolve.
func basesOf(state features.Reader) agents.Bases {
	if state == nil {
		return agents.Bases{}
	}
	return agents.BasesIn(state)
}

// otherTeams lists the other teams, and when to pass the task to one or ask
// one for something.
func otherTeams(b *strings.Builder, task *tasks.Task, coordinator agents.Agent, roster agents.Roster, bases agents.Bases) {
	var list strings.Builder
	for _, team := range roster.Teams() {
		lead, ok := roster.Coordinator(team)
		if team == coordinator.TeamName() || !ok {
			continue
		}
		list.WriteString(about("- "+team+", coordinated by "+lead.Name, teamDescription(lead)) + "\n")
	}
	if list.Len() == 0 {
		return
	}
	ask := "ask them with a \"request\" decision: they get a task of their own, this one waits for it, and their " +
		"summary comes back to you here, so you carry on"
	if task.Kind == tasks.Epic {
		ask = "give them a story of this epic, with \"team\" in the split"
	}
	text := list.String() + "\nYou are in the " + coordinator.TeamName() + " team. When this task needs something only " +
		"another team does well" + routingExamples(roster, bases) + ", " + ask + ". Ask for one thing at a time, and say exactly " +
		"what you need and what done looks like."
	if !task.TeamChosen && task.Parent == "" {
		text += "\n\nThe user didn't choose a team for this task. Before anyone works on it, decide whose work it is: if it " +
			"is another team's, pass it to them with a \"transfer\" decision, saying why; if it is your team's, go on."
	}
	section(b, "Other teams", text)
}

// teamDescription is what the team led by lead is for: the description of
// the built-in template lead was made from, else lead's own.
func teamDescription(lead agents.Agent) string {
	if template, found := agents.TemplateWithID(lead.Template); found && template.Description != "" {
		return template.Description
	}
	return lead.Description
}

// about is line followed by what it is about, when there is anything to say.
func about(line, description string) string {
	if description == "" {
		return line
	}
	return line + ": " + description
}

// routedWork is what each built-in template's team is asked for, in the
// order the routing sentence gives them.
var routedWork = []struct{ template, work string }{
	{"writing", "a document"}, {"research", "research"}, {"design", "a design"}, {"software", "code"},
}

// routingExamples is " (a document from Writing, ...)" for the teams in
// roster linked to routedWork's templates, or to templates of yours based
// on them, by their names now; "" when there are none.
func routingExamples(roster agents.Roster, bases agents.Bases) string {
	var examples []string
	for _, routed := range routedWork {
		for _, team := range roster.Teams() {
			if bases.Of(roster.TemplateOf(team)) == routed.template {
				examples = append(examples, routed.work+" from "+team)
				break
			}
		}
	}
	if len(examples) == 0 {
		return ""
	}
	return " (" + strings.Join(examples, ", ") + ")"
}

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

func refusedActions(task *tasks.Task) string {
	var refused strings.Builder
	for _, action := range task.Actions {
		if action.Decision == tasks.Denied {
			fmt.Fprintf(&refused, "- %s (%s, %s): `%s`: %s\n", action.Agent, action.Turn, action.Tool, tasks.Shorten(action.Command, 200), action.Reason)
		}
	}
	return refused.String()
}
