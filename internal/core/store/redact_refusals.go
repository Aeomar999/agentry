package store

import (
	"fmt"
	"slices"
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/core/events"
)

// asItIs refuses to redact a stream as it is: one erased, of which nothing
// is left, or one archived, whose rows are in a blob.
func asItIs(stream string, logged []events.Event) *events.Rejection {
	if erased(logged) {
		return erasedRedaction(stream)
	}
	if _, archived := MarkerOf(logged); archived {
		return &events.Rejection{Check: "redact", Reason: ArchivedRedactionRefusal(stream)}
	}
	return nil
}

// erased says whether a stream's live rows hold its task.erased.
func erased(logged []events.Event) bool {
	return slices.ContainsFunc(logged, func(e events.Event) bool { return e.Type == events.TaskErased })
}

// erasedRedaction is why an erased stream is never redacted.
func erasedRedaction(stream string) *events.Rejection {
	return &events.Rejection{Check: "redact", Reason: stream + " is erased: there is nothing left to redact"}
}

// ArchivedRedactionRefusal is why an archived task can't be redacted as it
// is: its rows are in the archive's blob, not the live log, so
// RedactArchived redacts it instead.
func ArchivedRedactionRefusal(stream string) string {
	return stream + " is archived: unarchive it, redact it, then archive it again"
}

// aSecret says what is wrong with a secret to redact: one that is empty,
// or part of what it becomes, would never be gone.
func aSecret(secret string) string {
	if secret == "" || strings.Contains(Redacted, secret) {
		return "a secret to redact is some text, and not part of " + Redacted
	}
	return ""
}

// redactable refuses a redaction the rows found can't make: none holds the
// secret; one is a knowledge version outside the agents stream, whose
// versions rewrittenIn doesn't follow; or one holds it in a key, which is
// refused here, before the rules would refuse its redaction for naming no
// fields.
func redactable(stream string, rewrites []rewrittenRow) *events.Rejection {
	if len(rewrites) == 0 {
		return holdsNone(stream)
	}
	for _, rw := range rewrites {
		if rw.kind == events.AgentLearned && stream != events.AgentsStream {
			return &events.Rejection{Check: "redact", Reason: fmt.Sprintf("the row at %d is a knowledge version, which is redacted by writing it again in full", rw.position)}
		}
	}
	for _, rw := range rewrites {
		if rw.inKey {
			return &events.Rejection{Check: "redact", Reason: fmt.Sprintf("the row at %d %s", rw.position, rw.refused)}
		}
	}
	return nil
}

// holdsNone is why a redaction of stream is refused when no row of it
// holds the secret.
func holdsNone(stream string) *events.Rejection {
	return &events.Rejection{Check: "redact", Reason: "no row of " + stream + " holds the secret"}
}

// HoldsNone says whether rejection is a redaction's, or its preview's,
// refusal of stream because no row of it holds the secret: a redaction
// across the whole log leaves such a stream out, rather than name it
// refused.
func HoldsNone(stream string, rejection *events.Rejection) bool {
	return rejection != nil && *rejection == *holdsNone(stream)
}

// rewritable refuses a redaction of stream with a row redactedData can't
// rewrite leaving the rest as it was. A secret left in a number gets what
// the user can do instead, which depends on the stream: only a task can
// be erased.
func (s *Store) rewritable(stream string, rewrites []rewrittenRow) *events.Rejection {
	for _, rw := range rewrites {
		if rw.refused == "" {
			continue
		}
		reason := fmt.Sprintf("the row at %d %s", rw.position, rw.refused)
		switch {
		case rw.inNumber && s.catalogue.IsTaskStream(stream):
			reason += ": erase the task, or redact a longer piece of the text"
		case rw.inNumber:
			reason += ": redact a longer piece of the text"
		}
		return &events.Rejection{Check: "redact", Reason: reason}
	}
	return nil
}
