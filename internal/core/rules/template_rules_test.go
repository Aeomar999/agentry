package rules

import (
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/core/events"
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

// A template is based on a built-in template or on none, as one saved
// before templates had a base was.
func TestATemplateIsBasedOnABuiltInTemplate(t *testing.T) {
	based := savingTemplate("papers", "Readers", research())
	based.Data["basedOn"] = "visionaries"
	if reason := proposeTemplate(t, based); reason != "" {
		t.Errorf("a template based on a built-in one was refused: %s", reason)
	}
	unknown := savingTemplate("papers", "Readers", research())
	unknown.Data["basedOn"] = "dreamers"
	refusedSaying(t, unknown, "there is no built-in template dreamers")
}

// A template of yours has no extensions of its own to pass on, so another
// isn't based on it: SaveTemplate takes its base instead.
func TestATemplateIsNotBasedOnOneOfYours(t *testing.T) {
	s := openSeeded(t)
	if _, rejection, err := s.Propose(savingTemplate("papers", "Readers", research())); err != nil || rejection != nil {
		t.Fatal(rejection, err)
	}
	onYours := savingTemplate("more-papers", "More Readers", research())
	onYours.Data["basedOn"] = "papers"
	_, rejection, err := s.Propose(onYours)
	if err != nil {
		t.Fatal(err)
	}
	if rejection == nil || !strings.Contains(rejection.Reason, "there is no built-in template papers") {
		t.Errorf("refused with %v, want no built-in template papers", rejection)
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

// newTemplate is savingTemplate as a save from a team makes it: never
// replacing one of yours.
func newTemplate(id, team string) events.Proposal {
	p := savingTemplate(id, team, research())
	p.Data["new"] = true
	return p
}

// Two saves sent at once both pass the web's check, so the rules refuse the
// second, by name in any case or by id, and the first stays.
func TestANewTemplateTakesNoNameOrIDOfYours(t *testing.T) {
	for _, second := range []struct {
		name string
		p    events.Proposal
		want string
	}{
		{"the same name", newTemplate("papers-2", "Readers"), "you have a template called Readers already"},
		{"the name in another case", newTemplate("papers-2", "READERS"), "you have a template called Readers already"},
		{"the same id", newTemplate("papers", "Others"), "your template Readers has the id papers"},
	} {
		t.Run(second.name, func(t *testing.T) {
			s := openSeeded(t)
			if _, rejection, err := s.Propose(newTemplate("papers", "Readers")); rejection != nil || err != nil {
				t.Fatalf("the first save: %v %v", rejection, err)
			}
			_, rejection, err := s.Propose(second.p)
			if err != nil || rejection == nil || rejection.Reason != second.want {
				t.Errorf("the second save: %v %v, want refused with %q", rejection, err, second.want)
			}
		})
	}
}

// An import replaces the template with its id, and a save already in the
// log replays as it did, so only a new save is refused.
func TestASaveThatIsntNewReplacesTheTemplateWithItsID(t *testing.T) {
	s := openSeeded(t)
	for _, p := range []events.Proposal{newTemplate("papers", "Readers"), savingTemplate("papers", "Readers", research())} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatalf("save: %v %v", rejection, err)
		}
	}
}

// replacingTemplate is savingTemplate as an update of your template id
// makes it.
func replacingTemplate(id, team string) events.Proposal {
	p := savingTemplate(id, team, research())
	p.Data["replacing"] = true
	return p
}

// An update replaces a live template of yours, under a name no other of
// yours has; it is never new as well. A save with neither flag, as older
// logs hold, still replaces whatever has its id.
func TestAnUpdateReplacesALiveTemplateOfYours(t *testing.T) {
	both := replacingTemplate("papers", "Readers")
	both.Data["new"] = true
	for _, c := range []struct {
		name string
		p    events.Proposal
		want string
	}{
		{"a live template", replacingTemplate("papers", "Readers"), ""},
		{"a live template, renamed", replacingTemplate("papers", "Reading Room"), ""},
		{"an unknown one", replacingTemplate("letters", "Letters"), "you have no template letters to update"},
		{"a removed one", replacingTemplate("gone", "Gone"), "you removed the template gone, so there is none to update: import its file to bring it back"},
		{"another's name", replacingTemplate("papers", "writers"), "you have a template called Writers already"},
		{"new as well", both, "a template is saved as a new one or replacing one of yours, not both"},
		{"neither, as older logs hold", savingTemplate("papers", "Readers", research()), ""},
		{"neither, on a removed id", savingTemplate("gone", "Gone", research()), ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			s := openSeeded(t)
			for _, first := range []events.Proposal{newTemplate("papers", "Readers"), newTemplate("writing-room", "Writers"),
				newTemplate("gone", "Gone"), removingTemplate("gone")} {
				if _, rejection, err := s.Propose(first); rejection != nil || err != nil {
					t.Fatalf("the first saves: %v %v", rejection, err)
				}
			}
			_, rejection, err := s.Propose(c.p)
			reason := ""
			if rejection != nil {
				reason = rejection.Reason
			}
			if err != nil || reason != c.want {
				t.Errorf("the update: %q %v, want %q", reason, err, c.want)
			}
		})
	}
}

// Two imports sent at once both pass the web's check, so the rules refuse
// a name, in any case, that another of yours has; the template with the id
// is replaced, under its own name or a new one.
func TestAnImportTakesNoNameOfAnotherOfYours(t *testing.T) {
	for _, second := range []struct {
		name string
		p    events.Proposal
		want string
	}{
		{"the same name, another id", savingTemplate("papers-2", "Readers", research()), "you have a template called Readers already"},
		{"the name in another case", savingTemplate("papers-2", "READERS", research()), "you have a template called Readers already"},
		{"the same id, another's name", savingTemplate("papers", "Writers", research()), "you have a template called Writers already"},
		{"the same id and name", savingTemplate("papers", "Readers", research()), ""},
		{"the same id, a new name", savingTemplate("papers", "Reading Room", research()), ""},
	} {
		t.Run(second.name, func(t *testing.T) {
			s := openSeeded(t)
			for _, first := range []events.Proposal{newTemplate("papers", "Readers"), newTemplate("writing-room", "Writers")} {
				if _, rejection, err := s.Propose(first); rejection != nil || err != nil {
					t.Fatalf("the first saves: %v %v", rejection, err)
				}
			}
			_, rejection, err := s.Propose(second.p)
			reason := ""
			if rejection != nil {
				reason = rejection.Reason
			}
			if err != nil || reason != second.want {
				t.Errorf("the import: %q %v, want %q", reason, err, second.want)
			}
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
	for _, p := range agents.CustomTeam("Readers", "", research(), you) {
		if _, rejection, err := s.Propose(p); err != nil || rejection != nil {
			t.Fatalf("adding the Readers team: %v, %v", rejection, err)
		}
	}
	p, err := agents.SaveTemplate(state.Roster(), state.Templates(), state.RemovedTemplates(), "Readers", "Readers", "mine", "My readers.", you)
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

// A removed template's id stays taken by the teams that still name it: a
// new save under it is refused, as two at once could both pass the web's
// check, and an import brings the template back.
func TestARemovedTemplatesIDIsOnlyImported(t *testing.T) {
	reasons := reasonsFor(t, newTemplate("papers", "Readers"), removingTemplate("papers"),
		newTemplate("papers", "Others"), savingTemplate("papers", "Readers", research()))
	if reasons[2] != "you removed a template with the id papers: import its file to bring it back" || reasons[3] != "" {
		t.Errorf("after removing papers: %q, want the new save refused and the import taken", reasons)
	}
}
