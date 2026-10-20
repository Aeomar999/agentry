package agents

import (
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// crew is a valid custom team's members: a coordinator, a maker and a
// checker, with prompts.
func crew() []Agent {
	return []Agent{
		{Name: "chef", Coordinates: true, Engine: "claude", Prompt: "Run the kitchen.", Description: "Plans the menu"},
		{Name: "cook", Engine: "claude", Prompt: "Cook.", Description: "Cooks the dishes", Color: "teal"},
		{Name: "taster", Engine: "claude", Prompt: "Taste.", Description: "Tastes every dish", Column: Reviews},
	}
}

func TestAValidCustomTeamPasses(t *testing.T) {
	if problem := CheckCustomTeam("Kitchen", crew(), FromEvents(seeded())); problem != "" {
		t.Errorf("a valid team was refused: %s", problem)
	}
	if problem := CheckCustomTeam("Kitchen", crew(), nil); problem != "" {
		t.Errorf("a valid team in an empty roster was refused: %s", problem)
	}
}

// Each case breaks one rule; the message names what broke it.
func TestACustomTeamIsRefused(t *testing.T) {
	writing, _ := TemplateNamed("writing")
	roster := FromEvents(seeded())
	for _, p := range AddTeam(writing, roster, you) {
		roster.Apply(events.Event{Proposal: p})
	}
	for _, c := range []struct {
		name, team string
		change     func([]Agent) []Agent
		want       string
	}{
		{"no team name", "", nil, "a team needs a name"},
		{"a blank team name", "  ", nil, "a team needs a name"},
		{"a team in the roster", "Writing", nil, "a team called Writing already"},
		{"a team in the roster, in other case", "wRiTiNg", nil, "a team called wRiTiNg already"},
		{"the default team", "software", nil, "a team called software already"},
		{"a template's team", "visionaries", nil, "the Visionaries template's team"},
		{"the Networking template's team, in other case", "NETWORKING", nil, "NETWORKING is the name of the Networking template's team"},
		{"no agents", "Kitchen", func([]Agent) []Agent { return nil }, "the Kitchen team needs agents"},
		{"no coordinator", "Kitchen", func(a []Agent) []Agent { a[0].Coordinates = false; return a },
			"exactly one coordinator; it has none"},
		{"two coordinators", "Kitchen", func(a []Agent) []Agent { a[2].Coordinates = true; return a },
			"exactly one coordinator; it has two: chef, taster"},
		{"an agent with no name", "Kitchen", func(a []Agent) []Agent { a[1].Name = ""; return a },
			"every agent of the Kitchen team needs a name"},
		{"an agent with a blank name", "Kitchen", func(a []Agent) []Agent { a[1].Name = " \t"; return a },
			"every agent of the Kitchen team needs a name"},
		{"an uppercase name", "Kitchen", func(a []Agent) []Agent { a[2].Name = "Taster"; return a },
			"an agent's name is lowercase letters, digits and dashes, starting with a letter: Taster"},
		{"a 32-character name", "Kitchen", func(a []Agent) []Agent { a[1].Name = "c" + strings.Repeat("o", 31); return a },
			"starting with a letter: c" + strings.Repeat("o", 31)},
		{"a name repeated", "Kitchen", func(a []Agent) []Agent { a[2].Name = " cook "; return a },
			"two agents called cook"},
		{"a name in the roster", "Kitchen", func(a []Agent) []Agent { a[1].Name = "editor"; return a },
			"an agent called editor already exists"},
		{"an unknown column", "Kitchen", func(a []Agent) []Agent { a[1].Column = "serve"; return a },
			"cook's column must be builds (\"\"), review or ship, not serve"},
		{"an unknown colour", "Kitchen", func(a []Agent) []Agent { a[2].Color = "mauve"; return a },
			"taster's colour must be one of"},
	} {
		members := crew()
		if c.change != nil {
			members = c.change(members)
		}
		if got := CheckCustomTeam(c.team, members, roster); !strings.Contains(got, c.want) {
			t.Errorf("%s: got %q, want it to say %q", c.name, got, c.want)
		}
	}
}

func TestCheckingACustomTeamChangesNothing(t *testing.T) {
	members := crew()
	members[0].Name = " chef "
	CheckCustomTeam("Kitchen", members, nil)
	if members[0].Name != " chef " {
		t.Errorf("the members were changed: %q", members[0].Name)
	}
}

func TestACustomTeamIsMadeCoordinatorFirst(t *testing.T) {
	members := crew()
	members[0], members[2] = members[2], members[0] // taster, cook, chef
	members[1].Name, members[1].Description = "  cook ", " Cooks the dishes "
	members[1].Template = "software"
	proposals := CustomTeam(" Kitchen ", "", members, you)
	var names []string
	for _, p := range proposals {
		names = append(names, p.Text("name"))
		if p.Type != events.AgentCreated || p.Actor != you || p.Text("team") != "Kitchen" || p.Text("template") != "" {
			t.Errorf("%s: %s by %v, team %q, template %q", p.Text("name"), p.Type, p.Actor, p.Text("team"), p.Text("template"))
		}
	}
	if strings.Join(names, " ") != "chef taster cook" {
		t.Errorf("the order: %v", names)
	}

	roster := FromEvents(seeded())
	for _, p := range proposals {
		roster.Apply(events.Event{Proposal: p})
	}
	if chef, ok := roster.Coordinator("Kitchen"); !ok || chef.Name != "chef" || chef.Prompt != "Run the kitchen." {
		t.Errorf("the Kitchen team's coordinator: %+v", chef)
	}
	if cook, _ := roster.Get("cook"); cook.Description != "Cooks the dishes" || cook.Color != "teal" {
		t.Errorf("the cook: %+v", cook)
	}
	if assignees := roster.Assignees("Kitchen"); len(assignees) != 2 {
		t.Errorf("the Kitchen team's agents: %+v", assignees)
	}
	if roster.TemplateOf("Kitchen") != "" {
		t.Errorf("a custom team has no template, got %q", roster.TemplateOf("Kitchen"))
	}
}

// A team copied from a built-in template is made from it, every agent,
// whatever template an agent sent says.
func TestACopiedTeamIsMadeFromItsTemplate(t *testing.T) {
	members := crew()
	members[1].Template = "software"
	roster := FromEvents(seeded())
	for _, p := range CustomTeam("Kitchen", "visionaries", members, you) {
		roster.Apply(events.Event{Proposal: p})
	}
	for _, agent := range roster {
		if agent.TeamName() == "Kitchen" && agent.Template != "visionaries" {
			t.Errorf("%s is made from %q", agent.Name, agent.Template)
		}
	}
	if roster.TemplateOf("Kitchen") != "visionaries" {
		t.Errorf("the Kitchen team is made from %q", roster.TemplateOf("Kitchen"))
	}
}

func TestACustomTeamIsTrimmed(t *testing.T) {
	members := crew()
	members[1].Engine, members[1].Model, members[1].Profile = " claude ", " opus\t", " developer "
	members[1].Color, members[2].Column = " teal ", " review "
	if problem := CheckCustomTeam("Kitchen", members, nil); problem != "" {
		t.Fatalf("a padded column and colour were refused: %s", problem)
	}
	proposals := CustomTeam("Kitchen", "", members, you)
	cook, taster := proposals[1], proposals[2]
	if cook.Text("engine") != "claude" || cook.Text("model") != "opus" || cook.Text("profile") != "developer" || cook.Text("color") != "teal" {
		t.Errorf("the cook's proposal wasn't trimmed: %v", cook.Data)
	}
	if taster.Text("column") != Reviews {
		t.Errorf("the taster's column: %q", taster.Text("column"))
	}
}

func TestACustomTeamGetsDefaultPrompts(t *testing.T) {
	members := crew()
	members[0].Prompt, members[1].Prompt, members[2].Prompt = "", " ", ""
	members[1].Description = "Cooks the dishes."
	proposals := CustomTeam("Kitchen", "", members, you)
	prompts := map[string]string{}
	for _, p := range proposals {
		prompts[p.Text("name")] = p.Text("prompt")
	}
	if chef := prompts["chef"]; !strings.HasPrefix(chef, "You are the coordinator") || strings.Contains(chef, "code") {
		t.Errorf("the coordinator's default prompt: %q", chef)
	}
	cook := prompts["cook"]
	if !strings.Contains(cook, "cook, one of the agents of the Kitchen team") || !strings.Contains(cook, "Your job: Cooks the dishes.\n") {
		t.Errorf("the cook's default prompt: %q", cook)
	}
	if strings.Contains(cook, "Verdict") || !strings.Contains(prompts["taster"], "`Verdict: approved`") {
		t.Errorf("only a reviewer ends with a verdict:\n%s\n%s", cook, prompts["taster"])
	}

	given := CustomTeam("Kitchen", "", crew(), you)
	if given[1].Text("prompt") != "Cook." {
		t.Errorf("a given prompt was replaced: %q", given[1].Text("prompt"))
	}
}

func TestAnAgentWithNoDescriptionStillGetsAPrompt(t *testing.T) {
	prompt := defaultPrompt(Agent{Name: "helper", Team: "Kitchen"})
	if !strings.Contains(prompt, "Your job: what your coordinator asks of you.") {
		t.Errorf("prompt: %q", prompt)
	}
}
