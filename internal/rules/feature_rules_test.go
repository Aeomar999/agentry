package rules

import (
	"bytes"
	"log/slog"
	"maps"
	"path/filepath"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
	"gitlab.com/dev.chempa/agentry/internal/current"
	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/store"
)

// noted is the one type in a fake feature's streams. It is in the fragile.
// namespace, so only a feature with the fragile- prefix registers it.
const noted events.Type = "fragile.noted"

// fake is a feature that owns the streams prefix starts, holding noted
// events, with check as its Streams.Check and guards as its Guards. Its
// state counts a stream's events.
func fake(name, prefix string, check features.Rule, guards map[events.Type]features.Rule) features.Feature {
	return features.Feature{Name: name, Guards: guards, Streams: &features.Streams{
		Prefix: prefix,
		Types:  map[events.Type]events.Spec{noted: {Fields: []string{"note"}, Writers: []events.ActorKind{events.Human}}},
		FromEvents: func(accepted []events.Event) any {
			if len(accepted) == 0 {
				return nil
			}
			return len(accepted)
		},
		Next: func(prev any, e events.Event) any {
			count, _ := prev.(int)
			return count + 1
		},
		Check: check,
	}}
}

// openInstalled is a store with the five default agents, whose rules run
// the installed features', as serve wires them.
func openInstalled(t *testing.T, installed ...features.Feature) *store.Store {
	t.Helper()
	catalogue, err := events.NewCatalogue(features.Registrations(installed...)...)
	if err != nil {
		t.Fatal(err)
	}
	return openChecked(t, catalogue, installed...)
}

// openChecked is openInstalled with a catalogue of its own, which may lack
// the installed features' streams or have a feature's that isn't installed.
func openChecked(t *testing.T, catalogue *events.Catalogue, installed ...features.Feature) *store.Store {
	t.Helper()
	var owned []*features.Streams
	for _, f := range installed {
		owned = append(owned, f.Streams)
	}
	state := current.New(catalogue, owned...)
	s, err := store.Open(filepath.Join(t.TempDir(), "agentry.db"), catalogue, state, func(p events.Proposal) *events.Rejection { return Check(p, state, catalogue, installed...) })
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	t.Cleanup(func() {
		if err := current.Equal(s, state); err != nil {
			t.Errorf("the state differs from a replay of the log: %v", err)
		}
	})
	for _, p := range agents.SeedProposals(nil) {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatalf("seeding %s: %v %v", p.Text("name"), rejection, err)
		}
	}
	return s
}

// logged is what slog says during the test.
func logged(t *testing.T) *bytes.Buffer {
	var said bytes.Buffer
	before := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&said, nil)))
	t.Cleanup(func() { slog.SetDefault(before) })
	return &said
}

// newTask is a task.created you propose in stream, with more data.
func newTask(stream string, by events.Actor, more map[string]any) events.Proposal {
	data := map[string]any{"title": "x", "repo": "/r", "coordinator": "tech-lead", "budget": 0, "maxTurns": 0}
	maps.Copy(data, more)
	return events.Proposal{Stream: stream, Actor: by, Type: events.TaskCreated, Data: data}
}

