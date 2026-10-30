package agents

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// papers is a template you saved: its prompts are yours, not in defaults/.
func papers() UserTemplate {
	return UserTemplate{ID: "papers", Team: "Papers", Description: "Reads papers.", Agents: []Agent{
		{Name: "lead", Prompt: "Hand out the reading.", Engine: "claude", Coordinates: true, Color: "teal"},
		{Name: "reader", Prompt: "Read the papers.", Engine: "codex", Column: Reviews},
	}}
}

func added(t Template, log []events.Event) []events.Event {
	for _, p := range AddTeam(t, FromEvents(log), events.Actor{Kind: events.Human, ID: "you"}) {
		log = append(log, events.Event{Proposal: p})
	}
	return log
}

func TestATeamIsAddedFromAUserTemplateWithItsPrompts(t *testing.T) {
	roster := FromEvents(added(papers().Template(), seeded()))
	lead, _ := roster.Coordinator("Papers")
	reader, _ := roster.Get("reader")
	if lead.Name != "lead" || lead.Prompt != "Hand out the reading." || lead.Template != "papers" {
		t.Errorf("the coordinator: %+v", lead)
	}
	if reader.Prompt != "Read the papers." || reader.Team != "Papers" || reader.Template != "papers" || reader.Column != Reviews {
		t.Errorf("the reader: %+v", reader)
	}
}

// The coordinator of a team added from your template is described by the
// template when it has no description of its own, and keeps its own if it has.
func TestAUserTemplatesDescriptionFillsAnEmptyCoordinators(t *testing.T) {
	lead, _ := FromEvents(added(papers().Template(), seeded())).Coordinator("Papers")
	if lead.Description != "Reads papers." {
		t.Errorf("an empty description is filled: %q", lead.Description)
	}
	described := papers()
	described.Agents = slices.Clone(described.Agents)
	described.Agents[0].Description = "Hands out the reading."
	lead, _ = FromEvents(added(described.Template(), seeded())).Coordinator("Papers")
	reader, _ := FromEvents(added(described.Template(), seeded())).Get("reader")
	if lead.Description != "Hands out the reading." || reader.Description != "" {
		t.Errorf("its own description is kept, and only the coordinator's is filled: %q, %q", lead.Description, reader.Description)
	}
}

func TestAUserTemplateAddedAgainGetsNumbers(t *testing.T) {
	log := added(papers().Template(), added(papers().Template(), seeded()))
	again := FromEvents(log)
	lead, _ := again.Coordinator("Papers 2")
	reader, _ := again.Get("reader-2")
	if lead.Name != "lead-2" || reader.Team != "Papers 2" || reader.Prompt != "Read the papers." {
		t.Errorf("a second Papers team: %v, %+v, %+v", again.Teams(), lead, reader)
	}
}

func TestASavedTeamIsAddedBackTheSame(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	log := seeded()
	for _, a := range papers().Agents {
		a.Team, a.Workers, a.Model, a.Profile = "Papers", []string{"laptop"}, "opus", "developer"
		log = append(log, events.Event{Proposal: a.Proposal(you, false)})
	}
	log = append(log, events.Event{Proposal: Agent{Name: "reader"}.Learned(you, "Skim the abstract first.")})
	made := FromEvents(log)

	saving, err := SaveTemplate(made, nil, nil, "Papers", "Papers", "papers", "Reads papers.", you)
	if err != nil {
		t.Fatal(err)
	}
	saved, ok := UserTemplatesFromEvents([]events.Event{logged(t, saving)}).WithID("papers")
	if !ok || saved.Team != "Papers" || saved.Description != "Reads papers." || saved.From != "Papers" {
		t.Fatalf("the saved template: %+v", saved)
	}

	for _, a := range made.Assignees("Papers") {
		log = append(log, events.Event{Proposal: a.Removal(you)})
	}
	lead, _ := made.Coordinator("Papers")
	log = added(saved.Template(), append(log, events.Event{Proposal: lead.Removal(you)}))

	again := FromEvents(log)
	for _, had := range made {
		if had.Team != "Papers" {
			continue
		}
		want := had
		want.Workers, want.Knowledge, want.Template, want.untouched = []string{}, "", "papers", false
		if want.Coordinates {
			want.Description = "Reads papers." // it had none, so it takes the template's
		}
		got, _ := again.Get(had.Name)
		if !equalAgents(got, want) {
			t.Errorf("%s added back:\n got %+v\nwant %+v", had.Name, got, want)
		}
	}
}

