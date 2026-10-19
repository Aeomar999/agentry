package rules

import (
	"encoding/json"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
)

// templateRules check the team templates you save, on their own: a template
// makes agents only when you add it, under the agent rules then.
var templateRules = map[events.Type]func(events.Proposal) string{
	events.TemplateSaved:   aSavedTemplate,
	events.TemplateRemoved: aRemovedTemplate,
}

// asAnAgent is what each of a template's agents must be to be made as one.
// TODO(task-175): check a template's team with the shared custom-team
// validation once it is in, instead of this and the checks below.
var asAnAgent = allOf(nameIsValid, engineIsKnown, hasPrompt, colorIsKnown, columnIsKnown)

func aSavedTemplate(p events.Proposal) string {
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
	members, err := templateAgents(p)
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

// templateAgents reads a template's agents, whether a proposal not yet
// written holds them as agents or the log gave them back as JSON.
func templateAgents(p events.Proposal) ([]agents.Agent, error) {
	encoded, err := json.Marshal(p.Data["agents"])
	if err != nil {
		return nil, err
	}
	var members []agents.Agent
	return members, json.Unmarshal(encoded, &members)
}

// aRemovedTemplate can't yet check that the id was saved and not removed:
// the state the rules read keeps no templates.
// TODO(task-176): refuse an id that isn't saved, once the state keeps templates.
func aRemovedTemplate(p events.Proposal) string {
	if strings.TrimSpace(p.Text("id")) == "" {
		return "which template? It needs an id"
	}
	return ""
}
