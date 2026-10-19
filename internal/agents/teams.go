package agents

import (
	"fmt"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// A team template is a ready-made team for one kind of work: a coordinator
// and the specialists it hands work to, each with a prompt written for that
// work (defaults/<folder>/<name>.md). Adding one makes ordinary agents you
// can then change; the template itself changes nothing afterwards.
type Template struct {
	ID          string   `json:"id"`          // stable, recorded on the agents it makes
	Team        string   `json:"team"`        // the team's name, which you may change
	Description string   `json:"description"` // what the team is for, on the page
	Folder      string   `json:"-"`           // where its prompts are, under defaults/
	Agents      []Agent  `json:"agents"`
	Features    []string `json:"features"` // what a team made from it turns on, such as "ideas"
	// Given says its agents carry their own prompts, as a template you
	// saved does, instead of reading them from defaults/.
	Given bool `json:"-"`
}

// Templates are the teams Agentry offers. Software is what a new install
// starts with.
var Templates = []Template{
	{ID: "software", Team: DefaultTeam, Agents: defaults,
		Description: "Builds and changes software: writes the code with tests, reviews it, tries it for real and ships it."},
	{ID: "writing", Team: "Writing", Folder: "writing",
		Description: "Writes articles, documentation, reports and proposals: researched, drafted for the reader, fact-checked and edited.",
		Agents: []Agent{
			{Name: "editor", Coordinates: true, Engine: "claude", Profile: "read-only", Color: "orange",
				Description: "Takes every piece of writing, sets its brief, decides who works on it next, and edits it"},
			{Name: "researcher", Engine: "claude", Profile: "developer", Color: "teal",
				Description: "Finds and records the facts, figures, quotes and sources a piece rests on"},
			{Name: "writer", Engine: "claude", Profile: "developer", Color: "blue",
				Description: "Outlines and drafts the piece for its reader, in plain language, every fact sourced"},
			{Name: "fact-checker", Engine: "claude", Profile: "developer", Color: "violet", Column: Reviews,
				Description: "Checks every claim in a draft against its sources and lists what must change"},
		}},
	{ID: "research", Team: "Research", Folder: "research",
		Description: "Answers questions with evidence: frames the question, gathers sources and data, analyses it reproducibly with charts, and challenges the conclusions.",
		Agents: []Agent{
			{Name: "research-lead", Coordinates: true, Engine: "claude", Profile: "read-only", Color: "orange",
				Description: "Takes every research question, frames it, plans the method and decides who works on it next"},
			{Name: "investigator", Engine: "claude", Profile: "developer", Color: "teal",
				Description: "Searches systematically for the evidence for and against, and records where each piece came from"},
			{Name: "analyst", Engine: "claude", Profile: "developer", Color: "blue",
				Description: "Analyses the evidence with scripts anyone can rerun, with uncertainty and SVG charts"},
			{Name: "critic", Engine: "claude", Profile: "developer", Color: "violet", Column: Reviews,
				Description: "Challenges the study's evidence and reasoning, reruns the analysis, and lists what must be fixed"},
		}},
	{ID: "design", Team: "Design", Folder: "design",
		Description: "Designs products and features: frames the problem, maps the flows, draws wireframes for every state, writes the words, and critiques it all.",
		Agents: []Agent{
			{Name: "product-lead", Coordinates: true, Engine: "claude", Profile: "read-only", Color: "orange",
				Description: "Takes every design task, frames the problem and its measure of success, and decides who works on it next"},
			{Name: "product-designer", Engine: "claude", Profile: "developer", Color: "pink",
				Description: "Maps the flows and draws SVG wireframes for every state, accessible and explained"},
			{Name: "ux-writer", Engine: "claude", Profile: "developer", Color: "amber",
				Description: "Writes the words on every screen and state, in the users' own language, as a copy deck"},
			{Name: "design-critic", Engine: "claude", Profile: "developer", Color: "violet", Column: Reviews,
				Description: "Reviews the designs against the spec, usability heuristics and WCAG 2.2 AA, with a severity for each finding"},
		}},
	{ID: "visionaries", Team: "Visionaries", Folder: "visionaries", Features: []string{"ideas"},
		Description: "Thinks together about what to build next: reads what exists, argues it out from several viewpoints, and suggests features as ideas. Changes no code.",
		Agents: []Agent{
			{Name: "visionary-lead", Coordinates: true, Engine: "claude", Profile: "read-mostly", Color: "orange",
				Description: "Runs a short session of the viewpoints, then merges, ranks and writes up the ideas that survive"},
			{Name: "product-strategist", Engine: "claude", Profile: "read-mostly", Color: "blue",
				Description: "Proposes features from who the users are, what they value and where the product stands"},
			{Name: "user-advocate", Engine: "claude", Profile: "read-mostly", Color: "pink",
				Description: "Speaks for the people using it: their pain points, their workflows, what confuses them today"},
			{Name: "technologist", Engine: "claude", Profile: "read-mostly", Color: "teal",
				Description: "Says what is newly possible and what the architecture makes cheap or expensive to build"},
			{Name: "skeptic", Engine: "claude", Profile: "read-mostly", Color: "violet", Column: Reviews,
				Description: "Pushes back on every idea: cost, risk, scope creep, and whether we need it at all"},
		}},
	{ID: "networking", Team: "Networking", Folder: "networking",
		Description: "Diagnoses and changes networks: connectivity, DNS, TLS, proxies, VPNs and firewalls. Works on config in repos, proves each change from where it matters, and touches nothing live without your go-ahead.",
		Agents: []Agent{
			{Name: "network-lead", Coordinates: true, Engine: "claude", Profile: "read-only", Color: "orange",
				Description: "Takes every networking task, splits and plans it, and decides who works on it next and what needs your go-ahead"},
			{Name: "network-engineer", Engine: "claude", Profile: "developer", Color: "blue",
				Description: "Investigates and changes the config in the repo: proxy and firewall config, compose and stack files, IaC, docs and diagrams"},
			{Name: "network-verifier", Engine: "claude", Profile: "network-diagnosis", Color: "teal", Column: Reviews,
				Description: "Proves a change works, or finds where it breaks, from inside the host or container, the LAN, the private network and the public internet, with the evidence"},
			{Name: "network-security", Engine: "claude", Profile: "network-diagnosis", Color: "violet", Column: Reviews,
				Description: "Checks every change for exposure, auth, TLS and a tested rollback before it lands"},
		}},
}

// prompted is the template's agents with their prompts, its team and its ID.
func (t Template) prompted() []Agent {
	agents := make([]Agent, len(t.Agents))
	for i, agent := range t.Agents {
		agent.Template = t.ID
		if t.Given {
			agents[i] = agent
			continue
		}
		path := "defaults/" + agent.Name + ".md"
		if t.Folder != "" {
			path = "defaults/" + t.Folder + "/" + agent.Name + ".md"
			agent.Team = t.Team
		}
		prompt, err := defaultPrompts.ReadFile(path)
		if err != nil {
			panic("missing default prompt: " + err.Error()) // caught by the tests
		}
		agent.Prompt = strings.TrimSpace(string(prompt))
		agents[i] = agent
	}
	return agents
}

// TemplateNamed is the template of this team, if there is one.
func TemplateNamed(team string) (Template, bool) {
	for _, t := range Templates {
		if strings.EqualFold(t.Team, team) {
			return t, true
		}
	}
	return Template{}, false
}

// TemplateWithID is the template with this ID, if there is one.
func TemplateWithID(id string) (Template, bool) {
	for _, t := range Templates {
		if id != "" && t.ID == id {
			return t, true
		}
	}
	return Template{}, false
}

// Lists says whether the template turns the feature on.
func (t Template) Lists(feature string) bool {
	return slices.Contains(t.Features, feature)
}

// templateByName is the template a team of this name was made from, as
// AddTeam names them: the template's team, or that with a number after it
// ("Visionaries 2"). The zero Template when none is. Only for agents from
// before they recorded their template.
func templateByName(team string) Template {
	for _, t := range Templates {
		number, numbered := strings.CutPrefix(strings.ToLower(team), strings.ToLower(t.Team)+" ")
		if strings.EqualFold(team, t.Team) || numbered && isNumber(number) {
			return t
		}
	}
	return Template{}
}

func isNumber(text string) bool {
	return text != "" && strings.Trim(text, "0123456789") == ""
}

// AddTeam is the events that add a team made from t to the roster. A name
// taken already, by an agent or a team, gets a number, so nothing you have
// is changed.
func AddTeam(t Template, current Roster, by events.Actor) []events.Proposal {
	team := t.Team
	for n := 2; contains(current.Teams(), team); n++ {
		team = fmt.Sprintf("%s %d", t.Team, n)
	}
	taken := map[string]bool{}
	for _, agent := range current {
		taken[agent.Name] = true
	}
	var proposals []events.Proposal
	for _, agent := range t.prompted() {
		name := agent.Name
		for n := 2; taken[name]; n++ {
			name = fmt.Sprintf("%s-%d", agent.Name, n)
		}
		taken[name] = true
		agent.Name, agent.Team = name, team
		if team == DefaultTeam {
			agent.Team = ""
		}
		proposals = append(proposals, agent.Proposal(by, false))
	}
	return proposals
}

func contains(list []string, item string) bool {
	for _, x := range list {
		if strings.EqualFold(x, item) {
			return true
		}
	}
	return false
}
