package events_test

import (
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/ideas"
)

func TestCheckShapeAndWriter(t *testing.T) {
	human := events.Actor{events.Human, "you"}
	coordinator := events.Actor{events.Agent, "coordinator"}
	cases := []struct {
		name      string
		proposal  events.Proposal
		wantCheck string // "" means accepted
	}{
		{"a valid task", events.Proposal{Stream: "task-1", Actor: human, Type: events.TaskCreated,
			Data: map[string]any{"title": "Remove TODOs", "repo": "/src/app", "coordinator": "tech-lead"}}, ""},
		{"unknown type", events.Proposal{Stream: "task-1", Actor: human, Type: "task.exploded"}, "shape"},
		{"missing field", events.Proposal{Stream: "task-1", Actor: human, Type: events.TaskCreated,
			Data: map[string]any{"title": "Remove TODOs"}}, "shape"},
		{"missing actor", events.Proposal{Stream: "task-1", Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": "y"}}, "shape"},
		{"missing correlation", events.Proposal{Stream: "task-1", Actor: events.Actor{events.Worker, "local"}, Type: events.TurnStarted,
			Data: map[string]any{"agent": "engineer", "hash": "ab"}}, "shape"},
		{"agent event outside the agents stream", events.Proposal{Stream: "task-1", Actor: human, Type: events.AgentCreated,
			Data: map[string]any{"name": "x", "prompt": "y", "engine": "claude"}}, "shape"},
		{"task event in the agents stream", events.Proposal{Stream: events.AgentsStream, Actor: human, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": "y"}}, "shape"},
		{"the policy may not create tasks", events.Proposal{Stream: "task-1", Actor: events.Actor{Kind: events.Policy, ID: "rules"}, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": "y", "coordinator": "tech-lead"}}, "who"},
		{"a human may not assign: the coordinator does", events.Proposal{Stream: "task-1", Actor: human, Type: events.TaskAssigned,
			Data: map[string]any{"agent": "engineer", "instructions": "go"}}, "who"},
		{"nobody proposes a rejection", events.Proposal{Stream: "task-1", Actor: events.Actor{events.System, "x"}, Type: events.ProposalRejected}, "who"},
		{"a policy load", events.Proposal{Stream: events.PolicyStream, Actor: events.Actor{events.System, "policy"}, Type: events.PolicyLoaded,
			Data: map[string]any{"hash": "ab12", "path": "/home/dev/.agentry/policy.toml"}}, ""},
		{"a policy change from the page", events.Proposal{Stream: events.PolicyStream, Actor: human, Type: events.PolicyLoaded,
			Data: map[string]any{"hash": "ab12", "path": "/p", "text": "version = 1", "change": "added profile read-mostly"}}, ""},
		{"an agent may not change the policy", events.Proposal{Stream: events.PolicyStream, Actor: coordinator, Type: events.PolicyLoaded,
			Data: map[string]any{"hash": "ab12", "path": "/p", "text": "version = 1"}}, "who"},
		{"a policy event in a task", events.Proposal{Stream: "task-1", Actor: events.Actor{events.System, "policy"}, Type: events.PolicyLoaded,
			Data: map[string]any{"hash": "ab12", "path": "/p"}}, "shape"},
		{"a task event in the policy stream", events.Proposal{Stream: events.PolicyStream, Actor: human, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": "y", "coordinator": "c"}}, "shape"},
		{"a join code", events.Proposal{Stream: events.WorkersStream, Actor: human, Type: events.WorkerInvited,
			Data: map[string]any{"code": "ab12", "expires": "2026-10-17T12:00:00Z"}}, ""},
		{"an agent may not make join codes", events.Proposal{Stream: events.WorkersStream, Actor: coordinator, Type: events.WorkerInvited,
			Data: map[string]any{"code": "ab12", "expires": "2026-10-17T12:00:00Z"}}, "who"},
		{"a worker event in a task", events.Proposal{Stream: "task-1", Actor: events.Actor{events.System, "workers"}, Type: events.WorkerConnected,
			Data: map[string]any{"name": "laptop"}}, "shape"},
		{"an idea", events.Proposal{Stream: "idea-1", Actor: events.Actor{events.System, "agentry"}, Type: ideas.IdeaSuggested,
			Data: map[string]any{"title": "x", "problem": "p", "proposal": "f", "size": "S", "repo": "/src/app", "task": "task-1"}}, ""},
		{"an idea event in a task", events.Proposal{Stream: "task-1", Actor: human, Type: ideas.IdeaDismissed,
			Data: map[string]any{"note": "no"}}, "shape"},
		{"a task event in an idea", events.Proposal{Stream: "idea-1", Actor: human, Type: events.TaskCreated,
			Data: map[string]any{"title": "x", "repo": "y", "coordinator": "c"}}, "shape"},
		{"only you accept an idea", events.Proposal{Stream: "idea-1", Actor: coordinator, Type: ideas.IdeaAccepted,
			Data: map[string]any{"task": "task-2"}}, "who"},
		{"the policy asks", events.Proposal{Stream: "task-1", Actor: events.Actor{events.Policy, "rules"}, Type: events.ActionAsked, Correlation: "toolu_1",
			Data: map[string]any{"reason": "may reach outside the worktree"}}, ""},
	}
	catalogue, err := events.NewCatalogue(ideas.Registration())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cases {
		rejection := catalogue.CheckShape(c.proposal)
		if rejection == nil {
			rejection = catalogue.CheckWriter(c.proposal)
		}
		got := ""
		if rejection != nil {
			got = rejection.Check
		}
		if got != c.wantCheck {
			t.Errorf("%s: got check %q (%v), want %q", c.name, got, rejection, c.wantCheck)
		}
	}
}

func TestNewCatalogueRefusesWhatWouldCollide(t *testing.T) {
	spec := events.Spec{Writers: []events.ActorKind{events.System}}
	notes := events.Registration{Prefix: "note-", Types: map[events.Type]events.Spec{"note.written": spec}}
	cases := []struct {
		name         string
		registration []events.Registration
		want         string // in the error
	}{
		{"a prefix ideas took", []events.Registration{ideas.Registration(), {Prefix: ideas.Prefix, Types: map[events.Type]events.Spec{"note.written": spec}}}, `prefix "idea-" is taken`},
		{"a prefix overlapping another", []events.Registration{notes, {Prefix: "note-pad-", Types: map[events.Type]events.Spec{"pad.written": spec}}}, `prefix "note-pad-" is taken`},
		{"a prefix a core stream starts with", []events.Registration{{Prefix: "agent", Types: map[events.Type]events.Spec{"note.written": spec}}}, `prefix "agent" is taken`},
		{"a type in its streams and in tasks'", []events.Registration{{Prefix: "note-", Types: map[events.Type]events.Spec{"note.written": spec}, TaskTypes: map[events.Type]events.Spec{"note.written": spec}}}, "note.written is taken"},
		{"a task- prefix", []events.Registration{{Prefix: "task-notes-", Types: map[events.Type]events.Spec{"note.written": spec}}}, `prefix "task-notes-" overlaps task-`},
		{"a prefix task- starts with", []events.Registration{{Prefix: "task", Types: map[events.Type]events.Spec{"note.written": spec}}}, `prefix "task" overlaps task-`},
		{"a one-letter prefix task- starts with", []events.Registration{{Prefix: "t", Types: map[events.Type]events.Spec{"note.written": spec}}}, `prefix "t" overlaps task-`},
		{"stream types with no prefix", []events.Registration{{Types: map[events.Type]events.Spec{"note.written": spec}}}, "need a stream prefix"},
	}
	for _, c := range cases {
		_, err := events.NewCatalogue(c.registration...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error with %q", c.name, err, c.want)
		}
	}
	if _, err := events.NewCatalogue(ideas.Registration(), events.Registration{TaskTypes: map[events.Type]events.Spec{"git.pr_opened": spec}}); err != nil {
		t.Errorf("a task type in a namespace of its own: %v", err)
	}
}

// A registration's types are all in one namespace of its own: its prefix's,
// or the one its task types share. Neither the core nor another
// registration may use it.
func TestNewCatalogueRefusesATypeOutsideItsNamespace(t *testing.T) {
	spec := events.Spec{Writers: []events.ActorKind{events.System}}
	notes := events.Registration{Prefix: "note-", Types: map[events.Type]events.Spec{"note.written": spec}}
	flags := events.Registration{TaskTypes: map[events.Type]events.Spec{"note.flagged": spec}}
	cases := []struct {
		name         string
		registration []events.Registration
		want         string // in the error
	}{
		{"a stream type outside its prefix's namespace", []events.Registration{{Prefix: "note-", Types: map[events.Type]events.Spec{"task.noted": spec}}}, `stream type task.noted is outside the note. namespace of stream prefix "note-"`},
		{"a stream type in a core task namespace", []events.Registration{{Prefix: "turn-", Types: map[events.Type]events.Spec{"turn.noted": spec}}}, "stream type turn.noted is in the turn. namespace, which the core uses"},
		{"a stream type in a core fixed-stream namespace", []events.Registration{{Prefix: "policy-", Types: map[events.Type]events.Spec{"policy.noted": spec}}}, "stream type policy.noted is in the policy. namespace, which the core uses"},
		{"a prefix in a core namespace", []events.Registration{{Prefix: "worker-"}}, `stream prefix "worker-" is in the worker. namespace, which the core uses`},
		{"a task type in task.", []events.Registration{{TaskTypes: map[events.Type]events.Spec{"task.pr_opened": spec}}}, "task type task.pr_opened is in the task. namespace, which the core uses"},
		{"a task type in turn.", []events.Registration{{TaskTypes: map[events.Type]events.Spec{"turn.paused": spec}}}, "task type turn.paused is in the turn. namespace, which the core uses"},
		{"a task type outside its prefix's namespace", []events.Registration{{Prefix: "note-", Types: map[events.Type]events.Spec{"note.written": spec}, TaskTypes: map[events.Type]events.Spec{"git.pr_opened": spec}}}, `task type git.pr_opened is outside the note. namespace of stream prefix "note-"`},
		{"task types in two namespaces", []events.Registration{{TaskTypes: map[events.Type]events.Spec{"git.pr_opened": spec, "pr.merged": spec}}}, "task type pr.merged is outside the git. namespace of task type git.pr_opened"},
		{"a prefix in another's namespace", []events.Registration{flags, notes}, "stream type note.written is in the note. namespace, which task type note.flagged has"},
		{"task types in a prefix's namespace", []events.Registration{notes, flags}, `task type note.flagged is in the note. namespace, which stream prefix "note-" has`},
		{"a prefix with no namespace", []events.Registration{{Prefix: "-", Types: map[events.Type]events.Spec{"turn.noted": spec}}}, `stream prefix "-" has no namespace`},
		{"a task type with no namespace", []events.Registration{{TaskTypes: map[events.Type]events.Spec{".x": spec}}}, "task type .x has no namespace"},
	}
	for _, c := range cases {
		_, err := events.NewCatalogue(c.registration...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: got %v, want an error with %q", c.name, err, c.want)
		}
	}
	accepted := [][]events.Registration{
		{ideas.Registration()},
		{ideas.Registration(), {Prefix: "note-", Types: map[events.Type]events.Spec{"note.written": spec}, TaskTypes: map[events.Type]events.Spec{"note.flagged": spec}}},
		{ideas.Registration(), {TaskTypes: map[events.Type]events.Spec{"git.pr_opened": spec, "git.pr_merged": spec}}},
	}
	for _, registrations := range accepted {
		if _, err := events.NewCatalogue(registrations...); err != nil {
			t.Errorf("each in a namespace of its own: %v", err)
		}
	}
}

// A feature's task types go in task streams; its stream types only in its
// own streams, which aren't tasks.
func TestARegisteredPrefixIsNotATask(t *testing.T) {
	spec := events.Spec{Writers: []events.ActorKind{events.System}}
	catalogue, err := events.NewCatalogue(ideas.Registration(), events.Registration{TaskTypes: map[events.Type]events.Spec{"git.pr_opened": spec}})
	if err != nil {
		t.Fatal(err)
	}
	for stream, want := range map[string]bool{"task-1": true, "idea-1": false, events.AgentsStream: false, events.SettingsStream: false} {
		if got := catalogue.IsTaskStream(stream); got != want {
			t.Errorf("IsTaskStream(%q) = %v, want %v", stream, got, want)
		}
	}
	agentry := events.Actor{Kind: events.System, ID: "agentry"}
	if rejection := catalogue.CheckShape(events.Proposal{Stream: "task-1", Actor: agentry, Type: "git.pr_opened"}); rejection != nil {
		t.Errorf("a task type in a task: %v", rejection)
	}
	if rejection := catalogue.CheckShape(events.Proposal{Stream: "idea-1", Actor: agentry, Type: "git.pr_opened"}); rejection == nil {
		t.Error("a task type in an idea was let through")
	}
	if rejection := events.Core().CheckShape(events.Proposal{Stream: "task-1", Actor: agentry, Type: "git.pr_opened"}); rejection == nil {
		t.Error("the core knows a type nobody registered")
	}
}

// Each stream is core, owned, a task's or foreign, by its name alone: a
// stream whose feature isn't registered is foreign, not a task.
func TestEveryStreamHasAKind(t *testing.T) {
	withIdeas, err := events.NewCatalogue(ideas.Registration())
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		stream          string
		withIdeas, core events.StreamKind
	}{
		{events.AgentsStream, events.CoreStream, events.CoreStream},
		{events.SettingsStream, events.CoreStream, events.CoreStream},
		{events.PolicyStream, events.CoreStream, events.CoreStream},
		{"task-1", events.TaskStream, events.TaskStream},
		{"task-12", events.TaskStream, events.TaskStream},
		{"idea-3", events.OwnedStream, events.ForeignStream},
		{"note-1", events.ForeignStream, events.ForeignStream},
		{"agents-2", events.ForeignStream, events.ForeignStream},
		{"tasks", events.ForeignStream, events.ForeignStream},
	}
	for _, c := range cases {
		if got := withIdeas.KindOf(c.stream); got != c.withIdeas {
			t.Errorf("with ideas, KindOf(%q) = %v, want %v", c.stream, got, c.withIdeas)
		}
		if got := events.Core().KindOf(c.stream); got != c.core {
			t.Errorf("the core's KindOf(%q) = %v, want %v", c.stream, got, c.core)
		}
	}
	for stream, want := range map[string]string{"idea-3": "idea-", "git-pr-12": "git-pr-", "notes": "notes", "a-": "a-"} {
		if got := events.ForeignPrefix(stream); got != want {
			t.Errorf("ForeignPrefix(%q) = %q, want %q", stream, got, want)
		}
	}
}

// Nothing goes into a foreign stream: not its feature's types, which this
// build doesn't know, nor a task's.
func TestAForeignStreamTakesNothing(t *testing.T) {
	you := events.Actor{Kind: events.Human, ID: "you"}
	for _, p := range []events.Proposal{
		{Stream: "idea-3", Actor: you, Type: ideas.IdeaDismissed, Data: map[string]any{"note": "no"}},
		{Stream: "idea-3", Actor: you, Type: events.TaskCreated, Data: map[string]any{"title": "x", "repo": "/r", "coordinator": "c"}},
	} {
		rejection := events.Core().CheckShape(p)
		if rejection == nil || rejection.Reason != "stream idea-3 belongs to a feature this build doesn't have" {
			t.Errorf("%s into idea-3: got %v, want the foreign stream's reason", p.Type, rejection)
		}
	}
}
