package current

import (
	"maps"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// Unknown counts events the state skipped or didn't understand: those of
// one foreign stream prefix, or those of one type the catalogue doesn't know
// in the streams it does. It is never an error and never breaks a stream.
type Unknown struct {
	Prefix  string      `json:"prefix,omitempty"`  // the foreign streams' prefix, "idea-"; or
	Type    events.Type `json:"type,omitempty"`    // the unknown type, "git.pr_opened"
	Streams int         `json:"streams,omitempty"` // how many foreign streams have the prefix
	Events  int         `json:"events"`
	Last    int64       `json:"last"` // the position of the last of them
}

// tally is the counts as they are kept, so a stream loaded whole and one
// applied event by event come to the same counts.
type tally struct {
	prefixes map[string]Unknown
	types    map[events.Type]Unknown
}

func newTally() tally {
	return tally{prefixes: map[string]Unknown{}, types: map[events.Type]Unknown{}}
}

// count counts one accepted event of a stream of kind, if it is skipped or
// not understood. first says it is the stream's first.
func (t tally) count(catalogue *events.Catalogue, kind events.StreamKind, e events.Event, first bool) {
	switch {
	case kind == events.ForeignStream:
		prefix := events.ForeignPrefix(e.Stream)
		u := t.prefixes[prefix]
		u.Prefix = prefix
		if first {
			u.Streams++
		}
		t.prefixes[prefix] = u.add(e)
	case !catalogue.Knows(e.Type):
		u := t.types[e.Type]
		u.Type = e.Type
		t.types[e.Type] = u.add(e)
	}
}

// add counts e. A stream loaded whole may come after a later one, so Last is
// the largest position seen, not the latest.
func (u Unknown) add(e events.Event) Unknown {
	u.Events++
	u.Last = max(u.Last, e.Position)
	return u
}

func (t tally) clone() tally {
	return tally{prefixes: maps.Clone(t.prefixes), types: maps.Clone(t.types)}
}

// list is the prefixes, then the types, each in name order: empty, not
// nil, when nothing was skipped.
func (t tally) list() []Unknown {
	prefixes := slices.SortedFunc(maps.Values(t.prefixes), func(a, b Unknown) int { return strings.Compare(a.Prefix, b.Prefix) })
	types := slices.SortedFunc(maps.Values(t.types), func(a, b Unknown) int { return strings.Compare(string(a.Type), string(b.Type)) })
	return append(append(make([]Unknown, 0, len(prefixes)+len(types)), prefixes...), types...)
}
