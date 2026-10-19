package ideas

import "testing"

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
