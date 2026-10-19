package features_test

import (
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/events"
	"gitlab.com/dev.chempa/agentry/internal/features"
	"gitlab.com/dev.chempa/agentry/internal/storetest"
)

var you = events.Actor{Kind: events.Human, ID: "you"}

// recording is a Proposer that keeps what reaches it and answers with
// event.
type recording struct {
	proposed []events.Proposal
	prefixes []string
	event    events.Event
}

func (r *recording) Propose(p events.Proposal) (events.Event, *events.Rejection, error) {
	r.proposed = append(r.proposed, p)
	return r.event, nil, nil
}

func (r *recording) Create(prefix string, p events.Proposal) (events.Event, *events.Rejection, error) {
	r.prefixes = append(r.prefixes, prefix)
	return r.Propose(p)
}

// votes registers vote.cast in its streams and vote.counted in task streams,
// and writes the core's task.created.
func votes() (features.Feature, events.Registration) {
	return features.Feature{Name: "votes", Writes: []events.Type{events.TaskCreated}},
		events.Registration{Prefix: "vote-", Types: map[events.Type]events.Spec{"vote.cast": {}},
			TaskTypes: map[events.Type]events.Spec{"vote.counted": {}}}
}

// A type the feature registered, in its streams or in tasks', or listed in
// Writes reaches the log as it was proposed, and its answer comes back.
func TestADeclaredTypePassesThroughUnchanged(t *testing.T) {
	feature, registered := votes()
	for _, kind := range []events.Type{"vote.cast", "vote.counted", events.TaskCreated} {
		inner := &recording{event: events.Event{Position: 7, Stream: "vote-1", Type: kind}}
		log := features.Checked(feature, registered, inner)
		p := events.Proposal{Stream: "vote-1", Actor: you, Type: kind, Data: map[string]any{"n": 1}}
		got, rejection, err := log.Propose(p)
		if err != nil || rejection != nil || !reflect.DeepEqual(got, inner.event) {
			t.Errorf("Propose %s: got %+v %v %v, want %+v", kind, got, rejection, err, inner.event)
		}
		if _, rejection, err := log.Create("vote-", p); err != nil || rejection != nil {
			t.Errorf("Create %s: %v %v", kind, rejection, err)
		}
		if want := []events.Proposal{p, p}; !reflect.DeepEqual(inner.proposed, want) || !reflect.DeepEqual(inner.prefixes, []string{"vote-"}) {
			t.Errorf("%s reached the log as %+v under %v, want %+v under [vote-]", kind, inner.proposed, inner.prefixes, want)
		}
	}
}

// A type the feature neither registered nor listed in Writes is refused with
// the rule "features", naming the feature and the type, and never reaches
// the log.
func TestAnUndeclaredTypeIsRefusedBeforeTheLog(t *testing.T) {
	feature, registered := votes()
	inner := &recording{}
	log := features.Checked(feature, registered, inner)
	p := events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskDone, Data: map[string]any{"summary": "s"}}
	for name, propose := range map[string]func() (events.Event, *events.Rejection, error){
		"Propose": func() (events.Event, *events.Rejection, error) { return log.Propose(p) },
		"Create":  func() (events.Event, *events.Rejection, error) { return log.Create("task-", p) },
	} {
		got, rejection, err := propose()
		if err != nil || rejection == nil || rejection.Check != "features" || !reflect.DeepEqual(got, events.Event{}) {
			t.Fatalf("%s: got %+v %+v %v, want a features rejection", name, got, rejection, err)
		}
		if !strings.Contains(rejection.Reason, "votes") || !strings.Contains(rejection.Reason, string(events.TaskDone)) {
			t.Errorf("%s: the reason should name the feature and the type: %q", name, rejection.Reason)
		}
	}
	if len(inner.proposed) != 0 {
		t.Errorf("refused proposals reached the log: %+v", inner.proposed)
	}
}

// Writes is read once, when Checked is made: changing the slice after
// doesn't widen what the feature may write.
func TestWritesIsReadWhenCheckedIsMade(t *testing.T) {
	feature, registered := votes()
	log := features.Checked(feature, registered, &recording{})
	feature.Writes[0] = events.TaskDone
	if _, rejection, _ := log.Propose(events.Proposal{Stream: "task-1", Actor: you, Type: events.TaskDone}); rejection == nil {
		t.Error("task.done went through after Writes was changed")
	}
}

// Against a real log: a feature proposing a core type outside its Writes is
// refused with the rule "features", and the log holds nothing new, not even
// a rejection; the same proposal from a feature that lists the type is
// stored.
func TestARefusedProposalLeavesTheLogAsItWas(t *testing.T) {
	s, _ := storetest.Open(t, filepath.Join(t.TempDir(), "agentry.db"))
	if _, rejection, err := s.Propose(events.Proposal{Stream: events.AgentsStream, Actor: you, Type: events.AgentCreated,
		Data: map[string]any{"name": "tech-lead", "prompt": "Lead.", "engine": "claude", "coordinates": true}}); err != nil || rejection != nil {
		t.Fatal(rejection, err)
	}
	task := events.Proposal{Actor: you, Type: events.TaskCreated,
		Data: map[string]any{"title": "Remove the TODOs", "repo": "/src/app", "coordinator": "tech-lead"}}
	before := s.LastPosition()

	silent := features.Checked(features.Feature{Name: "silent"}, events.Registration{}, s)
	_, rejection, err := silent.Create("task-", task)
	if err != nil || rejection == nil || rejection.Check != "features" {
		t.Fatalf("got %+v %v, want a features rejection", rejection, err)
	}
	if after := s.LastPosition(); after != before {
		t.Errorf("the log moved from %d to %d on a refused proposal", before, after)
	}
	if stream, err := s.Log("task-1"); err != nil || len(stream) != 0 {
		t.Errorf("task-1 holds %+v (%v), want nothing", stream, err)
	}

	feature, registered := votes()
	created, rejection, err := features.Checked(feature, registered, s).Create("task-", task)
	if err != nil || rejection != nil || created.Stream != "task-1" || s.LastPosition() != before+1 {
		t.Errorf("a declared task.created: got %+v %+v %v at %d", created, rejection, err, s.LastPosition())
	}
}