func equalAgents(a, b Agent) bool {
	encodedA, _ := json.Marshal(a)
	encodedB, _ := json.Marshal(b)
	return string(encodedA) == string(encodedB)
}

func TestSavingATeamThatIsntThere(t *testing.T) {
	if _, err := SaveTemplate(FromEvents(seeded()), nil, nil, "Papers", "Papers", "papers", "", events.Actor{Kind: events.Human, ID: "you"}); err == nil {
		t.Error("saved a team with no agents")
	}
}

func TestTheDefaultTeamIsSavedByItsName(t *testing.T) {
	saving, err := SaveTemplate(FromEvents(seeded()), nil, nil, DefaultTeam, DefaultTeam, "mine", "", events.Actor{Kind: events.Human, ID: "you"})
	if members, _ := TemplateAgents(saving.Data); err != nil || len(members) != len(defaults) {
		t.Errorf("the %s team saved %d agents (%v)", DefaultTeam, len(members), err)
	}
}

// A template is named apart from the team it was saved from: a team made
// from it gets the name, and the template keeps where it came from and,
// from the log, when it was saved.
func TestATemplateKeepsItsNameItsTeamAndWhenItWasSaved(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	log := added(papers().Template(), seeded())
	saving, err := SaveTemplate(FromEvents(log), nil, nil, "Papers", "Reading Room", "reading-room", "", you)
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 12, 9, 30, 0, 0, time.UTC)
	event := logged(t, saving)
	event.At = at
	saved, _ := UserTemplatesFromEvents([]events.Event{event}).WithID("reading-room")
	if saved.Team != "Reading Room" || saved.From != "Papers" || !saved.SavedAt.Equal(at) {
		t.Errorf("the saved template: %+v", saved)
	}
	lead, _ := FromEvents(added(saved.Template(), log)).Coordinator("Reading Room")
	if lead.Name != "lead-2" {
		t.Errorf("a team made from it is called Reading Room: %+v", lead)
	}
}

// A template.saved from before templates were named has no "from" or
// "basedOn": it replays with neither, its team as it was, and when the log says.
func TestATemplateSavedWithoutFromReplays(t *testing.T) {
	at := time.Date(2025, 3, 1, 8, 0, 0, 0, time.UTC)
	old := events.Event{At: at, Proposal: events.Proposal{Stream: events.TemplatesStream, Type: events.TemplateSaved, Data: map[string]any{
		"id": "papers", "team": "Papers", "description": "Reads papers.", "agents": []any{
			map[string]any{"name": "lead", "prompt": "Hand out the reading.", "engine": "claude", "coordinates": true}},
	}}}
	saved, ok := UserTemplatesFromEvents([]events.Event{old}).WithID("papers")
	if !ok || saved.Team != "Papers" || saved.From != "" || saved.BasedOn != "" || !saved.SavedAt.Equal(at) || len(saved.Agents) != 1 {
		t.Errorf("an old template: %+v", saved)
	}
}

// A template with no source team saves no "from", so an imported file
// from before templates were named writes the event it always did.
func TestATemplateWithNoSourceTeamSavesNoFrom(t *testing.T) {
	if data := papers().Save(events.Actor{Kind: events.Human, ID: "you"}).Data; data["from"] != nil {
		t.Errorf("saved %+v", data)
	}
}

// A save from a team never replaces a template: its event says it is new,
// so the rules refuse a name or id of yours, and an import's does not.
func TestASavedTeamIsANewTemplate(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	saving, err := SaveTemplate(FromEvents(seeded()), nil, nil, DefaultTeam, DefaultTeam, "mine", "", you)
	if err != nil || saving.Data["new"] != true {
		t.Errorf("saved %+v (%v), want new", saving.Data, err)
	}
	if imported := papers().Save(you); imported.Data["new"] != nil {
		t.Errorf("imported %+v, want not new", imported.Data)
	}
}

