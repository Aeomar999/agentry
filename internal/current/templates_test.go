package current

import (
	"errors"
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

var (
	chief    = agents.Agent{Name: "chief", Prompt: "Hand the work on.", Engine: "claude", Coordinates: true, Color: "teal"}
	courier  = agents.Agent{Name: "courier", Prompt: "Take the papers.", Engine: "claude", Profile: "safe", Column: agents.Ships}
	redrafts = agents.Agent{Name: "courier", Prompt: "Take the papers, signed.", Engine: "codex"}
)

func saved(id, description string, members ...agents.Agent) events.Proposal {
	return proposal(events.TemplatesStream, you, events.TemplateSaved,
		map[string]any{"id": id, "team": "Papers", "description": description, "agents": members})
}

func removed(id string) events.Proposal {
	return proposal(events.TemplatesStream, you, events.TemplateRemoved, map[string]any{"id": id})
}

// Each step's log, loaded at every split and applied after, keeps the
// templates the step says, in the order they were first saved, each with
// when it was last saved.
func TestUserTemplatesKeepTheirSaveOrder(t *testing.T) {
	at := func(step int) time.Time { return logOf(make([]events.Proposal, step+1)...)[step].At }
	papers := agents.UserTemplate{ID: "papers", Team: "Papers", Description: "First", Agents: []agents.Agent{chief, courier}, SavedAt: at(0)}
	again := agents.UserTemplate{ID: "papers", Team: "Papers", Description: "Again", Agents: []agents.Agent{chief, redrafts}, SavedAt: at(2)}
	orders := agents.UserTemplate{ID: "orders", Team: "Papers", Agents: []agents.Agent{chief}, SavedAt: at(1)}
	papersLast := papers
	papersLast.SavedAt = at(4)
	steps := []struct {
		name string
		p    events.Proposal
		want agents.UserTemplates
	}{
		{"save", saved("papers", "First", chief, courier), agents.UserTemplates{papers}},
		{"save another", saved("orders", "", chief), agents.UserTemplates{papers, orders}},
		{"save the first again", saved("papers", "Again", chief, redrafts), agents.UserTemplates{again, orders}},
		{"remove the first", removed("papers"), agents.UserTemplates{orders}},
		{"save the first once more", saved("papers", "First", chief, courier), agents.UserTemplates{orders, papersLast}},
		{"remove the other", removed("orders"), agents.UserTemplates{papersLast}},
		{"remove the last", removed("papers"), agents.UserTemplates{}},
	}
	var proposals []events.Proposal
	for _, step := range steps {
		proposals = append(proposals, step.p)
		log := logOf(proposals...)
		for split := range len(log) + 1 {
			s := loaded(log[:split])
			for _, e := range log[split:] {
				s.Apply(e)
			}
			if got := s.Templates(); !reflect.DeepEqual(got, step.want) {
				t.Fatalf("%s, loading %d of %d events:\n%+v\nwant %+v", step.name, split, len(log), got, step.want)
			}
		}
	}
}

func TestALogWithoutTemplatesHasNone(t *testing.T) {
	if got := loaded(everyStream()).Templates(); got != nil {
		t.Errorf("templates from a log with none: %+v", got)
	}
}

// Changing what Templates gave, down to an agent's prompt, changes
// nothing the state holds, and a later event changes nothing given before.
func TestTemplatesHandedOutShareNothing(t *testing.T) {
	log := logOf(saved("papers", "First", chief, redrafts))
	s := loaded(log)
	given := s.Templates()
	given[0].Description, given[0].Agents[1].Prompt = "changed", "changed"
	if want := agents.UserTemplatesFromEvents(log); !reflect.DeepEqual(s.Templates(), want) {
		t.Errorf("changing what was handed out changed the state:\n%+v", s.Templates())
	}

	before := s.Templates()
	s.Apply(events.Event{Position: 2, Proposal: saved("papers", "Again", chief)})
	if want := agents.UserTemplatesFromEvents(log); !reflect.DeepEqual(before, want) {
		t.Errorf("saving again changed what was handed out:\n%+v", before)
	}
}

// Templates saved before a snapshot come back from it, with the team each
// was saved from and when, and the events after it, saving the first again
// and removing the other, apply on top: the state equals a replay of the
// whole log.
func TestTemplatesComeBackFromASnapshot(t *testing.T) {
	orders := saved("orders", "", chief)
	orders.Data["from"] = "Orders desk"
	path, log := seededLog(t, saved("papers", "First", chief, courier), orders)
	cut := log[len(log)-1].Position
	snapshot := snapshotted(t, path, cut, nil)
	s, err := store.Open(path, withIdeas, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	propose(t, s, saved("papers", "Again", chief, redrafts))
	propose(t, s, removed("orders"))
	s.Close()

	restored := Restore(snapshot, "build-1", withIdeas, ideas.Streams())
	if want := agents.UserTemplatesFromEvents(log); !reflect.DeepEqual(restored.Templates(), want) {
		t.Errorf("the snapshot holds the templates\n%+v\nwant %+v", restored.Templates(), want)
	}
	again, err := store.Open(path, withIdeas, restored, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if restored.resumed != cut {
		t.Fatalf("Open replayed the whole log instead of going on from the snapshot at %d", cut)
	}
	if err := Equal(again, restored); err != nil {
		t.Error(err)
	}
	stream, err := again.Stream(events.TemplatesStream)
	if err != nil {
		t.Fatal(err)
	}
	savedAgain := stream[2].At
	want := agents.UserTemplates{{ID: "papers", Team: "Papers", Description: "Again", Agents: []agents.Agent{chief, redrafts}, SavedAt: savedAgain}}
	if got := restored.Templates(); !reflect.DeepEqual(got, want) {
		t.Errorf("after the snapshot's tail:\n%+v\nwant %+v", got, want)
	}
}

// Removing a template keeps its id and base, at every split of the log,
// until an import saves it again.
func TestRemovedTemplatesKeepTheirBase(t *testing.T) {
	based := saved("ideas", "", chief)
	based.Data["basedOn"] = "visionaries"
	steps := []struct {
		name string
		p    events.Proposal
		want agents.RemovedTemplates
	}{
		{"save", saved("papers", "First", chief), nil},
		{"save one with a base", based, nil},
		{"remove the first", removed("papers"), agents.RemovedTemplates{"papers": ""}},
		{"remove the one with a base", removed("ideas"), agents.RemovedTemplates{"papers": "", "ideas": "visionaries"}},
		{"import the first", saved("papers", "First", chief), agents.RemovedTemplates{"ideas": "visionaries"}},
	}
	var proposals []events.Proposal
	for _, step := range steps {
		proposals = append(proposals, step.p)
		log := logOf(proposals...)
		for split := range len(log) + 1 {
			s := loaded(log[:split])
			for _, e := range log[split:] {
				s.Apply(e)
			}
			if got := s.RemovedTemplates(); !maps.Equal(got, step.want) {
				t.Fatalf("%s, loading %d of %d events: %v, want %v", step.name, split, len(log), got, step.want)
			}
		}
	}
}

// The removed templates come back from a snapshot, unless the templates
// stream was broken: Resume rebuilds that from the log.
func TestRemovedTemplatesComeBackFromASnapshot(t *testing.T) {
	path, log := seededLog(t, saved("papers", "First", chief), removed("papers"))
	cut := log[len(log)-1].Position
	restored := Restore(snapshotted(t, path, cut, nil), "build-1", withIdeas, ideas.Streams())
	if got, want := restored.RemovedTemplates(), (agents.RemovedTemplates{"papers": ""}); !maps.Equal(got, want) {
		t.Errorf("the snapshot holds %v, want %v", got, want)
	}

	breaks := func(s *State) { s.broken[events.TemplatesStream] = errors.New("broken") }
	restored = Restore(snapshotted(t, path, cut, breaks), "build-1", withIdeas, ideas.Streams())
	if got := restored.RemovedTemplates(); got != nil {
		t.Errorf("a broken templates stream's snapshot holds %v", got)
	}
}

// Equal compares the removed templates with a replay, apart from the
// templates themselves.
func TestEqualSeesWrongRemovedTemplates(t *testing.T) {
	s := openStore(t, saved("papers", "First", chief), removed("papers"))
	state := New(withIdeas, ideas.Streams())
	state.Loaded(0)
	fromTheLog(t, s, state)
	if err := Equal(s, state); err != nil {
		t.Fatal(err)
	}
	state.removed = agents.RemovedTemplates{"papers": "visionaries"}
	if err := Equal(s, state); err == nil || !strings.HasPrefix(err.Error(), "templates: ") {
		t.Errorf("Equal with a wrong base: %v", err)
	}
}

// A snapshot thrown away takes its removed templates with it, even when
// the log has no templates stream to build them again.
func TestAThrownAwaySnapshotLeavesNoRemovedTemplates(t *testing.T) {
	created := func(id string) events.Proposal {
		return proposal(id, you, events.TaskCreated, map[string]any{"title": "One", "repo": "/r", "coordinator": "tech-lead"})
	}
	logPath, _ := seededLog(t, created("task-1"), created("task-2"))
	shorterPath, _ := seededLog(t, created("task-1"))
	path := snapshotted(t, logPath, 2, func(s *State) { s.removed = agents.RemovedTemplates{"ghost": ""} })
	state, _ := opened(t, shorterPath, path, "build-1")
	if got := state.RemovedTemplates(); got != nil {
		t.Errorf("after a whole replay: %v", got)
	}
}

// Changing what RemovedTemplates gave changes nothing the state holds.
func TestRemovedTemplatesHandedOutShareNothing(t *testing.T) {
	s := loaded(logOf(saved("papers", "First", chief), removed("papers")))
	s.RemovedTemplates()["papers"] = "changed"
	if got := s.RemovedTemplates(); got["papers"] != "" {
		t.Errorf("changing what was handed out changed the state: %v", got)
	}
}
