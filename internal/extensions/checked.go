package extensions

import (
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// Checked is log for the extension f, whose types are registered: it refuses,
// with the rule "features", any proposal whose type f neither registered
// (in registered's Types or TaskTypes) nor listed in its Writes. The refusal
// comes before log is called, so nothing of it is stored, and it comes back
// as any rejection does: an extension's mistake fails its own tests.
func Checked(f Extension, registered events.Registration, log Proposer) Proposer {
	may := map[events.Type]bool{}
	for t := range registered.Types {
		may[t] = true
	}
	for t := range registered.TaskTypes {
		may[t] = true
	}
	for _, t := range f.Writes {
		may[t] = true
	}
	return checked{extension: f.Name, may: may, log: log}
}

// checked is the Proposer Checked makes.
type checked struct {
	extension string
	may       map[events.Type]bool // the types the extension may propose
	log       Proposer
}

func (c checked) Propose(p events.Proposal) (events.Event, *events.Rejection, error) {
	if refused := c.refuse(p.Type); refused != nil {
		return events.Event{}, refused, nil
	}
	return c.log.Propose(p)
}

func (c checked) Create(prefix string, p events.Proposal) (events.Event, *events.Rejection, error) {
	if refused := c.refuse(p.Type); refused != nil {
		return events.Event{}, refused, nil
	}
	return c.log.Create(prefix, p)
}

// refuse is the "features" rejection of a type the extension may not propose,
// or nil.
func (c checked) refuse(t events.Type) *events.Rejection {
	if c.may[t] {
		return nil
	}
	return &events.Rejection{Check: "features",
		Reason: fmt.Sprintf("the %s feature may not write %s: it neither registers it nor lists it in Writes", c.extension, t)}
}

// Unwritable is the log of an extension that was given none: Checked with no
// types to write, onto a log that refuses everything itself, so nothing it
// proposes can reach a log that isn't there.
func Unwritable(name string) Proposer {
	return Checked(Extension{Name: name}, events.Registration{}, nowhere{name: name})
}

// nowhere refuses every proposal, for Unwritable.
type nowhere struct{ name string }

func (n nowhere) Propose(events.Proposal) (events.Event, *events.Rejection, error) {
	return events.Event{}, &events.Rejection{Check: "features", Reason: "the " + n.name + " feature has no log to write to"}, nil
}

func (n nowhere) Create(_ string, p events.Proposal) (events.Event, *events.Rejection, error) {
	return n.Propose(p)
}
