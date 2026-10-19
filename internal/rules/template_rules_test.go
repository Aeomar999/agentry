package rules

import (
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
)

// research is a template's agents that the rules accept, to break one way
// in each test.
func research() []agents.Agent {
	return []agents.Agent{
		{Name: "lead", Prompt: "Hand out the reading.", Engine: "claude", Coordinates: true, Color: "teal"},
		{Name: "reader", Prompt: "Read the papers.", Engine: "codex", Column: agents.Reviews},
	}
}

func savingTemplate(id, team string, members []agents.Agent) events.Proposal {
	return events.Proposal{Stream: events.TemplatesStream, Actor: you, Type: events.TemplateSaved,
		Data: map[string]any{"id": id, "team": team, "description": "Reads papers.", "agents": members}}
}

// proposeTemplate proposes p to a seeded log and returns why it was refused,
// or "" when it was accepted.
func proposeTemplate(t *testing.T, p events.Proposal) string {
	t.Helper()
	_, rejection, err := openSeeded(t).Propose(p)
	if err != nil {
		t.Fatal(err)
	}
	if rejection == nil {
		return ""
	}
	return rejection.Reason
}

func refusedSaying(t *testing.T, p events.Proposal, want string) {
	t.Helper()
	if reason := proposeTemplate(t, p); !strings.Contains(reason, want) {
		t.Errorf("refused with %q, want %q", reason, want)
	}
}

func TestATemplateIsSaved(t *testing.T) {
	if reason := proposeTemplate(t, savingTemplate("papers", "Readers", research())); reason != "" {
		t.Errorf("a good template was refused: %s", reason)
	}
}

func TestATemplateReadBackFromTheLogIsChecked(t *testing.T) {
	logged := []any{
		map[string]any{"name": "lead", "prompt": "Lead.", "engine": "claude", "coordinates": true},
		map[string]any{"name": "lead", "prompt": "Lead again.", "engine": "claude"},
	}
	p := savingTemplate("papers", "Readers", nil)
	p.Data["agents"] = logged
	refusedSaying(t, p, "two agents called lead")
}

func TestATemplateNeedsAnID(t *testing.T) {
	refusedSaying(t, savingTemplate(" ", "Readers", research()), "needs an id")
}

func TestATemplateNeedsATeam(t *testing.T) {
	refusedSaying(t, savingTemplate("papers", "", research()), "needs a team name")
}

// AddTeam gives a template's team name to its agents as it is, so a padded
// one is refused whichever way the template came.
func TestATemplatesTeamNameIsTrimmed(t *testing.T) {
	refusedSaying(t, savingTemplate("papers", " Readers ", research()), `a template's team name has no spaces at its ends: " Readers "`)
}

// A team is never added under a built-in template's team name, whatever
// its case, so a template is not saved under one; a numbered one is fine.
func TestATemplatesTeamIsNotABuiltInTeam(t *testing.T) {
	for _, team := range []string{"Writing", "writing"} {
		t.Run(team, func(t *testing.T) {
			refusedSaying(t, savingTemplate("papers", team, research()), team+" is the name of the Writing template's team: rename the team before saving it as a template")
		})
	}
	if reason := proposeTemplate(t, savingTemplate("papers", "Writing 2", research())); reason != "" {
		t.Errorf("a numbered team was refused: %s", reason)
	}
}

func TestATemplateCantTakeABuiltInID(t *testing.T) {
	for _, template := range agents.Templates {
		t.Run(template.ID, func(t *testing.T) {
			refusedSaying(t, savingTemplate(template.ID, "Readers", research()), "the built-in "+template.ID+" template has that id")
		})
	}
}

func TestATemplateIDIsLowercaseLettersDigitsAndDashes(t *testing.T) {
	for _, id := range []string{" research", "My Team", "Papers", "a/b", "papers-" + strings.Repeat("a", 25)} {
		t.Run(id, func(t *testing.T) {
			refusedSaying(t, savingTemplate(id, "Readers", research()), "a template's id is lowercase letters, digits and dashes, starting with a letter, at most 31 characters: "+id)
		})
	}
}

func TestATemplatesAgentsAreAList(t *testing.T) {
	p := savingTemplate("papers", "Readers", nil)
	p.Data["agents"] = "lead, reader"
	refusedSaying(t, p, "a list of agents")
}

func TestATemplateNeedsAnAgent(t *testing.T) {
	refusedSaying(t, savingTemplate("papers", "Readers", nil), "at least one agent")
}

func TestATemplateNeedsACoordinator(t *testing.T) {
	members := research()
	members[0].Coordinates = false
	refusedSaying(t, savingTemplate("papers", "Readers", members), "exactly one coordinator")
}

func TestATemplateHasOnlyOneCoordinator(t *testing.T) {
	members := research()
	members[1].Coordinates = true
	refusedSaying(t, savingTemplate("papers", "Readers", members), "exactly one coordinator")
}

func TestATemplatesAgentNamesDiffer(t *testing.T) {
	members := research()
	members[1].Name = "lead"
	refusedSaying(t, savingTemplate("papers", "Readers", members), "two agents called lead")
}

func TestATemplatesAgentHasAValidName(t *testing.T) {
	members := research()
	members[1].Name = "Reader"
	refusedSaying(t, savingTemplate("papers", "Readers", members), "lowercase letters")
}

func TestATemplatesAgentNameLeavesRoomForANumber(t *testing.T) {
	members := research()
	members[1].Name = "reader-" + strings.Repeat("a", 22) // 29 characters
	refusedSaying(t, savingTemplate("papers", "Readers", members), "agent "+members[1].Name+": a template's agent names are at most 28 characters")
}

