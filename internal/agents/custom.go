package agents

import (
	"slices"
	"strconv"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// A custom team is one you define yourself, agent by agent, instead of
// adding it from a template. CheckCustomTeam says what is wrong with one;
// CustomTeam is the events that add it.

// CheckCustomTeam is why the team of these members can't be added to the
// roster, or "" when it can. Engines, models and profiles are left to the
// rules and to policy.toml.
func CheckCustomTeam(team string, members []Agent, current Roster) string {
	team, members = trimmed(team, members)
	if problem := checkTeamName(team, current); problem != "" {
		return problem
	}
	if len(members) == 0 {
		return "the " + team + " team needs agents"
	}
	if problem := checkCoordinators(team, members); problem != "" {
		return problem
	}
	seen := map[string]bool{}
	for _, agent := range members {
		if problem := checkMember(team, agent, current, seen); problem != "" {
			return problem
		}
		seen[agent.Name] = true
	}
	return ""
}

func checkTeamName(team string, current Roster) string {
	switch {
	case team == "":
		return "a team needs a name"
	case contains(current.Teams(), team):
		return "there is a team called " + team + " already"
	}
	if t, ok := TemplateNamed(team); ok {
		return team + " is the name of the " + t.Team + " template's team; choose another"
	}
	return ""
}

func checkCoordinators(team string, members []Agent) string {
	var coordinators []string
	for _, agent := range members {
		if agent.Coordinates {
			coordinators = append(coordinators, agent.Name)
		}
	}
	switch len(coordinators) {
	case 0:
		return "the " + team + " team needs exactly one coordinator; it has none"
	case 1:
		return ""
	}
	return "the " + team + " team needs exactly one coordinator; it has " +
		number(len(coordinators)) + ": " + strings.Join(coordinators, ", ")
}

// checkMember checks one agent of the team, given the names seen before it.
func checkMember(team string, agent Agent, current Roster, seen map[string]bool) string {
	switch {
	case agent.Name == "":
		return "every agent of the " + team + " team needs a name"
	case !ValidName(agent.Name):
		return "an agent's name is lowercase letters, digits and dashes, starting with a letter: " + agent.Name
	case seen[agent.Name]:
		return "the " + team + " team has two agents called " + agent.Name
	case current.index(agent.Name) >= 0:
		return "an agent called " + agent.Name + " already exists"
	case !slices.Contains([]string{Builds, Reviews, Ships}, agent.Column):
		return agent.Name + "'s column must be builds (\"\"), review or ship, not " + agent.Column
	case agent.Color != "" && !slices.Contains(Colors, agent.Color):
		return agent.Name + "'s colour must be one of " + strings.Join(Colors, ", ") + ", not " + agent.Color
	}
	return ""
}

// CustomTeam is the events that add the team of these members to the
// roster, as CheckCustomTeam allowed it: the coordinator first, as in every
// template, so a team never has agents and no coordinator. An empty prompt
// gets a default one.
func CustomTeam(team string, members []Agent, by events.Actor) []events.Proposal {
	team, members = trimmed(team, members)
	slices.SortStableFunc(members, func(a, b Agent) int {
		if a.Coordinates == b.Coordinates {
			return 0
		} else if a.Coordinates {
			return -1
		}
		return 1
	})
	var proposals []events.Proposal
	for _, agent := range members {
		agent.Team, agent.Template = team, ""
		if agent.Prompt == "" {
			agent.Prompt = defaultPrompt(agent)
		}
		proposals = append(proposals, agent.Proposal(by, false))
	}
	return proposals
}

// trimmed is the team's name and a copy of its members with the spaces
// around their text fields gone, as the agents API trims one agent.
func trimmed(team string, members []Agent) (string, []Agent) {
	members = slices.Clone(members)
	for i := range members {
		agent := &members[i]
		agent.Name = strings.TrimSpace(agent.Name)
		agent.Description = strings.TrimSpace(agent.Description)
		agent.Prompt = strings.TrimSpace(agent.Prompt)
		agent.Engine = strings.TrimSpace(agent.Engine)
		agent.Model = strings.TrimSpace(agent.Model)
		agent.Profile = strings.TrimSpace(agent.Profile)
		agent.Color = strings.TrimSpace(agent.Color)
		agent.Column = strings.TrimSpace(agent.Column)
	}
	return strings.TrimSpace(team), members
}

// defaultPrompt is the prompt of an agent of a custom team you gave none:
// for its coordinator, a coordinator's prompt for any kind of work; for the
// others, one made from its description.
func defaultPrompt(agent Agent) string {
	if agent.Coordinates {
		prompt, err := defaultPrompts.ReadFile("defaults/custom/coordinator.md")
		if err != nil {
			panic("missing default prompt: " + err.Error()) // caught by the tests
		}
		return strings.TrimSpace(string(prompt))
	}
	job := strings.TrimSuffix(agent.Description, ".")
	if job == "" {
		job = "what your coordinator asks of you"
	}
	prompt := "You are " + agent.Name + ", one of the agents of the " + agent.Team + " team. Your job: " + job + ".\n\n" +
		"Your coordinator briefs you on each piece of work. Do only that work, as the brief describes it, and leave the rest to the other agents of your team. " +
		"When you finish, report what you did, how you checked it, and anything left or blocking you."
	if agent.Column == Reviews {
		prompt += " End your report with your verdict on its own last line: `Verdict: approved` when nothing must change, or `Verdict: changes needed`."
	}
	return prompt
}

func number(n int) string {
	if words := []string{"none", "one", "two", "three", "four", "five"}; n < len(words) {
		return words[n]
	}
	return strconv.Itoa(n)
}
