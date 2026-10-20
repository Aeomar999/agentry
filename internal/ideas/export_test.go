package ideas

import (
	"testing"

	"gitlab.com/dev.chempa/agentry/internal/agents"
)

// BeforeMarking runs f between Accept making the task and marking the idea.
func BeforeMarking(t *testing.T, f func()) {
	beforeMarking = f
	t.Cleanup(func() { beforeMarking = func() {} })
}

// Similarity is the share of words two ideas' problems and proposals have
// in common, as sameText weighs it.
func Similarity(a, b *Idea) float64 {
	these, those := textWords(a), textWords(b)
	shared := 0
	for word := range these {
		if those[word] {
			shared++
		}
	}
	return float64(shared) / float64(len(these)+len(those)-shared)
}

// AutoAccepted is the note on an idea Agentry accepted for you under rule.
func AutoAccepted(idea *Idea, roster agents.Roster, bases agents.Bases, rule string) string {
	return autoAccepted(idea, roster, bases, rule)
}

// View is an idea as GET /api/ideas answers it, and AcceptReply what
// accepting one answers.
type (
	View        = ideaView
	AcceptReply = acceptance
)