// A template is based on the built-in template its team is linked to:
// directly, through the base of your template it was added from, or not at
// all for a team you made yourself.
func TestASavedTemplateIsBasedOnTheBuiltInTemplateItsTeamCameFrom(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	ideas := papers()
	ideas.BasedOn = "visionaries"
	fromYours := added(ideas.Template(), seeded())
	madeByHand := seeded()
	for _, a := range papers().Agents {
		a.Team = "Papers"
		madeByHand = append(madeByHand, events.Event{Proposal: a.Proposal(you, false)})
	}
	for name, c := range map[string]struct {
		log  []events.Event
		team string
		want string
	}{
		"a built-in template's team":    {seeded(), DefaultTeam, "software"},
		"a team from yours with a base": {fromYours, "Papers", "visionaries"},
		"a team you made yourself":      {madeByHand, "Papers", ""},
	} {
		saving, err := SaveTemplate(FromEvents(c.log), UserTemplates{ideas}, nil, c.team, "Mine", "mine", "", you)
		if err != nil {
			t.Fatal(err)
		}
		saved, _ := UserTemplatesFromEvents([]events.Event{logged(t, saving)}).WithID("mine")
		if saved.BasedOn != c.want {
			t.Errorf("%s: based on %q, want %q", name, saved.BasedOn, c.want)
		}
		if _, written := saving.Data["basedOn"]; written != (c.want != "") {
			t.Errorf("%s: saved %+v", name, saving.Data)
		}
	}
}

// A team from a template you removed still names its id, so saving it again
// keeps the base the removed template had.
func TestATeamFromARemovedTemplateKeepsItsBase(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	log := added(papers().Template(), seeded())
	saving, err := SaveTemplate(FromEvents(log), nil, RemovedTemplates{"papers": "visionaries"}, "Papers", "Mine", "mine", "", you)
	if err != nil {
		t.Fatal(err)
	}
	if saving.Data["basedOn"] != "visionaries" {
		t.Errorf("saved %+v, want based on visionaries", saving.Data)
	}
}

// Updating a template saves its team's agents as they are now over it,
// keeping its id, name and base, and its description unless another is
// given. The team, Papers, is named apart from its template, Ideas.
func TestATemplateIsUpdatedFromItsTeam(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	roster := FromEvents(added(papers().Template(), seeded()))
	ideas := papers()
	ideas.Team, ideas.BasedOn = "Ideas", "visionaries"
	for i, a := range roster {
		if a.Name == "reader" {
			roster[i].Prompt = "Read the papers twice."
		}
	}
	for _, c := range []struct {
		name, description, want string
	}{
		{"no description", "", "Reads papers."},
		{"a description", "Reads closely.", "Reads closely."},
	} {
		saving, err := UpdateTemplate(roster, UserTemplates{ideas}, nil, "Papers", c.description, you)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		saved, _ := UserTemplatesFromEvents([]events.Event{logged(t, ideas.Save(you)), logged(t, saving)}).WithID("papers")
		reader := saved.Agents[slices.IndexFunc(saved.Agents, func(a Agent) bool { return a.Name == "reader" })]
		if saved.Team != "Ideas" || saved.BasedOn != "visionaries" || saved.Description != c.want || reader.Prompt != "Read the papers twice." {
			t.Errorf("%s: updated to %+v", c.name, saved)
		}
		if !saving.Flag("replacing") || saving.Flag("new") {
			t.Errorf("%s: saved %+v, want replacing", c.name, saving.Data)
		}
	}
}

// Only a team linked to a template of yours you still have has one to
// update.
func TestATeamNotFromATemplateOfYoursHasNoneToUpdate(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	fromPapers := FromEvents(added(papers().Template(), seeded()))
	for _, c := range []struct {
		name   string
		roster Roster
		yours  UserTemplates
		team   string
	}{
		{"a built-in template's team", FromEvents(seeded()), UserTemplates{papers()}, DefaultTeam},
		{"a removed template's team", fromPapers, nil, "Papers"},
		{"no team", fromPapers, UserTemplates{papers()}, "Nowhere"},
	} {
		_, err := UpdateTemplate(c.roster, c.yours, RemovedTemplates{"papers": ""}, c.team, "", you)
		if !errors.Is(err, ErrNoTemplateToUpdate) || !strings.HasPrefix(err.Error(), c.team+" isn't made from a template of yours") {
			t.Errorf("%s: %v", c.name, err)
		}
	}
}

