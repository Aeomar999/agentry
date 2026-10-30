package workerservice

import (
	"errors"
	"testing"
)

type fake struct {
	kind    string
	fails   bool
	running *bool
}

func (f fake) Kind() string { return f.kind }
func (f fake) Start(Spec) error {
	if f.fails {
		return errors.New("can't here")
	}
	*f.running = true
	return nil
}
func (f fake) Stop() error   { *f.running = false; return nil }
func (f fake) Running() bool { return *f.running }
func (f fake) Remove() error { return f.Stop() }

// Where launchd won't start the worker (over SSH), it runs in the
// background; where it will, the background one stops.
func TestFallbackStartsTheSecondWhenTheFirstCant(t *testing.T) {
	var firstUp, secondUp bool
	s := fallback{first: fake{kind: "launchd", fails: true, running: &firstUp}, second: fake{kind: "background", running: &secondUp}}
	if err := s.Start(Spec{}); err != nil || !secondUp || s.Kind() != "background" || !s.Running() {
		t.Fatalf("got %v, running %v, kind %s", err, secondUp, s.Kind())
	}
	s.first = fake{kind: "launchd", running: &firstUp}
	if err := s.Start(Spec{}); err != nil || !firstUp || secondUp || s.Kind() != "launchd" {
		t.Errorf("once launchd works: %v, launchd %v, background %v", err, firstUp, secondUp)
	}
}
