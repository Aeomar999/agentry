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
	if reason := proposeTemplate(t, savingTemplate("papers", "Research", research())); reason != "" {
		t.Errorf("a good template was refused: %s", reason)
	}
}

func TestATemplateReadBackFromTheLogIsChecked(t *testing.T) {
	logged := []any{
		map[string]any{"name": "lead", "prompt": "Lead.", "engine": "claude", "coordinates": true},
		map[string]any{"name": "lead", "prompt": "Lead again.", "engine": "claude"},
	}
	p := savingTemplate("papers", "Research", nil)
	p.Data["agents"] = logged
	refusedSaying(t, p, "two agents called lead")
}

func TestATemplateNeedsAnID(t *testing.T) {
	refusedSaying(t, savingTemplate(" ", "Research", research()), "needs an id")
}

func TestATemplateNeedsATeam(t *testing.T) {
	refusedSaying(t, savingTemplate("papers", "", research()), "needs a team name")
}

func TestATemplateCantTakeABuiltInID(t *testing.T) {
	refusedSaying(t, savingTemplate(agents.Templates[0].ID, "Research", research()), "built-in")
}

func TestATemplatesAgentsAreAList(t *testing.T) {
	p := savingTemplate("papers", "Research", nil)
	p.Data["agents"] = "lead, reader"
	refusedSaying(t, p, "a list of agents")
}

func TestATemplateNeedsAnAgent(t *testing.T) {
	refusedSaying(t, savingTemplate("papers", "Research", nil), "at least one agent")
}

func TestATemplateNeedsACoordinator(t *testing.T) {
	members := research()
	members[0].Coordinates = false
	refusedSaying(t, savingTemplate("papers", "Research", members), "exactly one coordinator")
}

func TestATemplateHasOnlyOneCoordinator(t *testing.T) {
	members := research()
	members[1].Coordinates = true
	refusedSaying(t, savingTemplate("papers", "Research", members), "exactly one coordinator")
}

func TestATemplatesAgentNamesDiffer(t *testing.T) {
	members := research()
	members[1].Name = "lead"
	refusedSaying(t, savingTemplate("papers", "Research", members), "two agents called lead")
}

func TestATemplatesAgentHasAValidName(t *testing.T) {
	members := research()
	members[1].Name = "Reader"
	refusedSaying(t, savingTemplate("papers", "Research", members), "lowercase letters")
}

func TestATemplatesAgentNameLeavesRoomForANumber(t *testing.T) {
	members := research()
	members[1].Name = "reader-" + strings.Repeat("a", 22) // 29 characters
	refusedSaying(t, savingTemplate("papers", "Research", members), "agent "+members[1].Name+": a template's agent names are at most 28 characters")
}

func TestATemplatesAgentNameMayBe28Characters(t *testing.T) {
	members := research()
	members[1].Name = "reader-" + strings.Repeat("a", 21)
	if reason := proposeTemplate(t, savingTemplate("papers", "Research", members)); reason != "" {
		t.Errorf("a 28-character agent name was refused: %s", reason)
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
		{"team", func(a *agents.Agent) { a.Team = "Research" }, "agent reader: a template's agents name no team"},
		{"template", func(a *agents.Agent) { a.Template = "research" }, "agent reader: a template's agents name no template"},
	} {
		t.Run(test.field, func(t *testing.T) {
			members := research()
			test.set(&members[1])
			refusedSaying(t, savingTemplate("papers", "Research", members), test.want)
		})
	}
}

func TestATemplatesAgentHasAKnownEngine(t *testing.T) {
	members := research()
	members[1].Engine = "gpt"
	refusedSaying(t, savingTemplate("papers", "Research", members), "agent reader: unknown engine gpt")
}

func TestATemplatesAgentHasAKnownColour(t *testing.T) {
	members := research()
	members[1].Color = "mauve"
	refusedSaying(t, savingTemplate("papers", "Research", members), "unknown colour mauve")
}

func TestATemplatesAgentHasAKnownColumn(t *testing.T) {
	members := research()
	members[1].Column = "deploy"
	refusedSaying(t, savingTemplate("papers", "Research", members), "not deploy")
}

func TestATemplatesAgentHasAPrompt(t *testing.T) {
	members := research()
	members[1].Prompt = "  "
	refusedSaying(t, savingTemplate("papers", "Research", members), "agent reader: an agent needs a prompt")
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
	reasons := reasonsFor(t, savingTemplate("papers", "Research", research()), removingTemplate("papers"))
	if reasons[1] != "" {
		t.Errorf("removing a saved template was refused: %s", reasons[1])
	}
}

func TestATemplateNeverSavedIsNotRemoved(t *testing.T) {
	refusedSaying(t, removingTemplate("papers"), "no saved template papers")
}

func TestATemplateIsRemovedOnce(t *testing.T) {
	reasons := reasonsFor(t, savingTemplate("papers", "Research", research()), removingTemplate("papers"), removingTemplate("papers"))
	if reasons[1] != "" || !strings.Contains(reasons[2], "no saved template papers") {
		t.Errorf("removing twice: %q, want the second refused", reasons)
	}
}

func TestARemovalNeedsAnID(t *testing.T) {
	refusedSaying(t, removingTemplate(""), "needs an id")
}

func TestASavedTeamIsAccepted(t *testing.T) {
	s, state := openSeededState(t)
	p, err := agents.SaveTemplate(state.Roster(), agents.DefaultTeam, "mine", "My software team.", you)
	if err != nil {
		t.Fatal(err)
	}
	if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
		t.Errorf("saving the %s team: %v, %v", agents.DefaultTeam, rejection, err)
	}
	if saved, ok := state.Templates().WithID("mine"); !ok || len(saved.Agents) == 0 {
		t.Errorf("the saved template: %+v", saved)
	}
}