// A removed template keeps the base it had then, with or without one, and
// an import of its id takes it out again.
func TestRemovedTemplatesKeepTheirBaseUntilImported(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	ideas := papers()
	ideas.ID, ideas.Team, ideas.BasedOn = "ideas", "Ideas", "visionaries"
	stream := []events.Event{
		logged(t, papers().Save(you)), logged(t, ideas.Save(you)),
		logged(t, RemoveTemplate("papers", you)), logged(t, RemoveTemplate("ideas", you)),
	}
	removed := RemovedTemplatesFromEvents(stream)
	if want := (RemovedTemplates{"papers": "", "ideas": "visionaries"}); !maps.Equal(removed, want) {
		t.Errorf("removed %v, want %v", removed, want)
	}
	brought := RemovedTemplatesFromEvents(append(stream, logged(t, ideas.Save(you))))
	if want := (RemovedTemplates{"papers": ""}); !maps.Equal(brought, want) {
		t.Errorf("after the import: %v, want %v", brought, want)
	}
	if live := UserTemplatesFromEvents(stream); len(live) != 0 {
		t.Errorf("the templates still hold %+v", live)
	}
}

// Next builds on a copy, as the state hands the old map to readers.
func TestRemovedTemplatesNextLeavesTheOriginal(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	before := RemovedTemplates{"papers": ""}
	after := before.Next(logged(t, papers().Save(you)), nil)
	if len(after) != 0 || !maps.Equal(before, RemovedTemplates{"papers": ""}) {
		t.Errorf("after %v, before %v", after, before)
	}
	ideas := papers()
	ideas.ID, ideas.BasedOn = "ideas", "visionaries"
	after = before.Next(logged(t, RemoveTemplate("ideas", you)), UserTemplates{ideas})
	if after["ideas"] != "visionaries" || len(before) != 1 {
		t.Errorf("after %v, before %v", after, before)
	}
}

// Bases resolve a link to the built-in template its team keeps the extensions
// of: a built-in link itself, yours, live or removed, its base, and anything
// else none.
func TestBasesResolveALinkToItsBuiltInTemplate(t *testing.T) {
	ideas, plain := papers(), papers()
	ideas.ID, ideas.BasedOn = "ideas", "visionaries"
	bases := Bases{Yours: UserTemplates{ideas, plain}, Removed: RemovedTemplates{"gone": "visionaries", "bare": ""}}
	for template, want := range map[string]string{"visionaries": "visionaries", "software": "software", "ideas": "visionaries",
		"papers": "", "gone": "visionaries", "bare": "", "unknown": "", "": ""} {
		if got := bases.Of(template); got != want {
			t.Errorf("the base of %q: %q, want %q", template, got, want)
		}
	}
}

// templates is a state holding your templates, with its templates stream
// broken or not.
type templates struct {
	yours   UserTemplates
	removed RemovedTemplates
	broken  error
}

func (s templates) Templates() UserTemplates           { return s.yours }
func (s templates) RemovedTemplates() RemovedTemplates { return s.removed }
func (s templates) Broken(stream string) error {
	if stream == events.TemplatesStream {
		return s.broken
	}
	return nil
}

// A broken templates stream gives none of yours, so a built-in link still
// resolves.
func TestABrokenTemplatesStreamLeavesOnlyBuiltInBases(t *testing.T) {
	ideas := papers()
	ideas.BasedOn = "visionaries"
	state := templates{yours: UserTemplates{ideas}, removed: RemovedTemplates{"gone": "visionaries"}}
	if bases := BasesIn(state); bases.Of("papers") != "visionaries" || bases.Of("gone") != "visionaries" {
		t.Errorf("whole stream: %+v", bases)
	}
	state.broken = errors.New("a bad event")
	bases := BasesIn(state)
	if bases.Of("papers") != "" || bases.Of("gone") != "" || bases.Of("visionaries") != "visionaries" {
		t.Errorf("broken stream: %+v", bases)
	}
}
