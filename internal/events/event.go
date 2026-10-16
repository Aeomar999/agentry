package events

import (
	"crypto/sha256"
	"encoding/hex"
	"time"
)

// Proposal is what anyone sends. It becomes an Event only if the validator accepts it.
type Proposal struct {
	Stream      string         `json:"stream"` // a task ID, or AgentsStream
	Actor       Actor          `json:"actor"`
	Type        Type           `json:"type"`
	Correlation string         `json:"correlation,omitempty"` // ties a request to its decision and result
	Data        map[string]any `json:"data,omitempty"`
}

// Event is a stored proposal. It is never changed after.
type Event struct {
	Position int64     `json:"position"` // its place in the whole log, starting at 1
	At       time.Time `json:"at"`
	Proposal
}

// Text returns a data field as a string, or "" if it is missing or not a string.
func (p Proposal) Text(field string) string {
	value, _ := p.Data[field].(string)
	return value
}

// Number returns a data field as an int, or 0. JSON numbers decode as float64.
func (p Proposal) Number(field string) int {
	switch value := p.Data[field].(type) {
	case int:
		return value
	case float64:
		return int(value)
	}
	return 0
}

// Flag returns a data field as a bool, or false.
func (p Proposal) Flag(field string) bool {
	value, _ := p.Data[field].(bool)
	return value
}

// Rejection says which check refused a proposal, and why.
type Rejection struct {
	Check  string `json:"check"` // shape | who | order
	Reason string `json:"reason"`
}

func (r *Rejection) Error() string { return r.Check + ": " + r.Reason }

// Hash is the short hash recorded for a prompt, so its delivery can be proven
// by the hash the engine echoes back (proof 02).
func Hash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}
