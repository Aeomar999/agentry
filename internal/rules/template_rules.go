package rules

import (
	"fmt"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
)

// templateRules check the team templates you save against those saved
// already: a template makes agents only when you add it, under the agent
// rules then.
var templateRules = map[events.Type]func(agents.UserTemplates, events.Proposal) string{
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

func aSavedTemplate(_ agents.UserTemplates, p events.Proposal) string {
	id := p.Text("id")
	switch {
	case strings.TrimSpace(id) == "":
		return "a template needs an id"
	case strings.TrimSpace(p.Text("team")) == "":
		return "a template needs a team name"
	case !agents.ValidName(id):
		// A template's id goes in URLs, as an agent's name goes in file
		// names, and the built-in templates' ids have the same shape.
		return "a template's id is lowercase letters, digits and dashes, starting with a letter, at most 31 characters: " + id
	}
	if _, builtIn := agents.TemplateWithID(id); builtIn {
		return "the built-in " + id + " template has that id"
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

func aRemovedTemplate(saved agents.UserTemplates, p events.Proposal) string {
	id := p.Text("id")
	if strings.TrimSpace(id) == "" {
		return "which template? It needs an id"
	}
	if _, ok := saved.WithID(id); !ok {
		return "there is no saved template " + id
	}
	return ""
}
