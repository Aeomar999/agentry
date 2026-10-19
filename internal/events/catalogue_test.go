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
		{"a core type", []events.Registration{{Prefix: "note-", Types: map[events.Type]events.Spec{events.TaskCreated: spec}}}, "task.created is taken"},
		{"another registration's type", []events.Registration{notes, {TaskTypes: map[events.Type]events.Spec{"note.written": spec}}}, "note.written is taken"},
		{"a task- prefix", []events.Registration{{Prefix: "task-notes-", Types: map[events.Type]events.Spec{"note.written": spec}}}, "starts with task-"},
		{"stream types with no prefix", []events.Registration{{Types: map[events.Type]events.Spec{"note.written": spec}}}, "need a stream prefix"},
		{"a task type in task.", []events.Registration{{TaskTypes: map[events.Type]events.Spec{"task.pr_opened": spec}}}, "in the task. namespace"},
		{"a task type in turn.", []events.Registration{{TaskTypes: map[events.Type]events.Spec{"turn.paused": spec}}}, "in the turn. namespace"},
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
		if got := events.Core().IsTaskStream(stream); got != (want || stream == "idea-1") {
			t.Errorf("the core's IsTaskStream(%q) = %v", stream, got)
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
