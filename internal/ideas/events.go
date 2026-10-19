package ideas

import (
	"strings"

	"gitlab.com/dev.chempa/agentry/internal/events"
)

// An idea's stream, named by its ID (idea-N): a feature a Visionaries
// session suggested for a repo, which you accept as a task or dismiss.
const (
	IdeaSuggested events.Type = "idea.suggested"
	IdeaAccepted  events.Type = "idea.accepted"  // you took it on: it became the task in "task", made just before
	IdeaDismissed events.Type = "idea.dismissed" // you turned it down, saying why in "note"
)

// Prefix starts every idea's stream name, so ideas never show up as tasks.
const Prefix = "idea-"

// IsStream says whether a stream is an idea's.
func IsStream(stream string) bool { return strings.HasPrefix(stream, Prefix) }

// Registration is what ideas adds to the event catalogue: its streams and
// the three types they hold.
func Registration() events.Registration {
	return events.Registration{Prefix: Prefix, Types: map[events.Type]events.Spec{
		IdeaSuggested: {Fields: []string{"title", "problem", "proposal", "size", "repo", "task"}, Writers: []events.ActorKind{events.Agent, events.System}},
		IdeaAccepted:  {Fields: []string{"task"}, Writers: []events.ActorKind{events.Human, events.System}}, // "note" may be empty; Agentry's: an idea the Visionaries agreed on by your rule
		IdeaDismissed: {Fields: []string{"note"}, Writers: []events.ActorKind{events.Human}},
	}}
}
