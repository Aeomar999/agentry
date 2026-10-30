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

// Has says whether the data has the field at all, even empty.
func (p Proposal) Has(field string) bool {
	_, present := p.Data[field]
	return present
}

// Number returns a data field as an int, or 0. JSON numbers decode as
// float64; a proposal not yet written may carry an int64, as a position.
func (p Proposal) Number(field string) int {
	switch value := p.Data[field].(type) {
	case int:
		return value
	case int64:
		return int(value)
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

// Real returns a data field as a float64, or 0.
func (p Proposal) Real(field string) float64 {
	switch value := p.Data[field].(type) {
	case float64:
		return value
	case int:
		return float64(value)
	}
	return 0
}

// List returns a data field as a list of strings, or nil. A list read back
// from the log decodes as []any.
func (p Proposal) List(field string) []string {
	switch value := p.Data[field].(type) {
	case []string:
		return value
	case []any:
		var list []string
		for _, item := range value {
			if text, ok := item.(string); ok {
				list = append(list, text)
			}
		}
		return list
	}
	return nil
}

// Numbers returns a data field as a list of whole numbers, or nil when it
// isn't one. A list read back from the log decodes as []any of float64; a
// proposal not yet written may carry an []int64, as positions.
func (p Proposal) Numbers(field string) []int64 {
	switch value := p.Data[field].(type) {
	case []int64:
		return value
	case []any:
		list := make([]int64, 0, len(value))
		for _, item := range value {
			number, ok := item.(float64)
			if !ok || number != float64(int64(number)) {
				return nil
			}
			list = append(list, int64(number))
		}
		return list
	}
	return nil
}

// Rejection says which check refused a proposal, and why.
type Rejection struct {
	Check  string `json:"check"` // shape | who | order, or features: an extension proposing a type it may not (extensions.Checked)
	Reason string `json:"reason"`
}

func (r *Rejection) Error() string { return r.Check + ": " + r.Reason }

// Hash is the short hash recorded for a prompt, so its delivery can be proven
// by the hash the engine echoes back (proof 02).
func Hash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}
