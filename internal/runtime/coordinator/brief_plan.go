package coordinator

import (
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/runtime/machines"
)

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
