package rules

import (
	"time"

	"gitlab.com/dev.chempa/agentry/internal/connections"
	"gitlab.com/dev.chempa/agentry/internal/events"
)

// connectionRules check Claude Code's connections against those made so far.
var connectionRules = map[events.Type]func(connections.Roster, events.Proposal) string{
	events.ConnectionMade: func(r connections.Roster, p events.Proposal) string {
		if !r.CanConnect(p.Text("code"), time.Now()) {
			return "that install command is unknown, used or expired; make a new one on the Integrations page"
		}
		return ""
	},
	events.ConnectionRevoked: func(r connections.Roster, p events.Proposal) string {
		if !r.Has(p.Text("id")) {
			return "no connection " + p.Text("id")
		}
		return ""
	},
}
