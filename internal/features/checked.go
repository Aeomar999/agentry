package features

import (
	"fmt"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Checked is log for the feature f, whose types are registered: it refuses,
// with the rule "features", any proposal whose type f neither registered
// (in registered's Types or TaskTypes) nor listed in its Writes. The refusal
// comes before log is called, so nothing of it is stored, and it comes back
// as any rejection does: a feature's mistake fails its own tests.
func Checked(f Feature, registered events.Registration, log Proposer) Proposer {
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
	return checked{feature: f.Name, may: may, log: log}
}

// checked is the Proposer Checked makes.
type checked struct {
	feature string
	may     map[events.Type]bool // the types the feature may propose
	log     Proposer
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

// refuse is the "features" rejection of a type the feature may not propose,
// or nil.
func (c checked) refuse(t events.Type) *events.Rejection {
	if c.may[t] {
		return nil
	}
	return &events.Rejection{Check: "features",
		Reason: fmt.Sprintf("the %s feature may not write %s: it neither registers it nor lists it in Writes", c.feature, t)}
}
