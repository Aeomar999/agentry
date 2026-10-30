package coordinator

import (
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/domain/agents"
	"gitlab.com/dev.chempa/agentry/internal/domain/tasks"
	"gitlab.com/dev.chempa/agentry/internal/extensions"
)

// basesOf are the bases of the templates in state; none without a state,
// so built-in links still resolve.
func basesOf(state extensions.Reader) agents.Bases {
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
