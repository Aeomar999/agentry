package rules

import (
	"fmt"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// templateRules check the team templates you save against those saved
// already, and those removed: a template makes agents only when you add
// it, under the agent rules then.
var templateRules = map[events.Type]func(agents.UserTemplates, agents.RemovedTemplates, events.Proposal) string{
	events.TemplateSaved:   aSavedTemplate,
	events.TemplateRemoved: aRemovedTemplate,
}

// asAnAgent is what each of a template's agents must be to be made as one.
// TODO(task-175): check a template's team with the shared custom-team
// validation once it is in, instead of this and the checks below.
var asAnAgent = allOf(nameIsValid, engineIsKnown, hasPrompt, colorIsKnown, columnIsKnown)

// templateAgentNameLimit leaves room in an agent's name for the number
// AddTeam gives it when the name is taken: "-99" still fits the agent rules.
const templateAgentNameLimit = 28

func aSavedTemplate(saved agents.UserTemplates, removed agents.RemovedTemplates, p events.Proposal) string {
	switch team := p.Text("team"); {
	case strings.TrimSpace(team) == "":
		return "a template needs a team name"
	case strings.TrimSpace(team) != team:
		return `a template's team name has no spaces at its ends: "` + team + `"`
	}
	// A team is never added under a built-in template's team name, so a
	// template of that name could be saved but never added.
	if t, ok := agents.TemplateNamed(p.Text("team")); ok {
		return p.Text("team") + " is the name of the " + t.Team + " template's team: rename the team before saving it as a template"
	}
	if reason := CheckTemplateID(p.Text("id")); reason != "" {
		return reason
	}
	if reason := builtInBase(p.Text("basedOn")); reason != "" {
		return reason
	}
	if reason := takesNoOtherTemplate(saved, removed, p); reason != "" {
		return reason
	}
	members, err := agents.TemplateAgents(p.Data)
	if err != nil {
		return "a template's agents are a list of agents: " + err.Error()
	}
	if len(members) == 0 {
		return "a template needs at least one agent"
	}
	names, coordinators := map[string]bool{}, 0
	for _, agent := range members {
		if names[agent.Name] {
			return "a template has two agents called " + agent.Name
		}
		names[agent.Name] = true
		if agent.Coordinates {
			coordinators++
		}
		if reason := asAnAgent(nil, agent.Proposal(p.Actor, false)); reason != "" {
			return "agent " + agent.Name + ": " + reason
		}
		if reason := keepsNothingOfAnInstall(agent); reason != "" {
			return "agent " + agent.Name + ": " + reason
		}
		if len(agent.Name) > templateAgentNameLimit {
			return fmt.Sprintf("agent %s: a template's agent names are at most %d characters, to leave room for a number when the name is taken", agent.Name, templateAgentNameLimit)
		}
	}
	if coordinators != 1 {
		return "a template has exactly one coordinator"
	}
	return ""
}

// builtInBase refuses a template based on anything but a built-in
// template: its teams keep that template's extensions, and only a built-in
// one has any. None at all is fine.
func builtInBase(basedOn string) string {
	if _, builtIn := agents.TemplateWithID(basedOn); basedOn != "" && !builtIn {
		return "a template is based on a built-in template, and there is no built-in template " + basedOn
	}
	return ""
}

// takesNoOtherTemplate refuses a save the name or id of a template it
// isn't meant to replace: a new one replaces none, an update replaces the
// live template of yours with its id, and an import, or a save older logs
// hold, whichever has its id.
func takesNoOtherTemplate(saved agents.UserTemplates, removed agents.RemovedTemplates, p events.Proposal) string {
	team, id := p.Text("team"), p.Text("id")
	switch {
	case p.Flag("new") && p.Flag("replacing"):
		return "a template is saved as a new one or replacing one of yours, not both"
	case p.Flag("new"):
		return notYoursYet(saved, removed, team, id)
	case p.Flag("replacing"):
		if reason := yoursToReplace(saved, removed, id); reason != "" {
			return reason
		}
	}
	return noOtherCalled(saved, team, id)
}

// yoursToReplace refuses an update of a template that isn't one of yours
// now: one you removed comes back only by importing its file. Only Propose
// runs it, as older logs hold no update.
func yoursToReplace(saved agents.UserTemplates, removed agents.RemovedTemplates, id string) string {
	if _, gone := removed[id]; gone {
		return "you removed the template " + id + ", so there is none to update: import its file to bring it back"
	}
	if _, yours := saved.WithID(id); !yours {
		return "you have no template " + id + " to update"
	}
	return ""
}

// notYoursYet refuses a new template, saved from a team, a name or id one of
// yours has, or the id of one you removed, whose teams still name it: the
// web checks before saving too, and this keeps two saves at once from both
// passing it, the second replacing the first. Only Propose runs it, so
// older logs holding a save that replaced one still replay.
func notYoursYet(saved agents.UserTemplates, removed agents.RemovedTemplates, team, id string) string {
	if _, gone := removed[id]; gone {
		return "you removed a template with the id " + id + ": import its file to bring it back"
	}
	for _, yours := range saved {
		switch {
		case strings.EqualFold(yours.Team, team):
			return "you have a template called " + yours.Team + " already"
		case yours.ID == id:
			return "your template " + yours.Team + " has the id " + id
		}
	}
	return ""
}

// noOtherCalled refuses an import or an update a name, in any case, that
// one of yours has under another id: the web checks before saving too, and
// this keeps two saves at once from both passing it. The template with its
// id is the one it replaces, so its name is free. Only Propose runs it, so
// older logs holding two templates of one name still replay.
func noOtherCalled(saved agents.UserTemplates, team, id string) string {
	for _, yours := range saved {
		if yours.ID != id && strings.EqualFold(yours.Team, team) {
			return "you have a template called " + yours.Team + " already"
		}
	}
	return ""
}

// CheckTemplateID is "" for an id a template you save may have, or why not.
// Every template saved, from a team or a file, is checked with it; the
// web's templateIDFor makes ids that pass it, as the page has no id field.
func CheckTemplateID(id string) string {
	switch {
	case strings.TrimSpace(id) == "":
		return "a template needs an id"
	case !agents.ValidName(id):
		// A template's id goes in URLs, as an agent's name goes in file
		// names, and the built-in templates' ids have the same shape.
		return "a template's id is lowercase letters, digits and dashes, starting with a letter, at most 31 characters: " + id
	}
	if _, builtIn := agents.TemplateWithID(id); builtIn {
		return "the built-in " + id + " template has that id"
	}
	return ""
}

// keepsNothingOfAnInstall refuses what SaveTemplate drops from a team's
// agents: the machines one runs on may not be there when the template is
// added, and its knowledge, team and template are set again then.
func keepsNothingOfAnInstall(agent agents.Agent) string {
	switch {
	case len(agent.Workers) > 0:
		return "a template's agents name no workers, as the machines may not be there when it is added"
	case agent.Knowledge != "":
		return "a template's agents carry no knowledge"
	case agent.Team != "":
		return "a template's agents name no team: adding the template sets it"
	case agent.Template != "":
		return "a template's agents name no template: adding the template sets it"
	}
	return ""
}

func aRemovedTemplate(saved agents.UserTemplates, _ agents.RemovedTemplates, p events.Proposal) string {
	id := p.Text("id")
	if strings.TrimSpace(id) == "" {
		return "which template? It needs an id"
	}
	if _, ok := saved.WithID(id); !ok {
		return "there is no saved template " + id
	}
	return ""
}