func TestATemplatesAgentNameMayBe28Characters(t *testing.T) {
	members := research()
	members[1].Name = "reader-" + strings.Repeat("a", 21)
	if reason := proposeTemplate(t, savingTemplate("papers", "Readers", members)); reason != "" {
		t.Errorf("a 28-character agent name was refused: %s", reason)
	}
}

// A template whose names are as long as its rules allow is added twice,
// through the agent rules: the second team's agents take a number and still
// have valid names.
func TestATemplateWithTheLongestNamesIsAddedTwice(t *testing.T) {
	members := research()
	members[0].Name = "lead-" + strings.Repeat("a", 23) // 28 characters
	s, state := openSeededState(t)
	if _, rejection, err := s.Propose(savingTemplate("papers", "Papers", members)); err != nil || rejection != nil {
		t.Fatalf("saving: %v, %v", rejection, err)
	}
	saved, _ := state.Templates().WithID("papers")
	for range 2 {
		for _, p := range agents.AddTeam(saved.Template(), state.Roster(), you) {
			if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
				t.Fatalf("adding %s: %v, %v", p.Text("name"), rejection, err)
			}
		}
	}
	if lead, ok := state.Roster().Coordinator("Papers 2"); !ok || lead.Name != members[0].Name+"-2" {
		t.Errorf("the second team's lead: %+v", lead)
	}
	if made := len(state.Roster().Assignees("Papers 2")) + 1; made != len(members) {
		t.Errorf("the second Papers team has %d agents, want %d", made, len(members))
	}
}

func TestATemplatesAgentKeepsNothingOfAnInstall(t *testing.T) {
	for _, test := range []struct {
		field string
		set   func(*agents.Agent)
		want  string
	}{
		{"workers", func(a *agents.Agent) { a.Workers = []string{"laptop"} }, "agent reader: a template's agents name no workers"},
		{"knowledge", func(a *agents.Agent) { a.Knowledge = "Skim the abstract first." }, "agent reader: a template's agents carry no knowledge"},
		{"team", func(a *agents.Agent) { a.Team = "Readers" }, "agent reader: a template's agents name no team"},
		{"template", func(a *agents.Agent) { a.Template = "research" }, "agent reader: a template's agents name no template"},
	} {
		t.Run(test.field, func(t *testing.T) {
			members := research()
			test.set(&members[1])
			refusedSaying(t, savingTemplate("papers", "Readers", members), test.want)
		})
	}
}

func TestATemplatesAgentHasAKnownEngine(t *testing.T) {
	members := research()
	members[1].Engine = "gpt"
	refusedSaying(t, savingTemplate("papers", "Readers", members), "agent reader: unknown engine gpt")
}

func TestATemplatesAgentHasAKnownColour(t *testing.T) {
	members := research()
	members[1].Color = "mauve"
	refusedSaying(t, savingTemplate("papers", "Readers", members), "unknown colour mauve")
}

func TestATemplatesAgentHasAKnownColumn(t *testing.T) {
	members := research()
	members[1].Column = "deploy"
	refusedSaying(t, savingTemplate("papers", "Readers", members), "not deploy")
}

func TestATemplatesAgentHasAPrompt(t *testing.T) {
	members := research()
	members[1].Prompt = "  "
	refusedSaying(t, savingTemplate("papers", "Readers", members), "agent reader: an agent needs a prompt")
}

func removingTemplate(id string) events.Proposal {
	return events.Proposal{Stream: events.TemplatesStream, Actor: you, Type: events.TemplateRemoved, Data: map[string]any{"id": id}}
}

// reasonsFor proposes each of ps in turn to one seeded log, and gives why
// each was refused, or "" for one accepted.
func reasonsFor(t *testing.T, ps ...events.Proposal) []string {
	t.Helper()
	s := openSeeded(t)
	reasons := make([]string, len(ps))
	for i, p := range ps {
		_, rejection, err := s.Propose(p)
		if err != nil {
			t.Fatal(err)
		}
		if rejection != nil {
			reasons[i] = rejection.Reason
		}
	}
	return reasons
}

func TestASavedTemplateIsRemoved(t *testing.T) {
	reasons := reasonsFor(t, savingTemplate("papers", "Readers", research()), removingTemplate("papers"))
	if reasons[1] != "" {
		t.Errorf("removing a saved template was refused: %s", reasons[1])
	}
}

func TestATemplateNeverSavedIsNotRemoved(t *testing.T) {
	refusedSaying(t, removingTemplate("papers"), "no saved template papers")
}

func TestATemplateIsRemovedOnce(t *testing.T) {
	reasons := reasonsFor(t, savingTemplate("papers", "Readers", research()), removingTemplate("papers"), removingTemplate("papers"))
	if reasons[1] != "" || !strings.Contains(reasons[2], "no saved template papers") {
		t.Errorf("removing twice: %q, want the second refused", reasons)
	}
}

func TestARemovalNeedsAnID(t *testing.T) {
	refusedSaying(t, removingTemplate(""), "needs an id")
}

func TestASavedTeamIsAccepted(t *testing.T) {
	s, state := openSeededState(t)
	for _, p := range agents.CustomTeam("Readers", research(), you) {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatalf("adding the Readers team: %v, %v", rejection, err)
		}
	}
	p, err := agents.SaveTemplate(state.Roster(), "Readers", "mine", "My readers.", you)
	if err != nil {
		t.Fatal(err)
	}
	if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
		t.Errorf("saving the Readers team: %v, %v", rejection, err)
	}
	if saved, ok := state.Templates().WithID("mine"); !ok || len(saved.Agents) == 0 {
		t.Errorf("the saved template: %+v", saved)
	}
}
