package rules

import (
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/tasks"
)

// TeamProblem is one reason a team edit is refused: about a field of one of
// its agents, or about the team.
type TeamProblem struct {
	Row    int    // the agent's place in the edit's agents; -1 for the team
	Field  string // an agent's field ("was", "name", "engine", ...), or the team's: "team", "coordinates", "agents" or "removed"
	Agent  string // with "removed", the agent removed
	Reason string
}

// teamEdited refuses a team edit for the first of its problems, or when
// the team has changed since the version the edit found, as another edit
// saved at the same time does.
func teamEdited(state State, p events.Proposal) string {
	if problems := TeamEditProblems(state, p); len(problems) > 0 {
		return problems[0].Reason
	}
	if team := p.Text("team"); p.Text("version") != state.Roster().TeamVersion(team) {
		return "the " + team + " team changed since you opened it: reload it"
	}
	return ""
}

// TeamEditProblems is every reason the rules would refuse the team edit p,
// in the edit's order, so a route can list them all before it writes. The
// team after the edit has exactly one coordinator, a name a new team could
// have (or its own), and agents named as agents are, each name its own; it
// removes only agents of the team, and none an unfinished task needs.
func TeamEditProblems(state State, p events.Proposal) []TeamProblem {
	edit, err := agents.TeamEditOf(p)
	if err != nil {
		return []TeamProblem{{Row: -1, Field: "agents", Reason: "a team edit's agents are a list of agents: " + err.Error()}}
	}
	roster := state.Roster()
	if reason := teamExists(roster, p); reason != "" {
		return []TeamProblem{{Row: -1, Field: "team", Reason: reason}}
	}
	var problems []TeamProblem
	if reason := newTeamNameIsFree(roster, p); reason != "" {
		problems = append(problems, TeamProblem{Row: -1, Field: "team", Reason: reason})
	}
	var members []agents.Agent
	for _, e := range edit.Agents {
		members = append(members, agents.Agent{Name: e.Name, Coordinates: e.Coordinates})
	}
	if reason := agents.CheckCoordinators(edit.To, members); reason != "" {
		problems = append(problems, TeamProblem{Row: -1, Field: "coordinates", Reason: reason})
	}
	problems = append(problems, rowProblems(roster, edit)...)
	problems = append(problems, membershipProblems(roster, edit)...)
	return append(problems, neededProblems(state, edit)...)
}

// asAnEditedAgent is what each agent of a team edit must be, by the field
// the rule is about.
var asAnEditedAgent = []struct {
	field string
	rule  rosterRule
}{{"name", nameIsValid}, {"engine", engineIsKnown}, {"prompt", hasPrompt}, {"color", colorIsKnown}, {"column", columnIsKnown}}

// rowProblems are the problems of each agent of the edit: its fields, the
// agent it was, and its name against the team's other agents and every
// agent of another team.
func rowProblems(roster agents.Roster, edit agents.TeamEdit) []TeamProblem {
	var problems []TeamProblem
	names, was := map[string]bool{}, map[string]bool{}
	for row, e := range edit.Agents {
		p := events.Proposal{Data: map[string]any{"name": e.Name, "engine": e.Engine, "prompt": e.Prompt, "color": e.Color, "column": e.Column}}
		for _, check := range asAnEditedAgent {
			if reason := check.rule(roster, p); reason != "" {
				problems = append(problems, TeamProblem{Row: row, Field: check.field, Reason: reason})
			}
		}
		if before, ok := roster.Get(e.Was); e.Was != "" && (!ok || before.TeamName() != edit.Team) {
			problems = append(problems, TeamProblem{Row: row, Field: "was", Reason: e.Was + " isn't an agent of the " + edit.Team + " team"})
		} else if was[e.Was] {
			problems = append(problems, TeamProblem{Row: row, Field: "was", Reason: "two of the team's agents were " + e.Was})
		}
		if other, ok := roster.Get(e.Name); ok && other.TeamName() != edit.Team {
			problems = append(problems, TeamProblem{Row: row, Field: "name", Reason: "an agent called " + e.Name + " already exists, in the " + other.TeamName() + " team"})
		} else if names[e.Name] {
			problems = append(problems, TeamProblem{Row: row, Field: "name", Reason: "the " + edit.To + " team has two agents called " + e.Name})
		}
		names[e.Name] = true
		if e.Was != "" {
			was[e.Was] = true
		}
	}
	return problems
}

// membershipProblems: every agent of the team is either kept, as the agent
// an edited one was, or removed, and nothing else is removed.
func membershipProblems(roster agents.Roster, edit agents.TeamEdit) []TeamProblem {
	kept := map[string]bool{}
	for _, e := range edit.Agents {
		kept[e.Was] = true
	}
	var problems []TeamProblem
	for _, name := range edit.Removed {
		switch agent, ok := roster.Get(name); {
		case !ok || agent.TeamName() != edit.Team:
			problems = append(problems, TeamProblem{Row: -1, Field: "removed", Agent: name, Reason: name + " isn't an agent of the " + edit.Team + " team to remove"})
		case kept[name]:
			problems = append(problems, TeamProblem{Row: -1, Field: "removed", Agent: name, Reason: name + " can't be both kept and removed"})
		}
	}
	for _, agent := range roster {
		if agent.TeamName() == edit.Team && !kept[agent.Name] && !slices.Contains(edit.Removed, agent.Name) {
			problems = append(problems, TeamProblem{Row: -1, Field: "agents", Reason: "the edit neither keeps nor removes " + agent.Name})
		}
	}
	return problems
}

// neededProblems: no removed agent is needed by an unfinished task. The
// tasks the team's coordinator coordinates move to the one the edit names,
// with any step handed to it, so they need it only for a turn it is taking.
func neededProblems(state State, edit agents.TeamEdit) []TeamProblem {
	lead, _ := state.Roster().Coordinator(edit.Team)
	var problems []TeamProblem
	for _, name := range edit.Removed {
		var needing []string
		for _, id := range state.TaskIDs() {
			task, err := state.Task(id)
			if err != nil || task == nil || !task.Needs(name) {
				continue
			}
			if name != lead.Name || neededBeyondMoving(task, name) {
				needing = append(needing, id)
			}
		}
		if len(needing) > 0 {
			problems = append(problems, TeamProblem{Row: -1, Field: "removed", Agent: name,
				Reason: name + " is still needed by " + strings.Join(needing, ", ") + ": finish those first, or keep " + name})
		}
	}
	return problems
}

// neededBeyondMoving says whether the task still needs the agent once it
// moves from it to another coordinator: for the turn it is taking, or a
// step handed to it on a task it doesn't coordinate.
func neededBeyondMoving(task *tasks.Task, agent string) bool {
	open := task.OpenTurn()
	return open != nil && open.Agent == agent || task.Coordinator != agent && task.Handoff != nil && task.Handoff.To == agent
}
