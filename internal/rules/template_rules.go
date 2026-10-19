package rules

import (
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

func aSavedTemplate(_ agents.UserTemplates, p events.Proposal) string {
	id := p.Text("id")
	switch {
	case strings.TrimSpace(id) == "":
		return "a template needs an id"
	case strings.TrimSpace(p.Text("team")) == "":
		return "a template needs a team name"
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
	}
	if coordinators != 1 {
		return "a template has exactly one coordinator"
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