// A feature's rules can only refuse: one that lets everything through
// doesn't get past a refusal of the core's.
func TestAFeaturesRulesOnlyRefuse(t *testing.T) {
	allow := func(events.Proposal, features.Reader) *events.Rejection { return nil }
	lenient := fake("lenient", "lenient-", allow, map[events.Type]features.Rule{
		events.TaskCreated: allow, events.TaskRenamed: allow, events.TaskHeld: allow, noted: allow})
	s := openChecked(t, events.Core(), lenient)
	refused := []struct {
		name string
		p    events.Proposal
		want string
	}{
		{"its stream, where the catalogue doesn't have it", events.Proposal{Stream: "lenient-1", Actor: you, Type: noted, Data: map[string]any{"note": "x"}},
			events.ForeignReason("lenient-1")},
		{"a task that doesn't exist", events.Proposal{Stream: "task-9", Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": "y"}},
			"task task-9 doesn't exist"},
		{"Agentry's task from no stream", newTask("task-1", agentry, nil),
			"Agentry makes a task only about an epic you closed, or from an idea the Visionaries agreed on"},
	}
	for _, c := range refused {
		if _, rejection, err := s.Propose(c.p); err != nil || rejection == nil || rejection.Reason != c.want {
			t.Errorf("%s: %v %v, want %q", c.name, rejection, err, c.want)
		}
	}
}

// The core lets Agentry make a task from a feature's stream only when that
// feature guards task.created: without ideas installed, Agentry's task from
// an idea is refused, whether the catalogue has the ideas' streams or not.
func TestWithoutIdeasAgentryMakesNoTaskFromAnIdea(t *testing.T) {
	unguarded := fake("unguarded", "idea-", nil, nil)
	fromAnIdea := newTask("task-1", agentry, map[string]any{"from": "idea-1", "idea": "idea-1", "parked": "backlog"})
	for name, catalogue := range map[string]*events.Catalogue{"the core's catalogue": events.Core(), "a catalogue with ideas": withIdeas} {
		for _, installed := range [][]features.Feature{nil, {unguarded}} {
			s := openChecked(t, catalogue, installed...)
			if _, rejection, err := s.Propose(fromAnIdea); err != nil || rejection == nil ||
				rejection.Reason != "Agentry makes a task only about an epic you closed, or from an idea the Visionaries agreed on" {
				t.Errorf("%s, %d features: %v %v", name, len(installed), rejection, err)
			}
		}
	}
}

// A feature's Streams.Check that panics refuses that proposal, says so once
// in the log, and the store goes on: the same stream and others take the
// next proposals.
func TestAPanickingStreamsCheckRefusesOnlyItsProposal(t *testing.T) {
	said := logged(t)
	fragile := fake("fragile", "fragile-", func(p events.Proposal, _ features.Reader) *events.Rejection {
		if p.Text("note") == "boom" {
			panic("the check broke")
		}
		return nil
	}, nil)
	s := openInstalled(t, fragile)
	note := func(stream, text string) events.Proposal {
		return events.Proposal{Stream: stream, Actor: you, Type: noted, Data: map[string]any{"note": text}}
	}

	_, rejection, err := s.Propose(note("fragile-1", "boom"))
	if err != nil || rejection == nil || rejection.Reason != "the fragile feature's rules failed on this fragile.noted, so it is refused" {
		t.Errorf("the panicking check: %v %v", rejection, err)
	}
	if lines := strings.Count(said.String(), "a feature's rule panicked"); lines != 1 || !strings.Contains(said.String(), "feature=fragile") ||
		!strings.Contains(said.String(), `panic="the check broke"`) {
		t.Errorf("want one line naming the feature and the panic, got %d:\n%s", lines, said)
	}
	for _, p := range []events.Proposal{note("fragile-2", "fine"), note("fragile-1", "fine"), newTask("task-1", you, nil)} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Errorf("%s into %s after the panic: %v %v", p.Type, p.Stream, rejection, err)
		}
	}
}

// A Guard that panics refuses that proposal the same way.
func TestAPanickingGuardRefusesOnlyItsProposal(t *testing.T) {
	said := logged(t)
	fragile := fake("fragile", "fragile-", nil, map[events.Type]features.Rule{
		events.TaskRenamed: func(p events.Proposal, _ features.Reader) *events.Rejection {
			if p.Text("title") == "boom" {
				panic("the guard broke")
			}
			return nil
		}})
	s := openInstalled(t, fragile)
	renamed := func(stream, title string) events.Proposal {
		return events.Proposal{Stream: stream, Actor: you, Type: events.TaskRenamed, Data: map[string]any{"title": title}}
	}
	for _, p := range []events.Proposal{newTask("task-1", you, nil), newTask("task-2", you, nil)} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Fatal(rejection, err)
		}
	}

	_, rejection, err := s.Propose(renamed("task-1", "boom"))
	if err != nil || rejection == nil || rejection.Reason != "the fragile feature's rules failed on this task.renamed, so it is refused" {
		t.Errorf("the panicking guard: %v %v", rejection, err)
	}
	if lines := strings.Count(said.String(), "a feature's rule panicked"); lines != 1 || !strings.Contains(said.String(), `panic="the guard broke"`) {
		t.Errorf("want one line naming the panic, got %d:\n%s", lines, said)
	}
	for _, p := range []events.Proposal{renamed("task-2", "fine"), renamed("task-1", "fine"),
		{Stream: "fragile-1", Actor: you, Type: noted, Data: map[string]any{"note": "x"}}} {
		if _, rejection, err := s.Propose(p); rejection != nil || err != nil {
			t.Errorf("%s into %s after the panic: %v %v", p.Type, p.Stream, rejection, err)
		}
	}
}
